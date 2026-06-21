package report

import (
	"strings"
	"testing"

	"gooze.dev/goozels/internal/model"
)

func sample() FileReport {
	return FileReport{
		FullPath: "/p/x.go",
		Mutations: []Mutation{
			{ID: "a", Status: Killed},
			{ID: "b", Status: Survived},
			{ID: "c", Status: Killed},
			{ID: "d", Status: Skipped},
		},
	}
}

func TestSummarizeAndScore(t *testing.T) {
	s := Summarize(sample())
	if s.Total != 4 || s.Killed != 2 || s.Survived != 1 || s.Skipped != 1 {
		t.Fatalf("summary = %+v", s)
	}
	if s.Score() != 50 {
		t.Errorf("score = %d, want 50", s.Score())
	}
	if Summarize(FileReport{}).Score() != 100 {
		t.Error("empty report should score 100")
	}
}

func TestMessage(t *testing.T) {
	msg := Summarize(sample()).Message()
	if !strings.Contains(msg, "1/4 mutants survived") || !strings.Contains(msg, "50%") {
		t.Errorf("message = %q", msg)
	}
}

func TestSummaryDiagnosticSeverity(t *testing.T) {
	if d := SummaryDiagnostic(sample()); d.Severity != model.SeverityWarning {
		t.Errorf("severity with survivors = %v, want warning", d.Severity)
	}
	clean := FileReport{Mutations: []Mutation{{ID: "a", Status: Killed}}}
	if d := SummaryDiagnostic(clean); d.Severity != model.SeverityInfo {
		t.Errorf("severity without survivors = %v, want info", d.Severity)
	}
}

func TestSurvivorDiagnostics(t *testing.T) {
	r := sample()
	r.Diff = []byte("--- a/x.go\n+++ b/x.go\n@@ -10,3 +10,3 @@\n-\ta + b\n+\ta - b\n")
	diags := SurvivorDiagnostics(r)
	if len(diags) != 1 {
		t.Fatalf("expected 1 survivor diagnostic, got %d", len(diags))
	}
	if diags[0].Range.Start.Line != 9 { // 1-based 10 -> 0-based 9
		t.Errorf("line = %d, want 9", diags[0].Range.Start.Line)
	}

	// No survivors -> no diagnostics even with a diff.
	clean := FileReport{Mutations: []Mutation{{ID: "a", Status: Killed}}, Diff: r.Diff}
	if got := SurvivorDiagnostics(clean); got != nil {
		t.Errorf("expected nil for clean report, got %v", got)
	}
}
