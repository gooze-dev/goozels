package main

import (
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/controller"
	"gooze.dev/pkg/goozels/internal/model"
)

func diag(line int, sev model.Severity, msg string) model.Diagnostic {
	return model.Diagnostic{Range: model.Range{Start: model.Position{Line: line}}, Severity: sev, Message: msg}
}

func sampleEntry(shown string) entry {
	return entry{shown: shown, check: controller.FileCheck{
		Results: []model.Diagnostic{diag(0, model.SeverityWarning, "summary"), diag(4, model.SeverityWarning, "survivor")},
		Edit:    []model.Diagnostic{diag(2, model.SeverityError, "bad directive")},
	}}
}

func TestRenderOrdersAndFlagsErrors(t *testing.T) {
	out := render([]entry{sampleEntry("a.go")}, renderOptions{})
	want := "a.go:1:1: warning: summary\na.go:3:1: error: bad directive\na.go:5:1: warning: survivor"
	if strings.Join(out.lines, "\n") != want || !out.errors {
		t.Errorf("lines =\n%s\nerrors = %v", strings.Join(out.lines, "\n"), out.errors)
	}
}

func TestRenderCaps(t *testing.T) {
	out := render([]entry{sampleEntry("a.go"), sampleEntry("b.go")}, renderOptions{maxLines: 2})
	got := strings.Join(out.lines, "\n")
	if !strings.Contains(got, "… 4 more; run goozels check a.go b.go\n"+controller.IgnoreHint) || len(out.lines) != 4 {
		t.Errorf("capped output:\n%s", got)
	}

	out = render([]entry{sampleEntry("a.go")}, renderOptions{maxBytes: 30})
	if len(out.lines) != 3 || out.lines[0] != "a.go:1:1: warning: summary" || out.lines[1] != "… 2 more; run goozels check a.go" {
		t.Errorf("byte-capped output = %q", out.lines)
	}

	if out := render([]entry{sampleEntry("a.go")}, renderOptions{hint: true}); out.lines[len(out.lines)-1] != controller.IgnoreHint {
		t.Errorf("hint option should add the hint under survivors, got %q", out.lines)
	}
}

func TestRenderBriefAndNotes(t *testing.T) {
	e := sampleEntry("a.go")
	e.brief = true
	e.note = "no results"
	out := render([]entry{e}, renderOptions{hint: true})
	want := "a.go:1:1: warning: summary" + briefSuffix + "\na.go:3:1: error: bad directive"
	if strings.Join(out.lines, "\n") != want {
		t.Errorf("brief lines =\n%s\nwant\n%s", strings.Join(out.lines, "\n"), want)
	}
	if len(out.notes) != 1 || out.notes[0] != "no results" {
		t.Errorf("notes = %q", out.notes)
	}
}

func TestRenderByteBoundary(t *testing.T) {
	ten := entry{shown: "f", check: controller.FileCheck{Edit: []model.Diagnostic{
		diag(0, model.SeverityWarning, "a"), diag(1, model.SeverityWarning, "b"),
	}}}
	n := len(formatDiagnostic("f", ten.check.Edit[0])) + 1 // a line and its newline
	if out := render([]entry{ten}, renderOptions{maxBytes: 2 * n}); len(out.lines) != 2 {
		t.Errorf("two lines of %d bytes fit in %d: got %q", n, 2*n, out.lines)
	}
	if out := render([]entry{ten}, renderOptions{maxBytes: 2*n - 1}); len(out.lines) != 2 || !strings.HasPrefix(out.lines[1], "… 1 more") {
		t.Errorf("only one line fits in %d bytes: got %q", 2*n-1, out.lines)
	}
	if out := render([]entry{ten}, renderOptions{maxLines: 2}); len(out.lines) != 2 {
		t.Errorf("two lines fit in maxLines 2: got %q", out.lines)
	}
}

func TestRenderSummaryOnly(t *testing.T) {
	e := entry{shown: "a.go", check: controller.FileCheck{Results: []model.Diagnostic{diag(0, model.SeverityInfo, "score")}}}
	if out := render([]entry{e}, renderOptions{hint: true}); strings.Join(out.lines, "\n") != "a.go:1:1: info: score" {
		t.Errorf("a file without survivors gets no hint: %q", out.lines)
	}
	e.brief = true
	if out := render([]entry{e}, renderOptions{}); strings.Join(out.lines, "\n") != "a.go:1:1: info: score" {
		t.Errorf("a brief summary without survivors gets no suffix: %q", out.lines)
	}
}

func TestRenderOneByteLimit(t *testing.T) {
	if out := render([]entry{sampleEntry("a.go")}, renderOptions{maxBytes: 1}); len(out.lines) == 0 || !strings.HasPrefix(out.lines[0], "… 3 more") {
		t.Errorf("no line fits in one byte: got %q", out.lines)
	}
}
