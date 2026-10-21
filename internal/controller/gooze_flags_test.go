package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"path/filepath"

	"gooze.dev/pkg/gooze/pkg/config"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// flagGooze is a GoozeRunner whose `run --help` lists flags, or fails.
type flagGooze struct {
	flags map[string]bool
	err   error
	calls *atomic.Int32
}

func (f flagGooze) Run(context.Context, string, ...string) (string, error) { return "", nil }

func (f flagGooze) RunFlags(context.Context) (map[string]bool, error) {
	if f.calls != nil {
		f.calls.Add(1)
	}

	return f.flags, f.err
}

func TestLoadGoozeFlags(t *testing.T) {
	deps := productionDeps("gooze")
	deps.Gooze = flagGooze{flags: map[string]bool{"run": true, "estimate": true}}
	s := NewServer(io.Discard, io.Discard, deps)

	if s.goozeSupports("run") {
		t.Fatal("nothing is supported before the probe")
	}

	s.loadGoozeFlags(context.Background())

	if !s.goozeSupports("run") || !s.goozeSupports("estimate") || s.goozeSupports("mutagens") {
		t.Errorf("supports run=%v estimate=%v mutagens=%v", s.goozeSupports("run"), s.goozeSupports("estimate"), s.goozeSupports("mutagens"))
	}
}

func TestLoadGoozeFlagsFailureMeansNone(t *testing.T) {
	deps := productionDeps("gooze")
	deps.Gooze = flagGooze{flags: map[string]bool{"run": true}, err: errors.New("no gooze")}
	s := NewServer(io.Discard, io.Discard, deps)

	s.loadGoozeFlags(context.Background())

	if s.goozeSupports("run") {
		t.Error("a failed probe must not claim support")
	}
}

func TestInitializedProbesGoozeOnce(t *testing.T) {
	var calls atomic.Int32

	deps := productionDeps("gooze")
	deps.Gooze = flagGooze{flags: map[string]bool{"run": true}, calls: &calls}
	h := newHarnessWithDeps(t, goplsResponder, deps)

	h.send(t, "initialized", "", map[string]any{})
	<-h.goplsSeen
	h.send(t, "initialized", "", map[string]any{})
	<-h.goplsSeen

	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)

	if n := calls.Load(); n != 1 {
		t.Errorf("gooze was probed %d times, want 1", n)
	}
}

// lensCommands asks for the code lenses of uri and returns their commands
// with arguments, e.g. "gooze.runFunction ./calc ^Add$".
func lensCommands(t *testing.T, h *harness, id, uri string) []string {
	t.Helper()

	h.send(t, "textDocument/codeLens", id, map[string]any{"textDocument": map[string]any{"uri": uri}})
	resp := h.awaitResponse(t, id)

	var lenses []struct {
		Command struct {
			Command   string `json:"command"`
			Arguments []any  `json:"arguments"`
		} `json:"command"`
	}
	if err := json.Unmarshal(resp.Result, &lenses); err != nil {
		t.Fatalf("decode code lenses: %v", err)
	}

	var got []string

	for _, l := range lenses {
		parts := []string{l.Command.Command}
		for _, a := range l.Command.Arguments {
			parts = append(parts, fmt.Sprint(a))
		}

		got = append(got, strings.Join(parts, " "))
	}

	return got
}

// openWithFlags starts a session whose gooze reports flags, waits for the
// probe, and opens uri with text.
func openWithFlags(t *testing.T, flags map[string]bool, uri, text string) *harness {
	t.Helper()

	var calls atomic.Int32

	deps := productionDeps("gooze")
	deps.Gooze = flagGooze{flags: flags, calls: &calls}
	h := newHarnessWithDeps(t, goplsResponder, deps)

	h.send(t, "initialize", "1", map[string]any{"rootUri": "file:///proj", "capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	h.send(t, "initialized", "", map[string]any{})

	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	time.Sleep(20 * time.Millisecond) // the probe stores its answer after the call returns

	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": text},
	})

	return h
}

const calcText = "package calc\n\nfunc Add(a, b int) int { return a + b }\n"

func TestCodeLensRunFunctionWhenGoozeHasRun(t *testing.T) {
	h := openWithFlags(t, map[string]bool{"run": true}, "file:///proj/calc/calc.go", calcText)

	got := lensCommands(t, h, "2", "file:///proj/calc/calc.go")
	if !slices.Contains(got, "gooze.runFunction ./calc ^Add$") {
		t.Errorf("lenses = %q, want a run-function lens for Add", got)
	}

	if !slices.Contains(got, "gooze.run ./calc") {
		t.Errorf("the file lenses must stay, got %q", got)
	}
}

func TestCodeLensNoRunFunctionWithoutGoozeRun(t *testing.T) {
	h := openWithFlags(t, map[string]bool{"estimate": true}, "file:///proj/calc/calc.go", calcText)

	for _, l := range lensCommands(t, h, "2", "file:///proj/calc/calc.go") {
		if strings.HasPrefix(l, command.RunFunction) {
			t.Errorf("a gooze without --run must not get function lenses: %q", l)
		}
	}
}

