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
