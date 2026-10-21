package report

import (
	"slices"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// shortCommit is how many characters of a commit a summary shows.
const shortCommit = 7

// partlyTested reports whether any of a file's reports covers only part of it
// (a narrowed gooze run merged into the reports), and the label that says so:
// "partly tested (functions matching ^Add$)" when the reports share one
// selection, "partly tested" otherwise.
func partlyTested(rep []rpt.Report) (string, bool) {
	var selections []string

	for _, r := range rep {
		if r.Scope == nil {
			continue
		}

		if s := describeScope(*r.Scope); !slices.Contains(selections, s) {
			selections = append(selections, s)
		}
	}

	switch {
	case selections == nil:
		return "", false
	// Equivalent: selections is not empty here, so <= 1 is == 1.
	case len(selections) == 1 && selections[0] != "": //gooze:ignore comparison
		return "partly tested (" + selections[0] + ")", true
	default:
		return "partly tested", true
	}
}

// describeScope names what a narrowed run tested of a file, e.g. "changed
// lines since 1a2b3c4; functions matching ^Add$".
func describeScope(s rpt.Scope) string {
	var parts []string

	if s.Base != "" {
		parts = append(parts, "changed lines since "+s.Base[:min(len(s.Base), shortCommit)])
	}

	if s.Run != "" {
		parts = append(parts, "functions matching "+s.Run)
	}

	if s.Mutagens != nil {
		parts = append(parts, "mutagens "+strings.Join(s.Mutagens, ", "))
	}

	return strings.Join(parts, "; ")
}
