package goozeyml

import (
	"strings"
	"testing"

	"gooze.dev/pkg/gooze/pkg/config"

	"gooze.dev/pkg/goozels/internal/model"
)

func newEditor() Editor { return New(config.NewRegistry()) }

func TestCompleteTopLevel(t *testing.T) {
	items := newEditor().Complete("ou", 0, 2)
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
	text := "run:\n  "
	items := newEditor().Complete(text, 1, 2)
	want := map[string]bool{"parallel": false, "mutation_timeout": false, "coverage_profile": false}
	for _, it := range items {
		if _, ok := want[it.Label]; ok {
			want[it.Label] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("expected nested run child %q in completions, got %v", k, labels(items))
		}
	}
}

func TestCompleteUnderScalarKey(t *testing.T) {
	if items := newEditor().Complete("output: x\n  ", 1, 2); items != nil {
		t.Errorf("expected no completions under a scalar key, got %v", labels(items))
	}
}

func TestCompleteEnumValue(t *testing.T) {
	text := "log:\n  level: "
	line := "  level: "
	items := newEditor().Complete(text, 1, len(line))
	want := []string{"debug", "info", "warn", "warning", "error"}
	if got := labels(items); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("enum values = %v, want %v", got, want)
	}
}

func TestHover(t *testing.T) {
	text := "run:\n  parallel: 4\n"
	h, ok := newEditor().Hover(text, 1, 4)
	if !ok {
		t.Fatal("expected hover on run.parallel")
	}
	if !strings.Contains(h.Markdown, "run.parallel") || !strings.Contains(h.Markdown, "Default: `1`") {
		t.Errorf("hover = %q, want key path and default", h.Markdown)
	}
}

func TestHoverDefaults(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "output: x\n", want: "Default: `.gooze-reports`"},
		{text: "no-cache: true\n", want: "Default: `false`"},
		{text: "paths:\n  exclude: []\n", want: "Default: `[]`"},
	}
	for _, tt := range tests {
		lines := strings.Split(tt.text, "\n")
		line := len(lines) - 2
		h, ok := newEditor().Hover(tt.text, line, strings.Index(lines[line], ":")-1)
		if !ok || !strings.Contains(h.Markdown, tt.want) {
			t.Errorf("hover %q = %q, want %q", tt.text, h.Markdown, tt.want)
		}
	}

	h, ok := newEditor().Hover("run:\n  coverage_profile: c.out\n", 1, 4)
	if !ok || strings.Contains(h.Markdown, "Default") {
		t.Errorf("empty default should be omitted, hover = %q", h.Markdown)
	}
}

func TestDiagnoseBadValueAndUnknownKey(t *testing.T) {
	text := "log:\n  level: loud\nbogus: 1\n"
	diags := newEditor().Diagnose(text)
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
	text := "output: .gooze-reports\nrun:\n  parallel: 4\n  coverage_profile: cover.out\nlog:\n  level: warning\n"
	if diags := newEditor().Diagnose(text); len(diags) != 0 {
		t.Errorf("valid config produced diagnostics: %+v", diags)
	}
}

func TestDiagnoseUsesRegistry(t *testing.T) {
	registry := config.NewMockRegistry(t)
	registry.EXPECT().Lookup("mode").Return(config.Field{Name: "mode", Kind: config.KindEnum, Enum: []string{"fast"}}, true)

	diags := New(registry).Diagnose("mode: slow\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "expected one of: fast") {
		t.Errorf("diagnostics = %+v, want one invalid-value diagnostic", diags)
	}
}

func TestValidateScalar(t *testing.T) {
	level, _ := config.Lookup(config.KeyLogLevel)
	parallel, _ := config.Lookup(config.KeyRunParallel)
	noCache, _ := config.Lookup(config.KeyNoCache)
	output, _ := config.Lookup(config.KeyOutput)

	tests := []struct {
		field config.Field
		raw   string
		valid bool
	}{
		{field: level, raw: "info", valid: true},
		{field: level, raw: "WARNING", valid: true},
		{field: level, raw: "-4", valid: true},
		{field: level, raw: "loud"},
		{field: config.Field{Kind: config.KindEnum, Enum: []string{"a"}}, raw: "1"},
		{field: parallel, raw: "4", valid: true},
		{field: parallel, raw: "lots"},
		{field: noCache, raw: "TRUE", valid: true},
		{field: noCache, raw: "maybe"},
		{field: output, raw: "anything", valid: true},
		{field: parallel, raw: " ", valid: true},
	}
	for _, tt := range tests {
		if msg := validateScalar(tt.field, tt.raw); (msg == "") != tt.valid {
			t.Errorf("validateScalar(%s, %q) = %q, want valid=%v", tt.field.Name, tt.raw, msg, tt.valid)
		}
	}
}

func labels(items []model.CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}
