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

	"gooze.dev/pkg/goozels/internal/model"
)

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
		origin := r.Source.Origin
		if origin == nil || !names(r.Source.Test, workdir, absPath) {
			continue
		}

		src := origin.FullPath
		if origin.ShortPath != "" {
			src = filepath.Join(workdir, origin.ShortPath)
		}

		seen[src] = true
	}

	sources := make([]string, 0, len(seen))
	for src := range seen {
		sources = append(sources, src)
	}

	slices.Sort(sources)

	return sources
}

// names reports whether f is the file at absPath, by FullPath or by ShortPath
// relative to workdir.
func names(f *rpt.File, workdir, absPath string) bool {
	if f == nil {
		return false
	}

	return f.FullPath == absPath || (f.ShortPath != "" && filepath.Join(workdir, f.ShortPath) == absPath)
}

// Summarize counts the mutation results of every report.
func Summarize(reports []rpt.Report) rpt.Summary {
	var s rpt.Summary

	for _, r := range reports {
		s.Merge(rpt.Summarize(r))
	}

	return s
}

// Message is a one-line human summary for an editor diagnostic.
func Message(s rpt.Summary) string {
	msg := "gooze: " + strconv.Itoa(s.Survived) + "/" + strconv.Itoa(s.Total) + " mutants survived"
	if s.NotCovered > 0 {
		msg += ", " + strconv.Itoa(s.NotCovered) + " not covered"
	}

	return msg + " — score " + strconv.FormatFloat(s.Score()*100, 'f', 2, 64) + "%"
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
		Source:   "gooze",
	}
}

// SurvivorDiagnostics marks the original source line each surviving or
// uncovered mutant changed, read from the report's unified diff, and says which
// change the tests missed. Positions refer to the source as gooze tested it, so
// they drift if the file has been edited since the run.
func SurvivorDiagnostics(reports []rpt.Report) []model.Diagnostic {
	var diags []model.Diagnostic

	seen := map[model.Diagnostic]bool{}

	for _, r := range reports {
		for _, d := range reportSurvivors(r) {
			if !seen[d] {
				seen[d] = true

				diags = append(diags, d)
			}
		}
	}

	return diags
}

// reportSurvivors marks the changed lines of one report for each of its
// surviving or uncovered mutations.
func reportSurvivors(r rpt.Report) []model.Diagnostic {
	if r.Diff == nil {
		return nil
	}

	var (
		diags   []model.Diagnostic
		changes = diffChanges(string(*r.Diff))
	)

	for _, res := range r.Result {
		for _, mu := range res.Mutations {
			if mu.Status != rpt.Survived && mu.Status != rpt.NotCovered {
				continue
			}

			for _, c := range changes {
				diags = append(diags, survivorDiagnostic(res.Name, mu.Status, c))
			}
		}
	}

	return diags
}

func survivorDiagnostic(mutagen string, status rpt.TestStatus, c change) model.Diagnostic {
	return model.Diagnostic{
		Range: model.Range{
			Start: model.Position{Line: c.line, Column: c.indent},
			End:   model.Position{Line: c.line, Column: c.end},
		},
		Severity: model.SeverityWarning,
		Message:  survivorMessage(mutagen, status, c.mutated),
		Source:   "gooze",
	}
}

// survivorMessage tells the reader which change the tests failed to detect.
func survivorMessage(mutagen string, status rpt.TestStatus, mutated string) string {
	name := "mutant"
	if mutagen != "" {
		name = mutagen + " mutant"
	}

	edit := "with this line removed"
	if mutated != "" {
		edit = "with: " + mutated
	}

	if status == rpt.NotCovered {
		return "gooze: " + name + " not covered by any test — nothing fails " + edit
	}

	return "gooze: surviving " + name + " — tests still pass " + edit
}

var topOfFile = model.Range{
	Start: model.Position{Line: 0, Column: 0},
	End:   model.Position{Line: 0, Column: 0},
}

// change is one original line a mutation replaced or removed.
type change struct {
	line    int    // zero-based line in the original source
	indent  int    // byte column where the line's code starts
	end     int    // byte column where the line ends
	mutated string // replacement text, trimmed; empty when the line was removed
}

// diffChanges extracts the original lines a unified diff ("--- original",
// "+++ mutated", "@@ -a,b +c,d @@" hunks) removes, pairing each with the added
// line that replaced it. gooze's hunks carry three lines of context, so the
// changed line is not the hunk's first line.
func diffChanges(diff string) []change {
	var (
		changes []change
		old     = -1 // next original line (one-based); -1 until the first hunk
		pending = 0  // index into changes of the first unpaired removal
	)

	for l := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			old = hunkOldStart(l)
			pending = len(changes)
		case old < 0 || strings.HasPrefix(l, "\\"):
			// File headers before the first hunk and "\ No newline at end of file".
		case strings.HasPrefix(l, "-"):
			changes = append(changes, removedLine(old-1, l[1:]))
			old++
		case strings.HasPrefix(l, "+"):
			if pending < len(changes) {
				changes[pending].mutated = strings.TrimSpace(l[1:])
				pending++
			}
		default: // context line (" text", or "" for a blank line)
			old++
			pending = len(changes)
		}
	}

	return changes
}

func removedLine(line int, text string) change {
	indent := len(text) - len(strings.TrimLeft(text, " \t"))

	return change{line: max(line, 0), indent: indent, end: len(text)}
}

// hunkOldStart returns the one-based original start line from a hunk header
// ("@@ -a,b +c,d @@"), or -1 when the header is malformed.
func hunkOldStart(header string) int {
	_, after, ok := strings.Cut(header, "-")
	if !ok {
		return -1
	}

	num, _, _ := strings.Cut(after, " ")
	num, _, _ = strings.Cut(num, ",")

	n, err := strconv.Atoi(num)
	if err != nil || n < 1 {
		return -1
	}

	return n
}
