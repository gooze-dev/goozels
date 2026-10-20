// Package report turns gooze mutation reports into editor diagnostics. It is
// pure domain logic over reports already read by gooze's report package and
// has no LSP or gopls dependency.
package report

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/domain/document"
	"gooze.dev/pkg/goozels/internal/model"
)

// Current is a source file as it is now, to compare with what gooze tested.
type Current struct {
	// Text is the source file's contents now.
	Text string
	// Tests holds the contents now of the files TestFiles names, by absolute
	// path. A missing entry means the file does not exist (or is unreadable);
	// a nil map skips comparing tests.
	Tests map[string]string
	// Rerun is the command that refreshes the results, e.g. "gooze run ./calc".
	Rerun string
}

// Diagnostics returns the diagnostics gooze's reports yield for the Go file at
// absPath, or nil when no report names it. The first is a top-of-file summary;
// when the file or its tests changed since gooze ran, it says so instead and
// gives the rerun command. Then comes one marker per line with surviving or
// uncovered mutants, in line order. Markers of a changed file are kept only
// where the line still reads as gooze tested it, so none points at the wrong
// line.
func Diagnostics(reports []rpt.Report, workdir, absPath string, cur Current) []model.Diagnostic {
	rep := ForFile(reports, workdir, absPath)
	if len(rep) == 0 {
		return nil
	}

	hash := hashOf(cur.Text)
	lines := document.Lines(cur.Text)
	keep := func(r rpt.Report, c change) bool {
		return sourceFresh(r, hash) || (c.line < len(lines) && lines[c.line] == c.text)
	}

	survivors := survivorDiagnostics(rep, keep)
	// Equivalent: only the slice's initial capacity changes.
	diags := make([]model.Diagnostic, 0, len(survivors)+1) //gooze:ignore arithmetic,numbers
	diags = append(diags, headDiagnostic(rep, workdir, absPath, cur))

	return append(diags, survivors...)
}

// headDiagnostic is the summary, or the stale notice when the source or its
// tests changed since the run.
func headDiagnostic(rep []rpt.Report, workdir, absPath string, cur Current) model.Diagnostic {
	d := SummaryDiagnostic(rep)

	switch state, changed := freshness(rep, workdir, absPath, cur); state {
	case SourceChanged:
		d.Message = staleMessage("results are from before "+filepath.Base(absPath)+" changed", rep, cur.Rerun)
		d.Severity = model.SeverityWarning
	case TestChanged:
		d.Message = staleMessage("tests changed since the last run ("+filepath.Base(changed)+")", rep, cur.Rerun)
		d.Severity = model.SeverityWarning
	case Fresh:
	}

	return d
}

// staleMessage says why results may be out of date, the score they gave and
// how to refresh them.
func staleMessage(why string, rep []rpt.Report, rerun string) string {
	msg := "gooze: " + why + "; score then " + percent(Summarize(rep))
	if rerun != "" {
		msg += "; rerun: " + rerun
	}

	return msg
}

// ForFile returns the reports whose origin is the Go file at absPath: its
// FullPath, or its ShortPath relative to workdir, names that file.
func ForFile(reports []rpt.Report, workdir, absPath string) []rpt.Report {
	var matched []rpt.Report

	for _, r := range reports {
		if names(r.Source.Origin, workdir, absPath) {
			matched = append(matched, r)
		}
	}

	return matched
}

// SourcesForTest returns, sorted and without duplicates, the source files whose
// reports record the test file at absPath as their test. Each source is
// resolved under workdir from its ShortPath, so reports made on another machine
// still point at local files.
func SourcesForTest(reports []rpt.Report, workdir, absPath string) []string {
	seen := map[string]bool{}

	for _, r := range reports {
		if r.Source.Origin != nil && names(r.Source.Test, workdir, absPath) {
			// Equivalent: only the map's keys are read.
			seen[resolve(r.Source.Origin, workdir)] = true //gooze:ignore boolean
		}
	}

	sources := make([]string, 0, len(seen))
	for src := range seen {
		sources = append(sources, src)
	}

	slices.Sort(sources)

	return sources
}

// Matching returns the reports whose origin lies in the packages a gooze path
// pattern selects, relative to workdir: "./..." is every report, "./pkg/..."
// pkg and the packages below it, "./pkg" only pkg.
func Matching(reports []rpt.Report, workdir, pattern string) []rpt.Report {
	dir, recursive := strings.CutSuffix(filepath.ToSlash(pattern), "/...")
	if dir == "..." {
		dir, recursive = ".", true
	}

	dir = filepath.Clean(filepath.Join(workdir, dir))

	var matched []rpt.Report

	for _, r := range reports {
		if r.Source.Origin == nil {
			continue
		}

		srcDir := filepath.Dir(resolve(r.Source.Origin, workdir))
		if srcDir == dir || (recursive && strings.HasPrefix(srcDir, dir+string(filepath.Separator))) {
			matched = append(matched, r)
		}
	}

	return matched
}

// names reports whether f is the file at absPath, by FullPath or by ShortPath
// relative to workdir.
func names(f *rpt.File, workdir, absPath string) bool {
	if f == nil {
		return false
	}

	return f.FullPath == absPath || (f.ShortPath != "" && filepath.Join(workdir, f.ShortPath) == absPath)
}

// resolve returns the local path of a report file: its ShortPath under
// workdir when recorded, otherwise its FullPath.
func resolve(f *rpt.File, workdir string) string {
	if f.ShortPath != "" {
		return filepath.Join(workdir, f.ShortPath)
	}

	return f.FullPath
}

// Summarize counts the mutation results of every report.
func Summarize(reports []rpt.Report) rpt.Summary {
	var s rpt.Summary

	for _, r := range reports {
		s.Merge(rpt.Summarize(r))
	}

	return s
}

// ScoreText leads with the mutation score, then counts the outcomes that
// occurred: "score 67.65% — 69 killed, 32 survived, 1 not covered by any test".
func ScoreText(s rpt.Summary) string {
	parts := []string{strconv.Itoa(s.Killed) + " killed"}

	for _, c := range []struct {
		n    int
		what string
	}{
		{s.Survived, "survived"},
		{s.NotCovered, "not covered by any test"},
		{s.Timeout, "timed out"},
		{s.Error, "errored"},
		{s.Skipped, "skipped"},
	} {
		// Equivalent: counts are never negative, so != 0 is > 0.
		if c.n > 0 { //gooze:ignore comparison
			parts = append(parts, strconv.Itoa(c.n)+" "+c.what)
		}
	}

	return "score " + percent(s) + " — " + strings.Join(parts, ", ")
}

// Message is a one-line human summary for an editor diagnostic.
func Message(s rpt.Summary) string {
	return "gooze: " + ScoreText(s)
}

func percent(s rpt.Summary) string {
	return strconv.FormatFloat(s.Score()*100, 'f', 2, 64) + "%"
}

// SummaryDiagnostic returns a top-of-file diagnostic summarizing the reports.
// Its severity is a warning when any mutant survived or was not covered,
// otherwise informational.
func SummaryDiagnostic(reports []rpt.Report) model.Diagnostic {
	s := Summarize(reports)
	severity := model.SeverityInfo

	if s.Survived+s.NotCovered > 0 {
		severity = model.SeverityWarning
	}

	return model.Diagnostic{
		Range:    topOfFile,
		Severity: severity,
		Message:  Message(s),
		Source:   source,
	}
}

// source labels every diagnostic derived from gooze reports.
const source = "gooze"

var topOfFile = model.Range{
	Start: model.Position{Line: 0, Column: 0},
	End:   model.Position{Line: 0, Column: 0},
}
