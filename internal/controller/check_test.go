package controller

import (
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/domain/report"
)

// fakeReports serves one canned report for a single path.
type fakeReports struct {
	path string
	rep  report.FileReport
}

func (f fakeReports) ReportForFile(_, absPath string) (report.FileReport, bool) {
	return f.rep, absPath == f.path
}

func TestCheckerGoFile(t *testing.T) {
	c := Checker{Reports: fakeReports{path: "/w/x.go", rep: report.FileReport{
		Mutations: []report.Mutation{{ID: "a", Status: report.Killed}, {ID: "b", Status: report.Survived}},
	}}}

	diags := c.Check("/w", "/w/x.go", "package x\n\nvar y = 1 //gooze:ignore bogus\n")
	var messages []string
	for _, d := range diags {
		messages = append(messages, d.Message)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "bogus") || !strings.Contains(joined, "1/2 mutants survived") {
		t.Errorf("diagnostics = %q, want the directive error and the report summary", joined)
	}
}

func TestCheckerConfigAndOtherFiles(t *testing.T) {
	c := Checker{Reports: fakeReports{}}
	if diags := c.Check("/w", "/w/gooze.yaml", "run:r\n"); len(diags) != 1 {
		t.Errorf("gooze.yaml diagnostics = %+v, want 1", diags)
	}
	if diags := c.Check("/w", "/w/README.md", "run:r\n"); diags != nil {
		t.Errorf("README.md diagnostics = %+v, want nil", diags)
	}
	if c.Handles("/w/README.md") || !c.Handles("/w/x.go") || !c.Handles("/w/.gooze.yml") {
		t.Error("Handles should accept only Go and gooze config files")
	}
}
