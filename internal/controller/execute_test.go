package controller

import (
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
			Title   string `json:"title"`
			Command string `json:"command"`
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
			hasGoozeRun = true
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

func TestReportDiagnosticsPublished(t *testing.T) {
	dir := t.TempDir()
	copyReports(t, filepath.Join(dir, config.DefaultReportsDir))
	goFile := filepath.Join(dir, "calc", "calc.go")

	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{
		"rootUri":      "file://" + dir,
		"capabilities": map[string]any{},
	})
	h.awaitResponse(t, "1")

	uri := "file://" + goFile
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": "package calc\n"},
	})
	// A change triggers a merged republish that includes report diagnostics.
	h.send(t, "textDocument/didChange", "", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []any{map[string]any{"text": "package calc\n\n"}},
	})

	note := h.awaitNotification(t, "textDocument/publishDiagnostics")
	var pd struct {
		Diagnostics []struct {
			Message string `json:"message"`
			Source  string `json:"source"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(note.Params, &pd); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	var summary, survivor, notCovered bool
	for _, d := range pd.Diagnostics {
		summary = summary || d.Message == "gooze: 3/16 mutants survived, 6 not covered — score 43.75%"
		survivor = survivor || strings.Contains(d.Message, "surviving mutant")
		notCovered = notCovered || strings.Contains(d.Message, "not covered by tests")
	}
	if !summary || !survivor || !notCovered {
		t.Errorf("expected summary, survivor and not-covered diagnostics, got %+v", pd.Diagnostics)
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

	if diags := s.reportDiagnostics("file:///proj/x.go"); len(diags) != 1 || !strings.Contains(diags[0].Message, "score 100.00%") {
		t.Errorf("diagnostics = %+v, want one summary", diags)
	}
	if diags := s.reportDiagnostics("file:///proj/y.go"); diags != nil {
		t.Errorf("diagnostics for a file without reports = %+v", diags)
	}
}

func TestReportDiagnosticsReadError(t *testing.T) {
	reports := report.NewMockReader(t)
	reports.EXPECT().ReadDir(mock.Anything).Return(nil, errors.New("no reports"))

	deps := productionDeps("gooze")
	deps.Reports = reports

	if diags := NewServer(io.Discard, io.Discard, deps).reportDiagnostics("file:///proj/x.go"); diags != nil {
		t.Errorf("diagnostics = %+v, want none", diags)
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
		"command":   "gooze.list",
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
