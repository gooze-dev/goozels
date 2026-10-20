package report

import (
	"strings"
	"testing"

	rpt "gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/model"
)

const hunk = "--- original\n+++ mutated\n@@ -10,3 +10,3 @@\n-\ta + b\n+\ta - b\n"

func newReport(short string, diff string, statuses ...rpt.TestStatus) rpt.Report {
	r := rpt.Report{
		Source: rpt.Source{Origin: &rpt.File{ShortPath: short, FullPath: "/build/" + short}},
		Result: []rpt.Result{{Name: "arithmetic", Version: 1}},
	}
	for _, s := range statuses {
		r.Result[0].Mutations = append(r.Result[0].Mutations, rpt.MutationResult{MutationID: "m", Status: s})
	}
	if diff != "" {
		d := []byte(diff)
		r.Diff = &d
	}
	return r
}

func sample() []rpt.Report {
	return []rpt.Report{
		newReport("x.go", "", rpt.Killed),
		newReport("x.go", hunk, rpt.Survived),
		newReport("x.go", "", rpt.Killed),
		newReport("x.go", "", rpt.Skipped),
	}
}

func TestForFile(t *testing.T) {
	reports := []rpt.Report{
		newReport("pkg/x.go", "", rpt.Killed),
		newReport("pkg/y.go", "", rpt.Killed),
		newReport("other/pkg/x.go", "", rpt.Killed),
		{},
	}

	if got := ForFile(reports, "/proj", "/proj/pkg/x.go"); len(got) != 1 || got[0].Source.Origin.ShortPath != "pkg/x.go" {
		t.Errorf("ShortPath match = %+v", got)
	}
	if got := ForFile(reports, "/elsewhere", "/build/pkg/y.go"); len(got) != 1 || got[0].Source.Origin.ShortPath != "pkg/y.go" {
		t.Errorf("FullPath match = %+v", got)
	}
	if got := ForFile(reports, "/proj", "/proj/x.go"); got != nil {
		t.Errorf("unrelated file matched %+v", got)
	}
}

func TestMessage(t *testing.T) {
	msg := Message(Summarize(sample()))
	if msg != "gooze: 1/4 mutants survived — score 50.00%" {
		t.Errorf("message = %q", msg)
	}
}

func TestMessageNotCoveredAndError(t *testing.T) {
	reports := []rpt.Report{newReport("x.go", "", rpt.Killed, rpt.NotCovered, rpt.Error, rpt.Error)}
	msg := Message(Summarize(reports))
	if msg != "gooze: 0/4 mutants survived, 1 not covered — score 50.00%" {
		t.Errorf("message = %q", msg)
	}
}

func TestSummaryDiagnosticSeverity(t *testing.T) {
	if d := SummaryDiagnostic(sample()); d.Severity != model.SeverityWarning || d.Source != "gooze" {
		t.Errorf("diagnostic with survivors = %+v, want gooze warning", d)
	}
	notCovered := []rpt.Report{newReport("x.go", "", rpt.Killed, rpt.NotCovered)}
	if d := SummaryDiagnostic(notCovered); d.Severity != model.SeverityWarning {
		t.Errorf("severity with not covered = %v, want warning", d.Severity)
	}
	clean := []rpt.Report{newReport("x.go", "", rpt.Killed, rpt.Error, rpt.Timeout)}
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

func withName(r rpt.Report, name string) rpt.Report {
	r.Result[0].Name = name
	return r
}

func TestSurvivorDiagnosticsMarksMutatedLine(t *testing.T) {
	reports := []rpt.Report{
		withName(newReport("x.go", realDiff, rpt.Survived), "comparison"),
		withName(newReport("x.go", realDiff, rpt.Killed), "comparison"),
	}

	diags := SurvivorDiagnostics(reports)
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
	reports := []rpt.Report{
		withName(newReport("x.go", removal, rpt.NotCovered), "statement"),
		newReport("x.go", hunk, rpt.Timeout),
	}

	diags := SurvivorDiagnostics(reports)
	if len(diags) != 1 || diags[0].Range.Start.Line != 3 {
		t.Fatalf("diags = %+v, want one marker on 0-based line 3 (timeouts are not marked)", diags)
	}
	if !strings.Contains(diags[0].Message, "not covered by any test") || !strings.Contains(diags[0].Message, "line removed") {
		t.Errorf("message = %q", diags[0].Message)
	}

	clean := []rpt.Report{newReport("x.go", hunk, rpt.Killed)}
	if got := SurvivorDiagnostics(clean); got != nil {
		t.Errorf("expected nil for clean report, got %v", got)
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

func TestSourcesForTest(t *testing.T) {
	withTest := func(src, test string) rpt.Report {
		r := newReport(src, "", rpt.Survived)
		r.Source.Test = &rpt.File{ShortPath: test, FullPath: "/build/" + test}
		return r
	}
	reports := []rpt.Report{
		withTest("calc/calc.go", "calc/calc_test.go"),
		withTest("calc/calc.go", "calc/calc_test.go"),
		withTest("calc/round.go", "calc/calc_test.go"),
		withTest("other/x.go", "other/x_test.go"),
		newReport("calc/nocase.go", "", rpt.Killed), // no test recorded
	}

	got := SourcesForTest(reports, "/w", "/w/calc/calc_test.go")
	want := []string{"/w/calc/calc.go", "/w/calc/round.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("SourcesForTest = %v, want %v", got, want)
	}
	if got := SourcesForTest(reports, "/w", "/w/calc/calc.go"); len(got) != 0 {
		t.Errorf("a source file is not a test: got %v", got)
	}
}
