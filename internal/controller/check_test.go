package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

	fc := c.Check("/w", "/w/x.go", "package x\n\nvar y = 1 //gooze:ignore bogus\n")
	var messages []string
	for _, d := range fc.Diagnostics() {
		messages = append(messages, d.Message)
	}
	joined := strings.Join(messages, "\n")
	if !strings.HasPrefix(joined, "gooze: score 50.00% — 1 killed, 1 survived\n") || !strings.Contains(joined, "bogus") {
		t.Errorf("diagnostics = %q, want the report summary, then the directive error", joined)
	}
	if fc.Package != "." || fc.Rerun != "gooze run ." {
		t.Errorf("package = %q, rerun = %q", fc.Package, fc.Rerun)
	}
}

func TestCheckerConfigAndOtherFiles(t *testing.T) {
	c := Checker{Reports: fakeReports{}, Config: config.NewRegistry()}
	if diags := c.Check("/w", "/w/gooze.yaml", "run:r\n").Diagnostics(); len(diags) != 1 {
		t.Errorf("gooze.yaml diagnostics = %+v, want 1", diags)
	}
	if diags := c.Check("/w", "/w/README.md", "run:r\n").Diagnostics(); len(diags) != 0 {
		t.Errorf("README.md diagnostics = %+v, want none", diags)
	}
	if c.Handles("/w/README.md") || !c.Handles("/w/x.go") || !c.Handles("/w/gooze.yaml") {
		t.Error("Handles should accept only Go and gooze config files")
	}
}

// fakeFiles serves file contents from a map.
type fakeFiles map[string]string

func (f fakeFiles) ReadFile(name string) ([]byte, error) {
	if text, ok := f[name]; ok {
		return []byte(text), nil
	}
	return nil, errors.New("missing")
}

func TestCheckerTestChanged(t *testing.T) {
	src := "package x\n"
	sum := sha256.Sum256([]byte(src))
	rep := goozereport.Report{Source: goozereport.Source{
		Origin: &goozereport.File{ShortPath: "pkg/x.go", Hash: hex.EncodeToString(sum[:])},
		Test:   &goozereport.File{ShortPath: "pkg/x_test.go", Hash: "old"},
	}}
	c := Checker{Reports: fakeReports{reports: []goozereport.Report{rep}}, Config: config.NewRegistry(),
		Files: fakeFiles{"/w/pkg/x_test.go": "package x // new test\n"}, Cwd: "/w/sub"}

	fc := c.Check("/w", "/w/pkg/x.go", src)
	want := "gooze: tests changed since the last run (x_test.go); score then 100.00%; rerun: cd .. && gooze run ./pkg"
	if len(fc.Results) != 1 || fc.Results[0].Message != want {
		t.Errorf("results = %+v, want %q", fc.Results, want)
	}

	c.Files = nil
	if fc := c.Check("/w", "/w/pkg/x.go", src); fc.Results[0].Message != "gooze: score 100.00% — 0 killed" {
		t.Errorf("without a file reader tests are not compared; got %q", fc.Results[0].Message)
	}
}

func TestCheckerSourcesForTestEdges(t *testing.T) {
	c := Checker{Reports: fakeReports{}, Config: config.NewRegistry()}
	if got := c.SourcesForTest("/w", "/w/x.go"); got != nil {
		t.Errorf("a source file tests nothing, got %v", got)
	}
	failing := Checker{Reports: failingReports{}, Config: config.NewRegistry()}
	if got := failing.SourcesForTest("/w", "/w/x_test.go"); got != nil {
		t.Errorf("unreadable reports: got %v", got)
	}
}

func TestCheckerRerunFromAnotherDirectory(t *testing.T) {
	for cwd, want := range map[string]string{
		"":     "gooze run ./pkg",
		"/w":   "gooze run ./pkg",
		"/a":   "cd ../w && gooze run ./pkg", // sorts below the workdir
		"/z/y": "cd ../../w && gooze run ./pkg",
	} {
		if got := (Checker{Cwd: cwd}).rerun("/w", "./pkg"); got != want {
			t.Errorf("rerun from %q = %q, want %q", cwd, got, want)
		}
	}
}

// failingReports cannot read any reports directory.
type failingReports struct{ goozereport.Reader }

func (failingReports) ReadDir(string) ([]goozereport.Report, error) {
	return nil, errors.New("unreadable")
}

func TestCheckerPackageTested(t *testing.T) {
	rep := goozereport.Report{Source: goozereport.Source{Origin: &goozereport.File{ShortPath: "pkg/x.go"}}}
	c := Checker{Reports: fakeReports{reports: []goozereport.Report{rep}}, Config: config.NewRegistry()}
	if fc := c.Check("/w", "/w/pkg/new.go", "package pkg\n"); !fc.PackageTested || fc.Results != nil {
		t.Errorf("a new file of a tested package: %+v", fc)
	}
	if fc := c.Check("/w", "/w/other/y.go", "package other\n"); fc.PackageTested {
		t.Errorf("a file of an untested package: %+v", fc)
	}
}
