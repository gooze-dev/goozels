package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"gooze.dev/pkg/gooze/pkg/config"
	"gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

func TestCodeLensAddsGoozeRun(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")

	uri := "file:///proj/main.go"
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": "package main\n"},
	})
	h.send(t, "textDocument/codeLens", "2", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	resp := h.awaitResponse(t, "2")

	var lenses []struct {
		Command struct {
			Title     string `json:"title"`
			Command   string `json:"command"`
			Arguments []any  `json:"arguments"`
		} `json:"command"`
	}
	if err := json.Unmarshal(resp.Result, &lenses); err != nil {
		t.Fatalf("decode code lenses: %v", err)
	}
	var hasGopls, hasGoozeRun, hasReport bool
	for _, l := range lenses {
		switch l.Command.Command {
		case "gopls.noop":
			hasGopls = true
		case "gooze.run":
			// The workspace root defaults to "." and /proj is outside it.
			hasGoozeRun = l.Command.Title == "gooze: run package" && len(l.Command.Arguments) == 1 && l.Command.Arguments[0] == "./..."
		case "gooze.view":
			hasReport = true
		}
	}
	if !hasGopls {
		t.Error("gopls code lens was dropped")
	}
	if !hasGoozeRun {
		t.Error("gooze.run code lens was not added")
	}
	if !hasReport {
		t.Error("gooze.view (report) code lens was not added")
	}
}

// projectCopy copies testdata's sample project and the reports a real gooze
// run made for it to a temp workspace, and returns its root.
func projectCopy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "project"))); err != nil {
		t.Fatal(err)
	}
	copyReports(t, filepath.Join(dir, config.DefaultReportsDir))
	return dir
}

