// Package model defines the neutral, LSP-agnostic result types the feature logic
// returns (positions, completions, hovers, diagnostics, commands). The adapter
// layer translates these to the LSP wire format; nothing here depends on it.
package model

// Position is a zero-based line and column. Column is a byte offset within the
// line; the adapter maps it to/from LSP's UTF-16 character units.
type Position struct {
	Line   int
	Column int
}

// Range is a half-open span between two positions.
type Range struct {
	Start Position
	End   Position
}

// Severity mirrors LSP diagnostic severities.
type Severity int

// Diagnostic severities, matching the LSP DiagnosticSeverity values.
const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// CompletionItem is one suggested completion. ReplaceStart/ReplaceEnd are byte
// columns on the request line that the insertion should replace.
type CompletionItem struct {
	Label        string
	Detail       string
	Doc          string
	InsertText   string
	ReplaceStart int
	ReplaceEnd   int
}

// Hover is markdown documentation for a position, with the range it applies to.
type Hover struct {
	Markdown string
	Range    Range
}

// Diagnostic is a problem reported for a range of a document.
type Diagnostic struct {
	Range    Range
	Severity Severity
	Message  string
	Source   string
}

// Command is an invocable action (an editor "command").
type Command struct {
	Title     string
	Command   string
	Arguments []any
}

// CodeLens is a Command shown inline at a range (e.g. a "▶ run" affordance).
type CodeLens struct {
	Range   Range
	Command Command
}

// Lines splits document text into zero-based lines without their terminators,
// so positions line up with LSP line indices.
func Lines(text string) []string {
	var lines []string

	start := 0

	for i := range len(text) {
		if text[i] == '\n' {
			line := text[start:i]
			if n := len(line); n > 0 && line[n-1] == '\r' {
				line = line[:n-1]
			}

			lines = append(lines, line)
			start = i + 1
		}
	}

	last := text[start:]
	if n := len(last); n > 0 && last[n-1] == '\r' {
		last = last[:n-1]
	}

	lines = append(lines, last)

	return lines
}
