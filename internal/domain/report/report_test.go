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

func TestSurvivorDiagnostics(t *testing.T) {
	reports := append(sample(),
		newReport("x.go", hunk, rpt.Survived),
		newReport("x.go", strings.Replace(hunk, "+10,3", "+20,3", 1), rpt.NotCovered),
		newReport("x.go", strings.Replace(hunk, "+10,3", "+30,3", 1), rpt.Timeout),
	)
	diags := SurvivorDiagnostics(reports)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %+v", diags)
	}
	if diags[0].Range.Start.Line != 9 || !strings.Contains(diags[0].Message, "surviving mutant") {
		t.Errorf("survivor diagnostic = %+v, want line 9", diags[0])
	}
	if diags[1].Range.Start.Line != 19 || !strings.Contains(diags[1].Message, "not covered") {
		t.Errorf("not-covered diagnostic = %+v, want line 19", diags[1])
	}

	clean := []rpt.Report{newReport("x.go", hunk, rpt.Killed)}
	if got := SurvivorDiagnostics(clean); got != nil {
		t.Errorf("expected nil for clean report, got %v", got)
	}
}
