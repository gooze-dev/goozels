package ignore

import (
	"strings"
	"testing"
)

func TestComplete(t *testing.T) {
	line := "x := 1 + 2 //gooze:ignore ar"
	col := len(line) // cursor after "ar"
	items := Complete(line, col)
	if len(items) == 0 {
		t.Fatal("expected completions for 'ar' prefix")
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Label, "ar") {
			t.Errorf("item %q does not match prefix 'ar'", it.Label)
		}
	}
	if items[0].Label != "arithmetic" {
		t.Errorf("first item = %q, want arithmetic", items[0].Label)
	}
	// Replace range should cover the typed "ar".
	if line[items[0].ReplaceStart:items[0].ReplaceEnd] != "ar" {
		t.Errorf("replace range = %q, want 'ar'", line[items[0].ReplaceStart:items[0].ReplaceEnd])
	}
}

func TestCompleteOutsideDirective(t *testing.T) {
	if items := Complete("x := 1 + 2", 5); items != nil {
		t.Errorf("expected no completions outside a directive, got %d", len(items))
	}
}

func TestHover(t *testing.T) {
	line := "//gooze:ignore comparison"
	col := strings.Index(line, "comparison") + 2
	h, ok := Hover(line, 0, col)
	if !ok {
		t.Fatal("expected hover over mutagen name")
	}
	if !strings.Contains(h.Markdown, "comparison") {
		t.Errorf("hover markdown missing mutagen name: %q", h.Markdown)
	}
}

func TestDiagnoseUnknownMutagen(t *testing.T) {
	src := "package main\n\nfunc f() {\n\tx := 1 //gooze:ignore bogus,arithmetic\n\t_ = x\n}\n"
	diags := Diagnose(src)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "bogus") {
		t.Errorf("diagnostic should mention 'bogus': %q", diags[0].Message)
	}
	if diags[0].Range.Start.Line != 3 {
		t.Errorf("diagnostic on line %d, want 3", diags[0].Range.Start.Line)
	}
}

func TestCompleteAfterTrailingSpace(t *testing.T) {
	line := "y := x * 2 //gooze:ignore "
	items := Complete(line, len(line))
	if len(items) != 9 {
		t.Fatalf("expected every mutagen after a trailing space, got %d", len(items))
	}
	if items[0].ReplaceStart != len(line) || items[0].ReplaceEnd != len(line) {
		t.Errorf("replace range = %d:%d, want empty at %d", items[0].ReplaceStart, items[0].ReplaceEnd, len(line))
	}
}

func TestCompleteInBlockComment(t *testing.T) {
	line := "/* gooze:ignore br */"
	items := Complete(line, strings.Index(line, "br")+2)
	if len(items) != 1 || items[0].Label != "branch" {
		t.Fatalf("expected branch completion, got %+v", items)
	}
	if items[0].Detail == "" || items[0].Doc == "" {
		t.Errorf("completion lacks detail or doc: %+v", items[0])
	}
}

func TestCursorAt(t *testing.T) {
	line := "//gooze:ignore arith,comparison"
	col := len("//gooze:ignore arith")
	c, ok := cursorAt(line, col)
	if !ok {
		t.Fatal("expected cursor in list")
	}
	if c.prefix != "arith" || line[c.tokenStart:c.tokenEnd] != "arith" {
		t.Errorf("cursor = %+v, want token arith", c)
	}

	if _, ok := cursorAt(line, 2); ok {
		t.Error("cursor in keyword should not be in list")
	}

	c, ok = cursorAt(line, len("//gooze:ignore "))
	if !ok || c.prefix != "" {
		t.Errorf("empty-token cursor: ok=%v prefix=%q", ok, c.prefix)
	}

	if _, ok := cursorAt("//gooze:ignore", len("//gooze:ignore")); !ok {
		t.Error("cursor right after a bare keyword should be in list")
	}
}

func TestKeywordPrefixIsNotDirective(t *testing.T) {
	line := "x := 1 + 2 //gooze:ignorearithmetic"
	if items := Complete(line, len(line)); items != nil {
		t.Errorf("completions = %+v, want none", items)
	}
	if _, ok := Hover(line, 0, len(line)-2); ok {
		t.Error("hover on //gooze:ignorearithmetic, want none")
	}
	if diags := Diagnose("package p\n\n" + line + "\n"); diags != nil {
		t.Errorf("diagnostics = %+v, want none", diags)
	}
}

func TestHoverKeyword(t *testing.T) {
	line := "x := 1 // gooze:ignore numbers"
	kw := strings.Index(line, "gooze:ignore")
	h, ok := Hover(line, 2, kw+3)
	if !ok || !strings.Contains(h.Markdown, "skips mutation generation") {
		t.Fatalf("keyword hover = %+v, %v", h, ok)
	}
	if h.Range.Start.Column != kw || h.Range.End.Column != kw+len("gooze:ignore") || h.Range.Start.Line != 2 {
		t.Errorf("hover range = %+v", h.Range)
	}
}

func TestHoverUnknownMutagen(t *testing.T) {
	line := "//gooze:ignore Bogus"
	h, ok := Hover(line, 0, len(line)-1)
	if !ok || !strings.Contains(h.Markdown, "Unknown gooze mutagen `Bogus`") {
		t.Errorf("hover = %+v, %v", h, ok)
	}
}
