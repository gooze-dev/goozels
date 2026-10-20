package goozeyml

import (
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/model"
)

func TestCompleteTopLevel(t *testing.T) {
	items := Complete("ou", 0, 2)
	found := false
	for _, it := range items {
		if it.Label == "output" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'output' in top-level completions, got %+v", labels(items))
	}
}

func TestCompleteNested(t *testing.T) {
	text := "log:\n  "
	items := Complete(text, 1, 2)
	want := map[string]bool{"level": false, "filename": false, "verbose": false}
	for _, it := range items {
		if _, ok := want[it.Label]; ok {
			want[it.Label] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("expected nested log child %q in completions, got %v", k, labels(items))
		}
	}
}

func TestCompleteEnumValue(t *testing.T) {
	text := "log:\n  level: "
	line := "  level: "
	items := Complete(text, 1, len(line))
	if len(items) != 4 {
		t.Fatalf("expected 4 enum values, got %d: %v", len(items), labels(items))
	}
}

func TestHover(t *testing.T) {
	text := "run:\n  parallel: 4\n"
	h, ok := Hover(text, 1, 4)
	if !ok {
		t.Fatal("expected hover on run.parallel")
	}
	if !strings.Contains(h.Markdown, "run.parallel") {
		t.Errorf("hover missing key path: %q", h.Markdown)
	}
}

func TestDiagnoseBadValueAndUnknownKey(t *testing.T) {
	text := "log:\n  level: loud\nbogus: 1\n"
	diags := Diagnose(text)
	var badValue, unknownKey bool
	for _, d := range diags {
		if strings.Contains(d.Message, "log.level") {
			badValue = true
		}
		if strings.Contains(d.Message, "unknown gooze config key") {
			unknownKey = true
		}
	}
	if !badValue {
		t.Error("expected invalid-value diagnostic for log.level")
	}
	if !unknownKey {
		t.Error("expected unknown-key diagnostic for bogus")
	}
}

func TestDiagnoseValidConfig(t *testing.T) {
	text := "output: .gooze-reports\nrun:\n  parallel: 4\nlog:\n  level: info\n"
	if diags := Diagnose(text); len(diags) != 0 {
		t.Errorf("valid config produced diagnostics: %+v", diags)
	}
}

func labels(items []model.CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

// goozeRepoConfig is gooze's own gooze.yaml with the typo that made gooze ignore
// the whole file: "run:r" is a plain scalar in YAML, not the key "run".
const goozeRepoConfig = `log:
    level: 0
output: .gooze-reports
paths:
    exclude: []
run:r
    mutation_timeout: 120
    parallel: 1
version: 1
`

func TestDiagnoseMissingSpaceAfterColon(t *testing.T) {
	diags := Diagnose(goozeRepoConfig)
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want exactly 1 (children of the broken line are not re-reported): %+v", len(diags), diags)
	}
	d := diags[0]
	if d.Range.Start.Line != 5 || d.Severity != model.SeverityError || !strings.Contains(d.Message, "missing a space after") {
		t.Errorf("diagnostic = %+v", d)
	}
	if d.Range.Start.Column != 0 || d.Range.End.Column != len("run:r") {
		t.Errorf("range = %+v, want the whole \"run:r\" token", d.Range)
	}
}

func TestDiagnoseSectionWithScalarValue(t *testing.T) {
	diags := Diagnose("run: fast\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "is a section") {
		t.Fatalf("diagnostics = %+v", diags)
	}
	if got := Diagnose("run: {parallel: 4}\n"); len(got) != 0 {
		t.Errorf("flow mapping should be accepted, got %+v", got)
	}
}

func TestDiagnoseLineWithoutColon(t *testing.T) {
	diags := Diagnose("output: .gooze-reports\njunk\n")
	if len(diags) != 1 || diags[0].Range.Start.Line != 1 || !strings.Contains(diags[0].Message, "not a valid") {
		t.Fatalf("diagnostics = %+v", diags)
	}
}

func TestParseEntryColonRules(t *testing.T) {
	cases := []struct {
		line, key, value string
		ok               bool
	}{
		{"output: a:b", "output", "a:b", true}, // later colons belong to the value
		{"run:", "run", "", true},              // key with nested lines below
		{"run:r", "", "", false},               // plain scalar, not a key
		{"url: http://x", "url", "http://x", true},
	}
	for _, c := range cases {
		e, ok := parseEntry(0, c.line)
		if ok != c.ok || e.key != c.key || e.value != c.value {
			t.Errorf("parseEntry(%q) = %q, %q, %v; want %q, %q, %v", c.line, e.key, e.value, ok, c.key, c.value, c.ok)
		}
	}
}
