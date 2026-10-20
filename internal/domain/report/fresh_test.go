package report

import (
	"strings"
	"testing"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestHashOfMatchesGooze(t *testing.T) {
	// gooze records the hex SHA-256 of the file bytes.
	if got := hashOf("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("hashOf = %s", got)
	}
}

func TestTestFiles(t *testing.T) {
	untested := newReport("calc/other.go", "", rpt.Killed)
	got := TestFiles([]rpt.Report{calcReport(), calcReport(), untested}, "/w", "/w/calc.go")
	if strings.Join(got, ",") != "/w/calc_test.go" {
		t.Errorf("TestFiles = %v", got)
	}
	got = TestFiles([]rpt.Report{untested}, "/w", "/w/calc/other.go")
	if strings.Join(got, ",") != "/w/calc/other_test.go" {
		t.Errorf("TestFiles without a recorded test = %v, want the conventional test file", got)
	}
}

func TestFreshness(t *testing.T) {
	tests := map[string]string{"/w/calc_test.go": "package calc\n"}
	cur := Current{Text: calcSource, Tests: tests}
	reports := []rpt.Report{calcReport()}

	if state, _ := freshness(reports, "/w", "/w/calc.go", cur); state != Fresh {
		t.Errorf("unchanged files: %v, want Fresh", state)
	}
	if state, _ := freshness(reports, "/w", "/w/calc.go", Current{Text: "changed", Tests: tests}); state != SourceChanged {
		t.Errorf("edited source: %v, want SourceChanged", state)
	}
	if state, _ := freshness(reports, "/w", "/w/calc.go", Current{Text: calcSource}); state != Fresh {
		t.Errorf("tests not compared: %v, want Fresh", state)
	}
	if state, test := freshness(reports, "/w", "/w/calc.go", Current{Text: calcSource, Tests: map[string]string{}}); state != TestChanged || test != "/w/calc_test.go" {
		t.Errorf("deleted test: %v %q, want TestChanged for calc_test.go", state, test)
	}

	noHash := newReport("calc.go", "", rpt.Killed)
	if state, _ := freshness([]rpt.Report{noHash}, "/w", "/w/calc.go", Current{Text: "x"}); state != Fresh {
		t.Errorf("reports without hashes: %v, want Fresh", state)
	}
	added := Current{Text: "x", Tests: map[string]string{"/w/calc_test.go": ""}}
	if state, _ := freshness([]rpt.Report{noHash}, "/w", "/w/calc.go", added); state != TestChanged {
		t.Errorf("test added after the run: %v, want TestChanged", state)
	}

	untracked := calcReport()
	untracked.Source.Test.Hash = ""
	if state, _ := freshness([]rpt.Report{untracked}, "/w", "/w/calc.go", Current{Text: calcSource, Tests: map[string]string{}}); state != Fresh {
		t.Errorf("test without a hash: %v, want Fresh", state)
	}
}

func TestFreshnessReportsFirstChangedTest(t *testing.T) {
	a, b := calcReport(), calcReport()
	b.Source.Test = &rpt.File{ShortPath: "other_test.go", Hash: "old"}
	cur := Current{Text: calcSource, Tests: map[string]string{}}
	if _, test := freshness([]rpt.Report{a, b}, "/w", "/w/calc.go", cur); test != "/w/calc_test.go" {
		t.Errorf("changed test = %q, want the first report's", test)
	}
}

func TestFreshnessComparesHashesExactly(t *testing.T) {
	for _, stored := range []string{"0000", "ffff"} { // below and above any real hash
		r := calcReport()
		r.Source.Origin.Hash = stored
		if state, _ := freshness([]rpt.Report{r}, "/w", "/w/calc.go", Current{Text: calcSource}); state != SourceChanged {
			t.Errorf("source hash %s: %v, want SourceChanged", stored, state)
		}
		r = calcReport()
		r.Source.Test.Hash = stored
		cur := Current{Text: calcSource, Tests: map[string]string{"/w/calc_test.go": "package calc\n"}}
		if state, _ := freshness([]rpt.Report{r}, "/w", "/w/calc.go", cur); state != TestChanged {
			t.Errorf("test hash %s: %v, want TestChanged", stored, state)
		}
	}
}
