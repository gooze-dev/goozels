package controller

import (
	"io"
	"testing"

	"gooze.dev/pkg/goozels/internal/model"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

func TestIsConfigURI(t *testing.T) {
	s := NewServer(io.Discard, io.Discard, productionDeps("gooze"))
	if !s.isConfigURI("file:///proj/gooze.yaml") {
		t.Error("gooze.yaml is not a config file")
	}
	for _, uri := range []string{"file:///proj/gooze.yml", "file:///proj/.gooze.yaml", "file:///proj/.gooze.yml", "file:///proj/main.go"} {
		if s.isConfigURI(uri) {
			t.Errorf("%s is a config file, gooze only reads gooze.yaml", uri)
		}
	}
}

func TestLSPCompletion(t *testing.T) {
	line := "//gooze:ignore ar"
	items := []model.CompletionItem{
		{Label: "arithmetic", Detail: "d", Doc: "docs", InsertText: "arithmetic", ReplaceStart: 15, ReplaceEnd: 17},
	}
	list := lspCompletion(3, line, lsp.CompletionKindEnumMember, items)
	if len(list.Items) != 1 {
		t.Fatalf("items = %d", len(list.Items))
	}
	it := list.Items[0]
	if it.Kind != lsp.CompletionKindEnumMember || it.Documentation == nil || it.TextEdit == nil {
		t.Fatalf("item not fully populated: %+v", it)
	}
	if it.TextEdit.Range.Start.Line != 3 || it.TextEdit.Range.Start.Character != 15 || it.TextEdit.Range.End.Character != 17 {
		t.Errorf("textEdit range = %+v", it.TextEdit.Range)
	}
	if it.TextEdit.NewText != "arithmetic" {
		t.Errorf("newText = %q", it.TextEdit.NewText)
	}
}

func TestLSPHover(t *testing.T) {
	lines := []string{"//gooze:ignore comparison"}
	h := model.Hover{
		Markdown: "docs",
		Range:    model.Range{Start: model.Position{Line: 0, Column: 15}, End: model.Position{Line: 0, Column: 25}},
	}
	got := lspHover(lines, h)
	if got.Contents.Kind != "markdown" || got.Contents.Value != "docs" {
		t.Errorf("contents = %+v", got.Contents)
	}
	if got.Range == nil || got.Range.Start.Character != 15 {
		t.Errorf("range = %+v", got.Range)
	}
}

func TestLSPDiagnostics(t *testing.T) {
	text := "x := 1 //gooze:ignore bogus"
	ds := []model.Diagnostic{{
		Range:    model.Range{Start: model.Position{Line: 0, Column: 22}, End: model.Position{Line: 0, Column: 27}},
		Severity: model.SeverityWarning,
		Message:  "unknown",
		Source:   "gooze",
	}}
	got := lspDiagnostics(text, ds)
	if len(got) != 1 || got[0].Severity != int(model.SeverityWarning) || got[0].Source != "gooze" {
		t.Fatalf("diagnostics = %+v", got)
	}
	if got[0].Range.Start.Character != 22 {
		t.Errorf("range start = %d", got[0].Range.Start.Character)
	}
}

func TestLSPCodeLenses(t *testing.T) {
	lines := []string{"package main"}
	ls := []model.CodeLens{{
		Range:   model.Range{Start: model.Position{Line: 0, Column: 0}, End: model.Position{Line: 0, Column: 0}},
		Command: model.Command{Title: "gooze: run package", Command: "gooze.run", Arguments: []any{"./..."}},
	}}
	got := lspCodeLenses(lines, ls)
	if len(got) != 1 || got[0].Command == nil {
		t.Fatalf("code lenses = %+v", got)
	}
	if got[0].Command.Command != "gooze.run" || got[0].Command.Title != "gooze: run package" {
		t.Errorf("command = %+v", got[0].Command)
	}
}

func TestLSPCompletionWithoutDocHasNoDocumentation(t *testing.T) {
	items := []model.CompletionItem{{Label: "arithmetic", InsertText: "arithmetic"}}
	list := lspCompletion(0, "//gooze:ignore ", lsp.CompletionKindEnumMember, items)
	if doc := list.Items[0].Documentation; doc != nil {
		t.Errorf("documentation = %+v, want none for an item without docs", doc)
	}
}

func TestPositionFromModel(t *testing.T) {
	lines := []string{"ab", "éx"}
	tests := map[string]struct {
		pos  model.Position
		want lsp.Position
	}{
		"first line":        {model.Position{Line: 0, Column: 2}, lsp.Position{Line: 0, Character: 2}},
		"second line":       {model.Position{Line: 1, Column: 3}, lsp.Position{Line: 1, Character: 2}}, // é is 2 bytes, 1 UTF-16 unit
		"line after last":   {model.Position{Line: 2, Column: 3}, lsp.Position{Line: 2, Character: 0}},
		"line far past end": {model.Position{Line: 9, Column: 3}, lsp.Position{Line: 9, Character: 0}},
		"negative line":     {model.Position{Line: -1, Column: 3}, lsp.Position{Line: -1, Character: 0}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := positionFromModel(lines, tt.pos); got != tt.want {
				t.Errorf("positionFromModel(%+v) = %+v, want %+v", tt.pos, got, tt.want)
			}
		})
	}
}
