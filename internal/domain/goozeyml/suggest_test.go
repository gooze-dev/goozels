package goozeyml

import (
	"strings"
	"testing"
)

func TestSuggest(t *testing.T) {
	ed := newEditor()
	for key, want := range map[string]string{
		"parallel":     "run.parallel", // the only key with that name
		"log.parallel": "run.parallel",
		"run.paralel":  "run.parallel", // a typo
		"rnu":          "run",
		"log.levl":     "log.level",
		"bogus":        "",
		"exclude":      "paths.exclude",
	} {
		if got := ed.suggest(key); got != want {
			t.Errorf("suggest(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestEditDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"", "abc", 3}, {"kitten", "sitting", 3}, {"run", "run", 0}, {"rnu", "run", 2}} {
		if got := editDistance(c.a, c.b); got != c.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestUnclosedFlow(t *testing.T) {
	for value, want := range map[string]string{
		"[":                "[",
		"{parallel: 4":     "{",
		"[a, {b: 1}":       "[",
		"{parallel: 4}":    "",
		"[a, \"]\"]":       "",
		"[\"unterminated]": "[",
		"plain":            "",
		"":                 "",
	} {
		if got := unclosedFlow(value); got != want {
			t.Errorf("unclosedFlow(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestDiagnoseSuggestsKey(t *testing.T) {
	diags := newEditor().Diagnose("run:\n  paralel: 4\n")
	if len(diags) != 1 || diags[0].Message != `unknown gooze config key "run.paralel"; did you mean "run.parallel"?` {
		t.Errorf("diagnostics = %+v", diags)
	}
}

func TestDiagnoseUnclosedFlow(t *testing.T) {
	diags := newEditor().Diagnose("run: [\n")
	if len(diags) != 1 || !strings.HasPrefix(diags[0].Message, `invalid YAML: "[" is never closed`) || diags[0].Severity != 1 {
		t.Errorf("diagnostics = %+v, want one invalid-YAML error", diags)
	}
}

func TestSuggestAmbiguousTypo(t *testing.T) {
	// "log.max_ze" is two edits from both "log.max_age" and "log.max_size".
	if got := newEditor().suggest("log.max_ze"); got != "" {
		t.Errorf("suggest(log.max_ze) = %q, want no suggestion for a tie", got)
	}
}

func TestUnique(t *testing.T) {
	leaf := func(k string) bool { return strings.HasSuffix(k, ".x") }
	if got, ok := unique([]string{"a.x", "b.y"}, leaf); !ok || got != "a.x" {
		t.Errorf("unique = %q, %v", got, ok)
	}
	if got, ok := unique([]string{"a.x", "b.x"}, leaf); ok || got != "" {
		t.Errorf("two matches: unique = %q, %v; want none", got, ok)
	}
	if _, ok := unique([]string{"a.y"}, leaf); ok {
		t.Error("no match: unique should fail")
	}
}

func TestUnclosedFlowEdges(t *testing.T) {
	for value, want := range map[string]string{
		"x [":    "", // not a flow collection: plain scalars may hold brackets
		"A [":    "",
		"~ {":    "",
		`["!]"`:  "[", // a quoted "!]" does not close the list
		"[]] [":  "[", // a stray "]" does not cancel a later "["
		"[~":     "[",
		"{a: 1}": "",
	} {
		if got := unclosedFlow(value); got != want {
			t.Errorf("unclosedFlow(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestDiagnoseUnknownKeyWithoutSuggestion(t *testing.T) {
	diags := newEditor().Diagnose("bogus: 1\n")
	if len(diags) != 1 || diags[0].Message != `unknown gooze config key "bogus"` {
		t.Errorf("diagnostics = %+v", diags)
	}
}