func TestCodeLensNoRunFunctionInTests(t *testing.T) {
	h := openWithFlags(t, map[string]bool{"run": true}, "file:///proj/calc/calc_test.go", "package calc\n\nfunc TestAdd(t *testing.T) {}\n")

	for _, l := range lensCommands(t, h, "2", "file:///proj/calc/calc_test.go") {
		if strings.HasPrefix(l, command.RunFunction) {
			t.Errorf("test files have nothing to mutate: %q", l)
		}
	}
}

// argsGooze records the arguments of each gooze run.
type argsGooze struct {
	mu   *sync.Mutex
	runs *[][]string
}

func (a argsGooze) Run(_ context.Context, _ string, args ...string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	*a.runs = append(*a.runs, args)

	return "Mutation score: 100.00% (1 killed, 0 survived; 0 cached, 1 tested; functions matching ^Add$)\n", nil
}

func (argsGooze) RunFlags(context.Context) (map[string]bool, error) {
	return map[string]bool{"run": true}, nil
}

func TestExecuteRunFunctionPassesThePattern(t *testing.T) {
	var (
		mu   sync.Mutex
		runs [][]string
	)

	deps := productionDeps("gooze")
	deps.Gooze = argsGooze{mu: &mu, runs: &runs}
	h := newHarnessWithDeps(t, goplsResponder, deps)

	h.send(t, "workspace/executeCommand", "5", map[string]any{"command": command.RunFunction, "arguments": []any{"./calc", "^Add$"}})
	h.awaitResponse(t, "5")

	mu.Lock()
	defer mu.Unlock()

	if len(runs) == 0 || strings.Join(runs[0], " ") != "run --run ^Add$ ./calc" {
		t.Errorf("gooze runs = %q", runs)
	}
}

