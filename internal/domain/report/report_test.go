package report

import (
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/model"
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

// realDiff is a unified diff as gooze stores it for a mutant: three lines of
// context, so the hunk starts three lines before the mutated line. gooze writes
// blank context lines as " "; editors strip that trailing space here, and
// diffChanges treats both forms alike.
const realDiff = `--- original
+++ mutated
@@ -205,7 +205,7 @@
 }

 func isLeadingComment(line int, slashOffset int, lineStarts []int, content []byte) bool {
-	if line <= 0 || line > len(lineStarts) {
+	if line == 0 || line > len(lineStarts) {
 		return false
 	}

`

func TestSurvivorDiagnosticsMarksMutatedLine(t *testing.T) {
	r := FileReport{Mutations: []Mutation{
		{ID: "a", Mutagen: "comparison", Status: Survived, Diff: []byte(realDiff)},
		{ID: "b", Mutagen: "comparison", Status: Killed, Diff: []byte(realDiff)},
	}}

	diags := SurvivorDiagnostics(r)
	if len(diags) != 1 {
		t.Fatalf("expected 1 survivor diagnostic (killed mutants are not marked), got %d", len(diags))
	}

	d := diags[0]
	if d.Range.Start.Line != 207 { // the "-" line is 1-based 208, not the hunk start 205
		t.Errorf("line = %d, want 207", d.Range.Start.Line)
	}
	if d.Range.Start.Column != 1 || d.Range.End.Column != len("\tif line <= 0 || line > len(lineStarts) {") {
		t.Errorf("columns = %d..%d, want the code after the tab", d.Range.Start.Column, d.Range.End.Column)
	}
	want := "gooze: surviving comparison mutant — tests still pass with: if line == 0 || line > len(lineStarts) {"
	if d.Message != want {
		t.Errorf("message = %q\nwant      %q", d.Message, want)
	}
}

func TestSurvivorDiagnosticsNotCoveredAndRemoval(t *testing.T) {
	removal := "--- original\n+++ mutated\n@@ -3,3 +3,2 @@\n a()\n-\tb()\n c()\n"
	r := FileReport{Mutations: []Mutation{{ID: "a", Mutagen: "statement", Status: NotCovered, Diff: []byte(removal)}}}

	diags := SurvivorDiagnostics(r)
	if len(diags) != 1 || diags[0].Range.Start.Line != 3 {
		t.Fatalf("diags = %+v, want one marker on 0-based line 3", diags)
	}
	if !strings.Contains(diags[0].Message, "not covered by any test") || !strings.Contains(diags[0].Message, "line removed") {
		t.Errorf("message = %q", diags[0].Message)
	}
}

func TestDiffChangesMultipleHunksAndBlankContext(t *testing.T) {
	diff := "--- original\n+++ mutated\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n\n@@ -10,2 +10,2 @@\n-x\n+X\n y\n\\ No newline at end of file\n"
	got := diffChanges(diff)
	if len(got) != 2 || got[0].line != 1 || got[0].mutated != "B" || got[1].line != 9 || got[1].mutated != "X" {
		t.Errorf("changes = %+v", got)
	}
	if diffChanges("") != nil || diffChanges("@@ garbage @@\n-x\n") != nil {
		t.Error("empty or malformed diffs should yield no changes")
	}
}

func TestScoreExcludesErroredAndCountsNotCovered(t *testing.T) {
	s := Summarize(FileReport{Mutations: []Mutation{
		{Status: Killed}, {Status: Killed}, {Status: Survived}, {Status: NotCovered}, {Status: Errored},
	}})
	if s.NotCovered != 1 || s.Errored != 1 || s.Total != 5 {
		t.Fatalf("summary = %+v", s)
	}
	if s.Score() != 50 { // gooze: killed / (total - errored) = 2 / 4
		t.Errorf("score = %d, want 50", s.Score())
	}
	if msg := s.Message(); !strings.Contains(msg, "1/5 mutants survived, 1 not covered by any test") {
		t.Errorf("message = %q", msg)
	}
	if d := SummaryDiagnostic(FileReport{Mutations: []Mutation{{Status: NotCovered}}}); d.Severity != model.SeverityWarning {
		t.Error("uncovered mutants should make the summary a warning")
	}
}
