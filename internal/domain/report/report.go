// Package report models gooze mutation-testing reports and turns them into
// editor diagnostics. It is pure domain logic: it operates on already-parsed
// report values (the adapter layer reads the YAML files) and has no LSP or gopls
// dependency.
package report

import (
	"strconv"
	"strings"

	"gooze.dev/pkg/goozels/internal/model"
)

// Status is the outcome of testing a single mutation. The integer values match
// gooze's model.TestStatus serialization.
type Status int

const (
	// Killed means tests detected the mutation (good).
	Killed Status = iota
	// Survived means tests did not detect the mutation (a test gap).
	Survived
	// Skipped means the mutation was not tested.
	Skipped
	// Timeout means testing the mutation timed out.
	Timeout
	// Errored means an error occurred while testing the mutation.
	Errored
)

// Mutation is one mutation result within a file report.
type Mutation struct {
	ID     string
	Status Status
}

// FileReport is gooze's report for a single source file.
type FileReport struct {
	ShortPath string
	FullPath  string
	Mutations []Mutation
	Diff      []byte
}

// Summary aggregates a file report's outcomes.
type Summary struct {
	Killed   int
	Survived int
	Skipped  int
	Timeout  int
	Errored  int
	Total    int
}

// Summarize counts a file report's mutation outcomes.
func Summarize(r FileReport) Summary {
	var s Summary

	for _, mu := range r.Mutations {
		s.Total++

		switch mu.Status {
		case Killed:
			s.Killed++
		case Survived:
			s.Survived++
		case Skipped:
			s.Skipped++
		case Timeout:
			s.Timeout++
		case Errored:
			s.Errored++
		}
	}

	return s
}

// Score is the mutation score (killed / total) as a percentage, matching gooze's
// formula. An empty report scores 100.
func (s Summary) Score() int {
	if s.Total == 0 {
		return 100
	}

	return s.Killed * 100 / s.Total
}

// Message is a one-line human summary for an editor diagnostic.
func (s Summary) Message() string {
	return "gooze: " +
		strconv.Itoa(s.Survived) + "/" + strconv.Itoa(s.Total) + " mutants survived — score " +
		strconv.Itoa(s.Score()) + "%"
}

// SummaryDiagnostic returns a top-of-file diagnostic summarizing the report. Its
// severity is a warning when any mutant survived, otherwise informational.
func SummaryDiagnostic(r FileReport) model.Diagnostic {
	s := Summarize(r)
	severity := model.SeverityInfo

	if s.Survived > 0 {
		severity = model.SeverityWarning
	}

	return model.Diagnostic{
		Range:    topOfFile,
		Severity: severity,
		Message:  s.Message(),
		Source:   "gooze",
	}
}

// SurvivorDiagnostics is a best-effort mapping of surviving mutants to source
// lines by parsing the report's unified diff. gooze does not store per-mutant
// line positions, so these markers are approximate and only emitted when the
// report contains at least one survivor.
func SurvivorDiagnostics(r FileReport) []model.Diagnostic {
	if Summarize(r).Survived == 0 || len(r.Diff) == 0 {
		return nil
	}

	var diags []model.Diagnostic

	for _, line := range hunkLines(string(r.Diff)) {
		diags = append(diags, model.Diagnostic{
			Range: model.Range{
				Start: model.Position{Line: line, Column: 0},
				End:   model.Position{Line: line, Column: 0},
			},
			Severity: model.SeverityWarning,
			Message:  "gooze: possible surviving mutant near here (approximate)",
			Source:   "gooze",
		})
	}

	return diags
}

var topOfFile = model.Range{
	Start: model.Position{Line: 0, Column: 0},
	End:   model.Position{Line: 0, Column: 0},
}

// hunkLines extracts zero-based new-file start lines from unified-diff hunk
// headers ("@@ -a,b +c,d @@").
func hunkLines(diff string) []int {
	var lines []int

	for _, l := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(l, "@@") {
			continue
		}

		plus := strings.Index(l, "+")
		if plus < 0 {
			continue
		}

		num := l[plus+1:]
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
