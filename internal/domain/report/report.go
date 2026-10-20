// Package report turns gooze mutation reports into editor diagnostics. It is
// pure domain logic over reports already read by gooze's report package and
// has no LSP or gopls dependency.
package report

import (
	"path/filepath"
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
		origin := r.Source.Origin
		if origin == nil {
			continue
		}

		if origin.FullPath == absPath || (origin.ShortPath != "" && filepath.Join(workdir, origin.ShortPath) == absPath) {
			matched = append(matched, r)
		}
	}

	return matched
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

// SurvivorDiagnostics is a best-effort mapping of surviving and not covered
// mutants to source lines by parsing each report's unified diff. gooze does not
// store per-mutant line positions, so these markers are approximate.
func SurvivorDiagnostics(reports []rpt.Report) []model.Diagnostic {
	var diags []model.Diagnostic

	seen := map[model.Diagnostic]bool{}

	for _, r := range reports {
		msg := survivorMessage(rpt.Summarize(r))
		if msg == "" || r.Diff == nil {
			continue
		}

		for _, line := range hunkLines(string(*r.Diff)) {
			d := model.Diagnostic{
				Range: model.Range{
					Start: model.Position{Line: line, Column: 0},
					End:   model.Position{Line: line, Column: 0},
				},
				Severity: model.SeverityWarning,
				Message:  msg,
				Source:   "gooze",
			}
			if !seen[d] {
				seen[d] = true
				diags = append(diags, d)
			}
		}
	}

	return diags
}

// survivorMessage describes the mutants of a report that tests missed, or ""
// when there are none.
func survivorMessage(s rpt.Summary) string {
	switch {
	case s.Survived > 0:
		return "gooze: possible surviving mutant near here (approximate)"
	case s.NotCovered > 0:
		return "gooze: possible mutant not covered by tests near here (approximate)"
	default:
		return ""
	}
}

var topOfFile = model.Range{
	Start: model.Position{Line: 0, Column: 0},
	End:   model.Position{Line: 0, Column: 0},
}

// hunkLines extracts zero-based new-file start lines from unified-diff hunk
// headers ("@@ -a,b +c,d @@").
func hunkLines(diff string) []int {
	var lines []int

	for l := range strings.SplitSeq(diff, "\n") {
		if !strings.HasPrefix(l, "@@") {
			continue
		}

		_, after, ok := strings.Cut(l, "+")
		if !ok {
			continue
		}

		num := after
		if comma := strings.IndexByte(num, ','); comma >= 0 {
			num = num[:comma]
		}

		if sp := strings.IndexByte(num, ' '); sp >= 0 {
			num = num[:sp]
		}

		if n, err := strconv.Atoi(num); err == nil && n > 0 {
			lines = append(lines, n-1)
		}
	}

	return lines
}