// openAndChange opens a Go file with text and changes it to changed, which
// triggers a merged republish, and returns the published messages.
func openAndChange(t *testing.T, dir, goFile, text, changed string) []string {
	t.Helper()
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{
		"rootUri":      "file://" + dir,
		"capabilities": map[string]any{},
	})
	h.awaitResponse(t, "1")

	uri := "file://" + goFile
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": text},
	})
	h.send(t, "textDocument/didChange", "", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []any{map[string]any{"text": changed}},
	})

	note := h.awaitNotification(t, "textDocument/publishDiagnostics")
	var pd struct {
		Diagnostics []struct {
			Message string `json:"message"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(note.Params, &pd); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	var messages []string
	for _, d := range pd.Diagnostics {
		messages = append(messages, d.Message)
	}
	return messages
}

func TestReportDiagnosticsPublished(t *testing.T) {
	dir := projectCopy(t)
	goFile := filepath.Join(dir, "calc", "calc.go")
	text, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(openAndChange(t, dir, goFile, "package calc\n", string(text)), "\n")
	for _, want := range []string{
		"gooze: score 43.75% — 7 killed, 3 survived, 6 not covered by any test",
		"gooze: 3 surviving mutants (comparison ×2, numbers) — tests still pass with `return n > 0` → `return n != 0`, `return n >= 0`, `return n > 1`",
		"gooze: 6 mutants not covered by any test (arithmetic ×4, numbers ×2) — `return n * 2` → `return n % 2`,",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, got)
		}
	}
}

func TestReportDiagnosticsStaleBuffer(t *testing.T) {
	dir := projectCopy(t)
	goFile := filepath.Join(dir, "calc", "calc.go")

	got := openAndChange(t, dir, goFile, "package calc\n", "package calc\n\n")
	want := "gooze: results are from before calc.go changed; score then 43.75%; rerun: gooze run ./calc"
	if len(got) != 1 || got[0] != want {
		t.Errorf("an edited buffer should show only the stale notice, got %q", got)
	}
}

func TestReportDiagnosticsReadsWorkspaceReports(t *testing.T) {
	reports := report.NewMockReader(t)
	reports.EXPECT().ReadDir(filepath.Join("/proj", config.DefaultReportsDir)).Return([]report.Report{{
		Source: report.Source{Origin: &report.File{ShortPath: "x.go"}},
		Result: []report.Result{{Name: "arithmetic", Mutations: []report.MutationResult{{Status: report.Killed}}}},
	}}, nil).Twice()

	deps := productionDeps("gooze")
	deps.Reports = reports
	s := NewServer(io.Discard, io.Discard, deps)
	s.setWorkdir("/proj")

	if fc := s.checker.Check("/proj", "/proj/x.go", "package x\n"); fc.Results == nil || len(fc.Results.Markers) != 0 || !strings.Contains(fc.Results.Summary.Message, "score 100.00%") {
		t.Errorf("results = %+v, want one summary", fc.Results)
	}
	if fc := s.checker.Check("/proj", "/proj/y.go", "package y\n"); fc.Results != nil {
		t.Errorf("results for a file without reports = %+v", fc.Results)
	}
}

func TestReportDiagnosticsReadError(t *testing.T) {
	reports := report.NewMockReader(t)
	reports.EXPECT().ReadDir(mock.Anything).Return(nil, errors.New("no reports"))

	deps := productionDeps("gooze")
	deps.Reports = reports

	if fc := NewServer(io.Discard, io.Discard, deps).checker.Check("/proj", "/proj/x.go", ""); fc.Results != nil {
		t.Errorf("results = %+v, want none", fc.Results)
	}
}

// copyReports copies the gooze-generated reports in testdata/reports to dir.
func copyReports(t *testing.T, dir string) {
	t.Helper()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "reports"))); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteGoozeCommandRunsLocally(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub script uses a POSIX shebang")
	}
	stub := writeStubGooze(t)
	h := newHarnessWithGooze(t, goplsResponder, stub)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	drainGopls(h)

	h.send(t, "workspace/executeCommand", "5", map[string]any{
		"command":   "gooze.estimate",
		"arguments": []any{"./..."},
	})

	// The server reports progress and then responds.
	note := h.awaitNotification(t, "window/showMessage")
	var sm struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(note.Params, &sm)
	if sm.Message == "" {
		t.Error("expected a non-empty showMessage")
	}
	h.awaitResponse(t, "5")

	// The gooze command must not be forwarded to gopls.
	for {
		select {
		case m := <-h.goplsSeen:
			if m.Method == "workspace/executeCommand" {
				t.Fatal("gooze command was forwarded to gopls")
			}
		default:
			return
		}
	}
}

func TestExecuteNonGoozeCommandForwarded(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	drainGopls(h)

	h.send(t, "workspace/executeCommand", "6", map[string]any{"command": "gopls.something"})
	h.awaitResponse(t, "6")

	forwarded := false
	for {
		select {
		case m := <-h.goplsSeen:
			if m.Method == "workspace/executeCommand" {
				forwarded = true
			}
		default:
			if !forwarded {
				t.Error("non-gooze command was not forwarded to gopls")
			}
			return
		}
	}
}

// writeStubGooze creates an executable that prints its args, standing in for the
// gooze binary.
func writeStubGooze(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gooze-stub")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho \"gooze stub $@\"\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

// recordingGooze is a GoozeRunner that returns canned output.
type recordingGooze struct{ out string }

func (r recordingGooze) Run(context.Context, string, ...string) (string, error) { return r.out, nil }

func (r recordingGooze) RunFlags(context.Context) (map[string]bool, error) { return nil, nil }

// notifications collects what a server writes to the client.
func notifications(t *testing.T, raw []byte) map[string][]string {
	t.Helper()
	got := map[string][]string{}
	r := jsonrpc.NewReader(bytes.NewReader(raw))
	for {
		msg, err := r.Read()
		if err != nil {
			return got
		}
		var p struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		got[msg.Method] = append(got[msg.Method], p.Message)
	}
}

func TestReportGoozeResultScoreAndCappedLog(t *testing.T) {
	dir := projectCopy(t)
	var client bytes.Buffer
	deps := productionDeps("gooze")
	deps.Gooze = recordingGooze{out: strings.Repeat("a report line\n", maxLogBytes/10)}
	s := NewServer(&client, io.Discard, deps)
	s.setWorkdir(dir)

	s.runGoozeCommand(&jsonrpc.Message{ID: json.RawMessage("1")}, lsp.ExecuteCommandParams{Command: "gooze.view"})

	got := notifications(t, client.Bytes())
	logs := got["window/logMessage"]
	if len(logs) != 1 || len(logs[0]) > maxLogBytes+200 || !strings.HasSuffix(logs[0], "more lines; run `gooze report view` in a terminal for all of it") {
		t.Errorf("log message is %d bytes, ending %q", len(logs[0]), logs[0][max(0, len(logs[0])-80):])
	}
	shown := strings.Join(got["window/showMessage"], "\n")
	if !strings.Contains(shown, "gooze report view finished — score 43.75% — 7 killed, 3 survived, 6 not covered by any test") {
		t.Errorf("finish message should carry the score, got:\n%s", shown)
	}
}

func TestScoreScope(t *testing.T) {
	for _, c := range []struct{ id, pattern, want string }{
		{"gooze.run", "./calc", "./calc"},
		{"gooze.run", "", "./..."},
		{"gooze.view", "", "./..."},
		{"gooze.estimate", "./calc", ""},
	} {
		args, _ := command.Args(c.id, c.pattern)
		if got := scoreScope(c.id, args); got != c.want {
			t.Errorf("scoreScope(%q, %q) = %q, want %q", c.id, c.pattern, got, c.want)
		}
	}
}

func TestCapLog(t *testing.T) {
	if got := capLog("gooze x", "short\n"); got != "short\n" {
		t.Errorf("short output changed: %q", got)
	}
	long := strings.Repeat("x", maxLogBytes+10)
	if got := capLog("gooze x", long); !strings.HasPrefix(got, strings.Repeat("x", maxLogBytes)+"… 1 more line;") {
		t.Errorf("a single long line should be cut at the limit, got %q…", got[:20])
	}
}

func TestDidSaveOfTestRepublishesOpenGoFiles(t *testing.T) {
	dir := projectCopy(t)
	goFile := filepath.Join(dir, "calc", "calc.go")
	text, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatal(err)
	}

	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"rootUri": "file://" + dir, "capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": "file://" + goFile, "languageId": "go", "version": 1, "text": string(text)},
	})

	test := filepath.Join(dir, "calc", "calc_test.go")
	if err := os.WriteFile(test, []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.send(t, "textDocument/didSave", "", map[string]any{"textDocument": map[string]any{"uri": "file://" + test}})

	note := h.awaitNotification(t, "textDocument/publishDiagnostics")
	if !strings.Contains(string(note.Params), "tests changed since the last run (calc_test.go)") {
		t.Errorf("saving a test should republish calc.go as out of date, got %s", note.Params)
	}
}

func TestCodeLensScopedToPackage(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"rootUri": "file:///proj", "capabilities": map[string]any{}})
	h.awaitResponse(t, "1")

	uri := "file:///proj/internal/domain/x.go"
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": "package domain\n"},
	})
	h.send(t, "textDocument/codeLens", "2", map[string]any{"textDocument": map[string]any{"uri": uri}})
	resp := h.awaitResponse(t, "2")

	if !strings.Contains(string(resp.Result), `"title":"gooze: run package","command":"gooze.run","arguments":["./internal/domain"]`) {
		t.Errorf("run lens should target the file's package, got %s", resp.Result)
	}
}

// failingGooze is a GoozeRunner whose runs fail.
type failingGooze struct{}

func (failingGooze) Run(context.Context, string, ...string) (string, error) {
	return "", errors.New("exit status 1")
}

func (failingGooze) RunFlags(context.Context) (map[string]bool, error) {
	return nil, errors.New("exit status 1")
}

func TestReportGoozeResultFailure(t *testing.T) {
	var client bytes.Buffer
	deps := productionDeps("gooze")
	deps.Gooze = failingGooze{}
	s := NewServer(&client, io.Discard, deps)

	s.runGoozeCommand(&jsonrpc.Message{ID: json.RawMessage("1")}, lsp.ExecuteCommandParams{Command: "gooze.run"})

	got := notifications(t, client.Bytes())
	if logs := got["window/logMessage"]; len(logs) != 0 {
		t.Errorf("a run without output should log nothing, got %q", logs)
	}
	shown := got["window/showMessage"]
	if len(shown) != 2 || shown[1] != "gooze run ./... failed: exit status 1" {
		t.Errorf("showMessage = %q", shown)
	}
}

func TestScoreOfOneReport(t *testing.T) {
	deps := productionDeps("gooze")
	deps.Reports = fakeReports{reports: []report.Report{{
		Source: report.Source{Origin: &report.File{ShortPath: "pkg/x.go"}},
		Result: []report.Result{{Name: "arithmetic", Mutations: []report.MutationResult{{Status: report.Killed}}}},
	}}}
	s := NewServer(io.Discard, io.Discard, deps)
	s.setWorkdir("/w")

	if got := s.score("./pkg"); got != "score 100.00% — 1 killed" {
		t.Errorf("score(./pkg) = %q", got)
	}
	if got := s.score("./other"); got != "" {
		t.Errorf("score of a package without reports = %q, want empty", got)
	}
	if got := s.score(""); got != "" {
		t.Errorf("score without a scope = %q, want empty", got)
	}
}

func TestCapLogBoundaries(t *testing.T) {
	whole := strings.Repeat("x", maxLogBytes-1) + "\n"
	if got := capLog("gooze x", whole); got != whole {
		t.Error("output of exactly maxLogBytes should be logged whole")
	}
	if maxLogBytes < 16<<10 {
		t.Errorf("maxLogBytes = %d; a package's report should fit", maxLogBytes)
	}

	line := strings.Repeat("y", 99) + "\n"
	got := capLog("gooze x", strings.Repeat(line, maxLogBytes/100+10))
	kept, rest, _ := strings.Cut(got, "… ")
	if len(kept)%100 != 0 || !strings.HasPrefix(rest, "10 more lines;") {
		t.Errorf("output should be cut at a line end: kept %d bytes, then %q", len(kept), rest[:min(len(rest), 20)])
	}
}

func TestEstimateFinishesWithoutScore(t *testing.T) {
	dir := projectCopy(t)
	var client bytes.Buffer
	deps := productionDeps("gooze")
	deps.Gooze = recordingGooze{}
	s := NewServer(&client, io.Discard, deps)
	s.setWorkdir(dir)

	s.runGoozeCommand(&jsonrpc.Message{ID: json.RawMessage("1")}, lsp.ExecuteCommandParams{Command: "gooze.estimate"})

	shown := notifications(t, client.Bytes())["window/showMessage"]
	if len(shown) != 2 || shown[1] != "gooze run --estimate ./... finished" {
		t.Errorf("showMessage = %q, want no score after an estimate", shown)
	}
}

func TestScoreWithoutScopeIgnoresRootReports(t *testing.T) {
	deps := productionDeps("gooze")
	deps.Reports = fakeReports{reports: []report.Report{{
		Source: report.Source{Origin: &report.File{ShortPath: "x.go"}},
		Result: []report.Result{{Name: "arithmetic", Mutations: []report.MutationResult{{Status: report.Killed}}}},
	}}}
	s := NewServer(io.Discard, io.Discard, deps)
	s.setWorkdir("/w")
	if got := s.score(""); got != "" {
		t.Errorf("score(\"\") = %q, want empty even with reports in the root package", got)
	}
}
