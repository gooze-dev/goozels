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
	// NotCovered means no test exercises the mutated line, so the mutant
	// survives without any test running.
	NotCovered
)

// Mutation is one mutation result within a file report.
type Mutation struct {
	ID      string
	Mutagen string
	Status  Status
	// Diff is gooze's unified diff of the mutated source against the original.
	// gooze stores it only for mutations that were not killed.
	Diff []byte
}

// FileReport is the merged set of gooze results for a single source file. gooze
// writes one report per mutation; the adapter groups them by source file.
type FileReport struct {
	ShortPath string
	FullPath  string
	Mutations []Mutation
}

// Summary aggregates a file report's outcomes.
type Summary struct {
	Killed     int
	Survived   int
	Skipped    int
	Timeout    int
	Errored    int
	NotCovered int
	Total      int
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
		case NotCovered:
			s.NotCovered++
		}
	}

	return s
}

// Score is the mutation score as a percentage, matching gooze's formula: killed
// over all mutations except errored ones. A report with nothing scored is 100.
func (s Summary) Score() int {
	scored := s.Total - s.Errored
	if scored <= 0 {
		return 100
	}

	return s.Killed * 100 / scored
}

// Message is a one-line human summary for an editor diagnostic.
func (s Summary) Message() string {
	msg := "gooze: " + strconv.Itoa(s.Survived) + "/" + strconv.Itoa(s.Total) + " mutants survived"
	if s.NotCovered > 0 {
		msg += ", " + strconv.Itoa(s.NotCovered) + " not covered by any test"
	}

	return msg + " — score " + strconv.Itoa(s.Score()) + "%"
}

// SummaryDiagnostic returns a top-of-file diagnostic summarizing the report. Its
// severity is a warning when any mutant survived, otherwise informational.
func SummaryDiagnostic(r FileReport) model.Diagnostic {
	s := Summarize(r)
	severity := model.SeverityInfo

	if s.Survived > 0 || s.NotCovered > 0 {
		severity = model.SeverityWarning
	}

	return model.Diagnostic{
		Range:    topOfFile,
		Severity: severity,
		Message:  s.Message(),
		Source:   "gooze",
	}
}

// SurvivorDiagnostics marks the original source line each surviving or
// uncovered mutant changed, read from the mutant's unified diff. Positions refer to the source as
// gooze tested it, so they drift if the file has been edited since the run.
func SurvivorDiagnostics(r FileReport) []model.Diagnostic {
	var diags []model.Diagnostic

	for _, mu := range r.Mutations {
		if mu.Status != Survived && mu.Status != NotCovered {
			continue
		}

		for _, c := range diffChanges(string(mu.Diff)) {
			diags = append(diags, model.Diagnostic{
				Range: model.Range{
					Start: model.Position{Line: c.line, Column: c.indent},
					End:   model.Position{Line: c.line, Column: c.end},
				},
				Severity: model.SeverityWarning,
				Message:  survivorMessage(mu, c.mutated),
				Source:   "gooze",
			})
		}
	}

	return diags
}

// survivorMessage tells the reader which change the tests failed to detect.
func survivorMessage(mu Mutation, mutated string) string {
	name := "mutant"
	if mu.Mutagen != "" {
		name = mu.Mutagen + " mutant"
	}

	change := "with this line removed"
	if mutated != "" {
		change = "with: " + mutated
	}

	if mu.Status == NotCovered {
		return "gooze: " + name + " not covered by any test — nothing fails " + change
	}

	return "gooze: surviving " + name + " — tests still pass " + change
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
// line that replaced it.
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