// awaitRequest drains client-bound messages until the server sends a request
// with method.
func (h *harness) awaitRequest(t *testing.T, method string) *jsonrpc.Message {
	t.Helper()

	deadline := time.After(3 * time.Second)

	for {
		select {
		case msg := <-h.inbox:
			if msg.IsRequest() && msg.Method == method {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for request %s", method)
			return nil
		}
	}
}

// answer replies to a server request as the editor would.
func (h *harness) answer(t *testing.T, req *jsonrpc.Message, result any) {
	t.Helper()

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if err := h.clientW.Write(&jsonrpc.Message{JSONRPC: "2.0", ID: req.ID, Result: raw}); err != nil {
		t.Fatal(err)
	}
}

// functionRunSession runs gooze.runFunction in a copy of testdata whose last
// narrowed run left reports in .gooze-reports/run.
func functionRunSession(t *testing.T, gooze GoozeRunner) *harness {
	t.Helper()

	return functionRunSessionIn(t, gooze, true)
}

// functionRunSessionIn is functionRunSession, with or without reports in run/
// (a run with nothing to re-test leaves none).
func functionRunSessionIn(t *testing.T, gooze GoozeRunner, withRunReports bool) *harness {
	t.Helper()

	dir := projectCopy(t)
	if withRunReports {
		copyReports(t, filepath.Join(dir, config.DefaultReportsDir, "run"))
	} else if err := os.MkdirAll(filepath.Join(dir, config.DefaultReportsDir, "run"), 0o750); err != nil {
		// gooze leaves an empty run/ when it re-tested nothing.
		t.Fatal(err)
	}

	deps := productionDeps("gooze")
	deps.Gooze = gooze
	h := newHarnessWithDeps(t, goplsResponder, deps)

	h.send(t, "initialize", "1", map[string]any{"rootUri": "file://" + dir, "capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	// The offer can reach the client before the command's own response, so
	// callers wait for what they expect instead of draining up to it.
	h.send(t, "workspace/executeCommand", "5", map[string]any{"command": command.RunFunction, "arguments": []any{"./calc", "^Add$"}})

	return h
}

func TestRunFunctionOffersAMerge(t *testing.T) {
	var (
		mu   sync.Mutex
		runs [][]string
	)

	h := functionRunSession(t, argsGooze{mu: &mu, runs: &runs})

	req := h.awaitRequest(t, "window/showMessageRequest")

	var p lsp.ShowMessageRequestParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatal(err)
	}

	wantMsg := "gooze: function run done — score 43.75% — 7 killed, 3 survived, 6 not covered by any test. Its reports are in .gooze-reports/run until you merge them into the main reports."
	if p.Type != lsp.MessageInfo || p.Message != wantMsg || len(p.Actions) != 1 || p.Actions[0].Title != command.MergeTitle {
		t.Errorf("request = %+v\nwant message %q and one %q action", p, wantMsg, command.MergeTitle)
	}

	h.answer(t, req, lsp.MessageActionItem{Title: command.MergeTitle})

	shown := h.awaitNotification(t, "window/showMessage")
	for !strings.Contains(string(shown.Params), "gooze report merge") {
		shown = h.awaitNotification(t, "window/showMessage")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(runs) != 2 || strings.Join(runs[1], " ") != "report merge" {
		t.Errorf("gooze runs = %q, want the function run then report merge", runs)
	}
}

func TestRunFunctionMergeDeclined(t *testing.T) {
	var (
		mu   sync.Mutex
		runs [][]string
	)

	h := functionRunSession(t, argsGooze{mu: &mu, runs: &runs})
	h.answer(t, h.awaitRequest(t, "window/showMessageRequest"), nil)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(runs) != 1 {
		t.Errorf("a dismissed request must not merge; gooze runs = %q", runs)
	}
}

func TestRunFunctionFailedOffersNoMerge(t *testing.T) {
	h := functionRunSession(t, failingGooze{})

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case msg := <-h.inbox:
			if msg.Method == "window/showMessageRequest" {
				t.Fatalf("a failed run must not offer a merge: %s", msg.Params)
			}
		case <-deadline:
			return
		}
	}
}

func TestOtherClientResponsesStillReachGopls(t *testing.T) {
	h := newHarness(t, goplsResponder)

	if err := h.clientW.Write(&jsonrpc.Message{JSONRPC: "2.0", ID: json.RawMessage("42"), Result: json.RawMessage("null")}); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-h.goplsSeen:
		if msg.IDKey() != "42" {
			t.Errorf("gopls got %+v", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a response to a gopls request was not forwarded")
	}
}

func TestOnlyFunctionRunsOfferAMerge(t *testing.T) {
	for _, id := range []string{command.Run, command.View, command.Estimate} {
		var (
			mu   sync.Mutex
			runs [][]string
		)

		deps := productionDeps("gooze")
		deps.Gooze = argsGooze{mu: &mu, runs: &runs}
		h := newHarnessWithDeps(t, goplsResponder, deps)

		h.send(t, "workspace/executeCommand", "5", map[string]any{"command": id, "arguments": []any{"./calc"}})

		// Check every message, not just those after the response: an offer
		// would be sent before it.
		responded, deadline := false, time.After(3*time.Second)

	drain:
		for {
			select {
			case msg := <-h.inbox:
				if msg.Method == "window/showMessageRequest" {
					t.Errorf("%s offered a merge: %s", id, msg.Params)
				}

				if msg.IsResponse() && msg.IDKey() == "5" && !responded {
					responded, deadline = true, time.After(100*time.Millisecond)
				}
			case <-deadline:
				break drain
			}
		}

		if !responded {
			t.Errorf("%s got no response", id)
		}
	}
}

func TestRunFunctionMergeNeedsTheMergeButton(t *testing.T) {
	var (
		mu   sync.Mutex
		runs [][]string
	)

	h := functionRunSession(t, argsGooze{mu: &mu, runs: &runs})
	// Any other title, sorting before or after the merge button's, merges nothing.
	h.answer(t, h.awaitRequest(t, "window/showMessageRequest"), lsp.MessageActionItem{Title: "Not now"})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(runs) != 1 {
		t.Errorf("gooze runs = %q, want only the function run", runs)
	}
}

func TestRunFunctionThatTestedNothing(t *testing.T) {
	var (
		mu   sync.Mutex
		runs [][]string
	)

	h := functionRunSessionIn(t, argsGooze{mu: &mu, runs: &runs}, false)

	want := "gooze: nothing re-tested for ^Add$: it is unchanged since the last run, so its results are already shown, or it has nothing to mutate. To test it again anyway: gooze run --no-cache --run '^Add$' ./calc"

	deadline := time.After(3 * time.Second)

	for {
		select {
		case msg := <-h.inbox:
			if msg.Method == "window/showMessageRequest" {
				t.Fatalf("nothing to merge, but got an offer: %s", msg.Params)
			}

			var p lsp.ShowMessageParams
			if msg.Method == "window/showMessage" && json.Unmarshal(msg.Params, &p) == nil && strings.HasPrefix(p.Message, "gooze: nothing re-tested") {
				if p.Message != want || p.Type != lsp.MessageInfo {
					t.Errorf("message = %+v\nwant %q", p, want)
				}

				return
			}
		case <-deadline:
			t.Fatal("no message about the cached function")
		}
	}
}

func TestRunFunctionWithoutARunDir(t *testing.T) {
	dir := projectCopy(t)

	var (
		mu   sync.Mutex
		runs [][]string
	)

	deps := productionDeps("gooze")
	deps.Gooze = argsGooze{mu: &mu, runs: &runs}
	s := NewServer(io.Discard, io.Discard, deps)
	s.setWorkdir(dir)

	// No run/ at all (an older gooze, or a cleaned folder) is not an error.
	s.offerMerge([]string{"run", "--run", "^Add$", "./calc"})

	if len(s.clientRequests) != 0 {
		t.Errorf("no reports means no merge offer, got %d requests", len(s.clientRequests))
	}
}

func TestRunFunctionBelowAThresholdStillOffersTheMerge(t *testing.T) {
	h := functionRunSession(t, belowThresholdGooze{})

	if req := h.awaitRequest(t, "window/showMessageRequest"); !strings.Contains(string(req.Params), command.MergeTitle) {
		t.Errorf("a run below a threshold still wrote reports to merge, got %s", req.Params)
	}
}
