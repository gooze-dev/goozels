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
	if msg != "gooze: score 50.00% — 2 killed, 1 survived, 1 skipped" {
		t.Errorf("message = %q", msg)
	}
}

func TestMessageNotCoveredAndError(t *testing.T) {
	reports := []rpt.Report{newReport("x.go", "", rpt.Killed, rpt.NotCovered, rpt.Timeout, rpt.Error, rpt.Error)}
	msg := Message(Summarize(reports))
	if msg != "gooze: score 33.33% — 1 killed, 1 not covered by any test, 1 timed out, 2 errored" {
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

func TestMessagePerfectScore(t *testing.T) {
	reports := []rpt.Report{newReport("x.go", "", rpt.Killed, rpt.Killed)}
	if msg := Message(Summarize(reports)); msg != "gooze: score 100.00% — 2 killed" {
		t.Errorf("message = %q", msg)
	}
}

func TestMatching(t *testing.T) {
	reports := []rpt.Report{
		newReport("x.go", "", rpt.Killed),
		newReport("pkg/a.go", "", rpt.Killed),
		newReport("pkg/sub/b.go", "", rpt.Killed),
		{},
	}
	count := func(pattern string) int { return len(Matching(reports, "/w", pattern)) }
	for pattern, want := range map[string]int{"./...": 3, "...": 3, ".": 1, "./pkg": 1, "./pkg/...": 2, "./pkg/sub": 1, "./other": 0} {
		if got := count(pattern); got != want {
			t.Errorf("Matching(%q) = %d reports, want %d", pattern, got, want)
		}
	}
}

// calcSource is the source text a report was made from, with a surviving
// mutant on its third line.
const calcSource = "package calc\n\nfunc f(n int) bool { return n > 0 }\n"

const calcDiff = "--- original\n+++ mutated\n@@ -2,2 +2,2 @@\n \n-func f(n int) bool { return n > 0 }\n+func f(n int) bool { return n >= 0 }\n"

func calcReport() rpt.Report {
	r := withName(newReport("calc.go", calcDiff, rpt.Survived), "comparison")
	r.Source.Origin.Hash = hashOf(calcSource)
	r.Source.Test = &rpt.File{ShortPath: "calc_test.go", Hash: hashOf("package calc\n")}
	return r
}

func messages(diags []model.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Message)
	}
	return out
}

func TestDiagnosticsFresh(t *testing.T) {
	cur := Current{Text: calcSource, Tests: map[string]string{"/w/calc_test.go": "package calc\n"}, Rerun: "gooze run ."}
	got := messages(Diagnostics([]rpt.Report{calcReport()}, "/w", "/w/calc.go", cur))
	want := []string{
		"gooze: score 0.00% — 0 killed, 1 survived",
		"gooze: surviving comparison mutant — tests still pass with `func f(n int) bool { return n > 0 }` → `func f(n int) bool { return n >= 0 }`",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if Diagnostics([]rpt.Report{calcReport()}, "/w", "/w/other.go", cur) != nil {
		t.Error("a file without reports should yield nil")
	}
}

func TestDiagnosticsSourceChanged(t *testing.T) {
	tests := map[string]string{"/w/calc_test.go": "package calc\n"}

	shifted := Current{Text: "package calc\n\n// doc\nfunc f(n int) bool { return n > 0 }\n", Tests: tests, Rerun: "gooze run ."}
	got := messages(Diagnostics([]rpt.Report{calcReport()}, "/w", "/w/calc.go", shifted))
	want := "gooze: results are from before calc.go changed; score then 0.00%; rerun: gooze run ."
	if len(got) != 1 || got[0] != want {
		t.Errorf("a moved line must not be marked; got %q, want only %q", got, want)
	}

	tail := Current{Text: calcSource + "\nfunc g() {}\n", Tests: tests}
	got = messages(Diagnostics([]rpt.Report{calcReport()}, "/w", "/w/calc.go", tail))
	if len(got) != 2 || got[0] != "gooze: results are from before calc.go changed; score then 0.00%" {
		t.Errorf("a line that still reads as tested keeps its marker; got %q", got)
	}
}

func TestDiagnosticsTestChanged(t *testing.T) {
	cur := Current{Text: calcSource, Tests: map[string]string{"/w/calc_test.go": "package calc // edited\n"}, Rerun: "gooze run ."}
	got := messages(Diagnostics([]rpt.Report{calcReport()}, "/w", "/w/calc.go", cur))
	if len(got) != 2 || got[0] != "gooze: tests changed since the last run (calc_test.go); score then 0.00%; rerun: gooze run ." {
		t.Errorf("diagnostics = %q", got)
	}
}

func TestNamesAndResolveNeedAShortPath(t *testing.T) {
	r := rpt.Report{Source: rpt.Source{Origin: &rpt.File{FullPath: "/build/x.go"}, Test: &rpt.File{FullPath: "/build/x_test.go"}}}
	if got := ForFile([]rpt.Report{r}, "/w", "/w"); got != nil {
		t.Errorf("an empty ShortPath must not name the workdir, got %+v", got)
	}
	if got := ForFile([]rpt.Report{r}, "/w", "/a.go"); got != nil {
		t.Errorf("FullPath must match exactly, got %+v", got)
	}
	if got := SourcesForTest([]rpt.Report{r}, "/w", "/build/x_test.go"); strings.Join(got, ",") != "/build/x.go" {
		t.Errorf("without a ShortPath the source is its FullPath, got %v", got)
	}
}

func TestDiagnosticsSummaryAtTopOfFile(t *testing.T) {
	diags := Diagnostics([]rpt.Report{calcReport()}, "/w", "/w/calc.go", Current{Text: calcSource})
	if diags[0].Range != (model.Range{}) {
		t.Errorf("summary range = %+v, want the start of the file", diags[0].Range)
	}
}

func TestDiagnosticsStaleKeepsOnlyMatchingLines(t *testing.T) {
	r := calcReport()
	for name, text := range map[string]string{
		"line replaced by greater text": "package calc\n\nzzz\n",
		"file now shorter":              "package calc\n",
		"file now much shorter":         "x",
	} {
		got := messages(Diagnostics([]rpt.Report{r}, "/w", "/w/calc.go", Current{Text: text}))
		if len(got) != 1 {
			t.Errorf("%s: got %q, want only the stale notice", name, got)
		}
	}
}
