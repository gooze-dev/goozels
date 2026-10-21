package report

import (
	"testing"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// summary returns the summary message gooze's reports give calc.go.
func summary(t *testing.T, cur Current, reports ...rpt.Report) string {
	t.Helper()

	results, ok := FileResults(reports, "/w", "/w/calc.go", cur)
	if !ok {
		t.Fatal("no results for calc.go")
	}

	return results.Summary.Message
}

func withScope(r rpt.Report, scope *rpt.Scope) rpt.Report {
	r.Scope = scope
	return r
}

var freshCalc = Current{Text: calcSource, Tests: map[string]string{"/w/calc_test.go": "package calc\n"}, Rerun: "gooze run ."}

func TestSummaryOfAPartlyTestedFile(t *testing.T) {
	cases := map[string]struct {
		scope *rpt.Scope
		want  string
	}{
		"functions": {&rpt.Scope{Run: "^Add$"},
			"gooze: partly tested (functions matching ^Add$) — score 0.00% — 0 killed, 1 survived; rerun the whole file: gooze run ."},
		"mutagens": {&rpt.Scope{Mutagens: []string{"comparison", "logical"}},
			"gooze: partly tested (mutagens comparison, logical) — score 0.00% — 0 killed, 1 survived; rerun the whole file: gooze run ."},
		"changed lines": {&rpt.Scope{Base: "1a2b3c4d5e6f", Lines: []rpt.LineRange{{Start: 3, End: 3}}},
			"gooze: partly tested (changed lines since 1a2b3c4) — score 0.00% — 0 killed, 1 survived; rerun the whole file: gooze run ."},
		"several selections": {&rpt.Scope{Base: "abc", Lines: []rpt.LineRange{{Start: 3, End: 3}}, Run: "^f$", Mutagens: []string{"comparison"}},
			"gooze: partly tested (changed lines since abc; functions matching ^f$; mutagens comparison) — score 0.00% — 0 killed, 1 survived; rerun the whole file: gooze run ."},
		"empty scope still partial": {&rpt.Scope{},
			"gooze: partly tested — score 0.00% — 0 killed, 1 survived; rerun the whole file: gooze run ."},
	}

	for name, tc := range cases {
		if got := summary(t, freshCalc, withScope(calcReport(), tc.scope)); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, tc.want)
		}
	}
}

func TestSummaryOfDifferentScopes(t *testing.T) {
	a := withScope(calcReport(), &rpt.Scope{Run: "^f$"})
	b := withScope(withName(newReport("calc.go", calcDiff, rpt.Killed), "arithmetic"), &rpt.Scope{Mutagens: []string{"arithmetic"}})
	b.Source = a.Source

	want := "gooze: partly tested — score 50.00% — 1 killed, 1 survived; rerun the whole file: gooze run ."
	if got := summary(t, freshCalc, a, b); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}

	// The same scope on several reports is named once.
	c := withScope(withName(newReport("calc.go", calcDiff, rpt.Killed), "arithmetic"), &rpt.Scope{Run: "^f$"})
	c.Source = a.Source

	want = "gooze: partly tested (functions matching ^f$) — score 50.00% — 1 killed, 1 survived; rerun the whole file: gooze run ."
	if got := summary(t, freshCalc, a, c); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestSummaryOfAFileWithSomePartialReports(t *testing.T) {
	full := withName(newReport("calc.go", calcDiff, rpt.Killed), "arithmetic")
	partial := withScope(calcReport(), &rpt.Scope{Run: "^f$"})
	full.Source = partial.Source

	want := "gooze: partly tested (functions matching ^f$) — score 50.00% — 1 killed, 1 survived; rerun the whole file: gooze run ."
	if got := summary(t, freshCalc, full, partial); got != want {
		t.Errorf("any partial report makes the file partly tested:\n got %q\nwant %q", got, want)
	}
}

func TestSummaryOfAPartlyTestedFileWithoutRerun(t *testing.T) {
	cur := freshCalc
	cur.Rerun = ""

	want := "gooze: partly tested (functions matching ^f$) — score 0.00% — 0 killed, 1 survived"
	if got := summary(t, cur, withScope(calcReport(), &rpt.Scope{Run: "^f$"})); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestStaleSummaryOfAPartlyTestedFile(t *testing.T) {
	changed := Current{Text: calcSource + "// edit\n", Tests: freshCalc.Tests, Rerun: "gooze run ."}

	want := "gooze: partly tested (functions matching ^f$); results are from before calc.go changed; score then 0.00%; rerun: gooze run ."
	if got := summary(t, changed, withScope(calcReport(), &rpt.Scope{Run: "^f$"})); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
