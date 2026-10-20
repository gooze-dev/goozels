package controller

import (
	"strings"
	"testing"

	"gooze.dev/pkg/gooze/pkg/config"
	goozereport "gooze.dev/pkg/gooze/pkg/report"
)

// fakeReports serves canned reports for any directory.
type fakeReports struct {
	goozereport.Reader
	reports []goozereport.Report
}

func (f fakeReports) ReadDir(string) ([]goozereport.Report, error) { return f.reports, nil }

func TestCheckerGoFile(t *testing.T) {
	origin := &goozereport.File{ShortPath: "x.go", FullPath: "/w/x.go"}
	reportWith := func(s goozereport.TestStatus) goozereport.Report {
		return goozereport.Report{
			Source: goozereport.Source{Origin: origin},
			Result: []goozereport.Result{{Name: "arithmetic", Mutations: []goozereport.MutationResult{{MutationID: "m", Status: s}}}},
		}
	}
	c := Checker{
		Reports: fakeReports{reports: []goozereport.Report{reportWith(goozereport.Killed), reportWith(goozereport.Survived)}},
		Config:  config.NewRegistry(),
	}

	var messages []string
	for _, d := range c.Check("/w", "/w/x.go", "package x\n\nvar y = 1 //gooze:ignore bogus\n") {
		messages = append(messages, d.Message)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "bogus") || !strings.Contains(joined, "1/2 mutants survived") {
		t.Errorf("diagnostics = %q, want the directive error and the report summary", joined)
	}
}

func TestCheckerConfigAndOtherFiles(t *testing.T) {
	c := Checker{Reports: fakeReports{}, Config: config.NewRegistry()}
	if diags := c.Check("/w", "/w/gooze.yaml", "run:r\n"); len(diags) != 1 {
		t.Errorf("gooze.yaml diagnostics = %+v, want 1", diags)
	}
	if diags := c.Check("/w", "/w/README.md", "run:r\n"); diags != nil {
		t.Errorf("README.md diagnostics = %+v, want nil", diags)
	}
	if c.Handles("/w/README.md") || !c.Handles("/w/x.go") || !c.Handles("/w/gooze.yaml") {
		t.Error("Handles should accept only Go and gooze config files")
	}
}
