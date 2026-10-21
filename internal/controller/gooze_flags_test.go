package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gooze.dev/pkg/goozels/internal/domain/command"
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
