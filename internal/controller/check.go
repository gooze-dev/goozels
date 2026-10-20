package controller

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"gooze.dev/pkg/gooze/pkg/config"
	goozereport "gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/internal/domain/goozeyml"
	"gooze.dev/pkg/goozels/internal/domain/ignore"
	"gooze.dev/pkg/goozels/internal/domain/report"
	"gooze.dev/pkg/goozels/internal/model"
)

// Checker computes the gooze diagnostics the language server would publish for a
// file, for callers outside an LSP session: the `goozels check` command and
// coding-agent hooks that run after an edit. The language server uses it too.
type Checker struct {
	Reports goozereport.Reader
	Config  config.Registry
	// Files reads test files from disk, to tell whether they changed since
	// gooze ran. Nil treats every recorded test as unchanged.
	Files FileReader
	// Cwd is the directory the reader of a rerun command is in. When it is set
	// and differs from the workdir, the command starts with a cd there.
	Cwd string
}

// IgnoreHint explains how to silence a mutant that cannot be killed, for one
// line under a list of survivors.
const IgnoreHint = "gooze: " + report.IgnoreHint

// FileCheck is what Check found for one file.
type FileCheck struct {
	// Edit holds problems in the file's own text: `//gooze:ignore`
	// directives in Go files, or a gooze config file's settings.
	Edit []model.Diagnostic
	// Results holds what gooze's reports say about a Go file: a summary (or
	// a notice that they are out of date) first, then survivor markers in
	// line order. It is nil when gooze has no results for the file.
	Results []model.Diagnostic
	// Package is the gooze path pattern of a Go file's package, relative to
	// the workdir, e.g. "./calc".
	Package string
	// Rerun is the command that refreshes a Go file's results.
	Rerun string
	// PackageTested reports whether gooze has results for any file of a Go
	// file's package, so a file without results was either added since the
	// run or had nothing to mutate.
	PackageTested bool
}

// Diagnostics returns every diagnostic of the check in line order, the
// results' summary first.
func (f FileCheck) Diagnostics() []model.Diagnostic {
	all := slices.Concat(f.Results, f.Edit)
	slices.SortStableFunc(all, func(a, b model.Diagnostic) int {
		return cmp.Compare(a.Range.Start.Line, b.Range.Start.Line)
	})

	return all
}

// Check returns gooze diagnostics for the file at absPath whose contents are
// text. workdir is the workspace root holding gooze's reports directory. Files
// that are neither Go sources nor gooze config files yield an empty check.
func (c Checker) Check(workdir, absPath, text string) FileCheck {
	switch {
	case c.Config.IsConfigFile(filepath.Base(absPath)):
		return FileCheck{Edit: goozeyml.New(c.Config).Diagnose(text)}
	case strings.HasSuffix(absPath, ".go"):
		pattern := command.PackagePattern(workdir, absPath)
		fc := FileCheck{Edit: ignore.Diagnose(text), Package: pattern, Rerun: c.rerun(workdir, pattern)}

		if all, err := c.Reports.ReadDir(filepath.Join(workdir, config.DefaultReportsDir)); err == nil {
			fc.Results = c.results(all, workdir, absPath, text, fc.Rerun)
			// Equivalent: a length is never negative, so != 0 is > 0.
			fc.PackageTested = len(report.Matching(all, workdir, pattern)) > 0 //gooze:ignore comparison
		}

		return fc
	default:
		return FileCheck{}
	}
}

// Handles reports whether Check produces diagnostics for files like absPath, so
// callers can skip reading files it would ignore.
func (c Checker) Handles(absPath string) bool {
	return strings.HasSuffix(absPath, ".go") || c.Config.IsConfigFile(filepath.Base(absPath))
}

// SourcesForTest returns the source files whose gooze reports were tested by
// the test file at absPath, so a caller that just edited a test can show the
// scores and surviving mutants of the code it tests.
func (c Checker) SourcesForTest(workdir, absPath string) []string {
	if !strings.HasSuffix(absPath, "_test.go") {
		return nil
	}

	all, err := c.Reports.ReadDir(filepath.Join(workdir, config.DefaultReportsDir))
	if err != nil {
		return nil
	}

	return report.SourcesForTest(all, workdir, absPath)
}

// results returns the report-derived diagnostics of a .go file from all the
// reports of workdir, or nil when none is about it.
func (c Checker) results(all []goozereport.Report, workdir, absPath, text, rerun string) []model.Diagnostic {
	cur := report.Current{Text: text, Rerun: rerun}

	if c.Files != nil {
		cur.Tests = map[string]string{}

		for _, test := range report.TestFiles(all, workdir, absPath) {
			if data, err := c.Files.ReadFile(test); err == nil {
				cur.Tests[test] = string(data)
			}
		}
	}

	return report.Diagnostics(all, workdir, absPath, cur)
}

// rerun is the command that reruns gooze for a pattern from c.Cwd.
func (c Checker) rerun(workdir, pattern string) string {
	line := command.RunCommandLine(pattern)
	// Equivalent: no string sorts below "", so <= "" is == "" (the workdir comparisons are killed by a test).
	if c.Cwd == "" || c.Cwd == workdir { //gooze:ignore comparison
		return line
	}

	dir := workdir
	if rel, err := filepath.Rel(c.Cwd, workdir); err == nil {
		dir = rel
	}

	return "cd " + dir + " && " + line
}
