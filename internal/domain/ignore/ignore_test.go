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
