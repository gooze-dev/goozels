package report

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/model"
)

// IgnoreHint explains how to silence a mutant that cannot be killed. It is
// meant for one line next to a list of survivors, not for each of them.
const IgnoreHint = "a mutant that cannot change behavior (an equivalent mutant) can be skipped with //gooze:ignore <mutagen> on its line"

// mutant is one surviving or uncovered mutation of one source line.
type mutant struct {
	change

	mutagen string
	status  rpt.TestStatus
}

// survivorDiagnostics marks each line with surviving or uncovered mutants,
// read from the reports' unified diffs, in line order. Mutants of the same line
// share one diagnostic that lists each mutated variant. keep filters the
// changes worth marking.
func survivorDiagnostics(reports []rpt.Report, keep func(rpt.Report, change) bool) []model.Diagnostic {
	byLine := map[int][]mutant{}
	seen := map[mutant]bool{}

	for _, r := range reports {
		for _, m := range reportMutants(r) {
			if !seen[m] && keep(r, m.change) {
				seen[m] = true
				byLine[m.line] = append(byLine[m.line], m)
			}
		}
	}

	diags := make([]model.Diagnostic, 0, len(byLine))

	for _, line := range slices.Sorted(maps.Keys(byLine)) {
		diags = append(diags, lineDiagnostic(byLine[line]))
	}

	return diags
}

// reportMutants lists the changed lines of one report for each of its
// surviving or uncovered mutations.
func reportMutants(r rpt.Report) []mutant {
	if r.Diff == nil {
		return nil
	}

	var (
		mutants []mutant
		changes = diffChanges(string(*r.Diff))
	)

	for _, res := range r.Result {
		for _, mu := range res.Mutations {
			// Equivalent: NotCovered is the highest status, so < NotCovered is != NotCovered.
			if mu.Status != rpt.Survived && mu.Status != rpt.NotCovered { //gooze:ignore comparison
				continue
			}

			for _, c := range changes {
				mutants = append(mutants, mutant{change: c, mutagen: res.Name, status: mu.Status})
			}
		}
	}

	return mutants
}

// lineDiagnostic marks one line for the mutants that changed it.
func lineDiagnostic(ms []mutant) model.Diagnostic {
	slices.SortFunc(ms, func(a, b mutant) int {
		return cmp.Or(cmp.Compare(a.status, b.status), cmp.Compare(a.mutagen, b.mutagen), cmp.Compare(a.mutated, b.mutated))
	})

	c := ms[0].change

	return model.Diagnostic{
		Range: model.Range{
			Start: model.Position{Line: c.line, Column: c.indent},
			End:   model.Position{Line: c.line, Column: c.end},
		},
		Severity: model.SeverityWarning,
		Message:  lineMessage(ms),
		Source:   source,
	}
}

// lineMessage tells the reader which changes of a line the tests failed to
// detect, showing the original line once and each mutated variant.
func lineMessage(ms []mutant) string {
	orig := strings.TrimSpace(ms[0].text)

	// Equivalent: a line has at least one mutant, so <= 1 is == 1.
	if len(ms) == 1 { //gooze:ignore comparison
		m := ms[0]
		edit := "`" + orig + "` → " + variant(orig, m.mutated, false)

		// Equivalent: NotCovered is the highest status, so >= NotCovered is == NotCovered.
		if m.status == rpt.NotCovered { //gooze:ignore comparison
			return "gooze: " + mutantName(m.mutagen) + " not covered by any test — " + edit
		}

		return "gooze: surviving " + mutantName(m.mutagen) + " — tests still pass with " + edit
	}

	shown := ms[:min(len(ms), maxVariants)]
	variants := make([]string, 0, len(shown))

	for _, m := range shown {
		variants = append(variants, variant(orig, m.mutated, true))
	}

	list := strings.Join(variants, ", ")
	// Equivalent: more is never negative, so != 0 is > 0.
	if more := len(ms) - len(shown); more > 0 { //gooze:ignore comparison
		list += " and " + strconv.Itoa(more) + " more"
	}

	head, verb := groupHead(ms)

	return "gooze: " + head + " — " + verb + "`" + orig + "` → " + list
}

// maxVariants is how many mutated variants of one line a marker spells out.
const maxVariants = 5

// groupHead counts a line's mutants by outcome and mutagen, e.g. "4 surviving
// mutants (comparison ×3, logical)", and returns the verb that introduces them.
func groupHead(ms []mutant) (head, verb string) {
	uncovered := 0

	for _, m := range ms {
		// Equivalent: NotCovered is the highest status, so >= NotCovered is == NotCovered.
		if m.status == rpt.NotCovered { //gooze:ignore comparison
			uncovered++
		}
	}

	n, tally := strconv.Itoa(len(ms)), " ("+mutagenTally(ms)+")"

	switch uncovered {
	case 0:
		return n + " surviving mutants" + tally, "tests still pass with "
	case len(ms):
		return n + " mutants not covered by any test" + tally, ""
	default:
		return n + " surviving mutants" + tally + ", " + strconv.Itoa(uncovered) + " of them not covered by any test", ""
	}
}

// mutagenTally names the mutagens of ms, most frequent first, with a count
// when more than one: "comparison ×3, logical".
func mutagenTally(ms []mutant) string {
	counts := map[string]int{}

	for _, m := range ms {
		counts[cmp.Or(m.mutagen, "unknown")]++
	}

	names := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})

	for i, name := range names {
		// Equivalent: every counted name occurs at least once, so != 1 is > 1.
		if counts[name] > 1 { //gooze:ignore comparison
			names[i] = name + " ×" + strconv.Itoa(counts[name])
		}
	}

	return strings.Join(names, ", ")
}

func mutantName(mutagen string) string {
	// Equivalent: no string sorts below "", so < "" never holds and <= "" is == "".
	if mutagen == "" { //gooze:ignore comparison
		return "mutant"
	}

	return mutagen + " mutant"
}

// Variants of a line longer than shortLine share their prefix with it as "…",
// when that prefix is at least minElided bytes.
const (
	shortLine = 24
	minElided = 8
)

// variant renders a mutated line in backquotes. With elide set, the prefix a
// long line shares with orig is shortened to "…" at a space, so the variants
// of one line differ visibly: "… < 0 {".
func variant(orig, mutated string, elide bool) string {
	// Equivalent: no string sorts below "", so <= "" is == "".
	if mutated == "" { //gooze:ignore comparison
		return "(line removed)"
	}

	if elide && len(orig) > shortLine {
		if cut := elisionCut(orig, mutated); cut >= minElided {
			mutated = "… " + mutated[cut:]
		}
	}

	return "`" + mutated + "`"
}

// elisionCut returns where a variant's display may start: after the last
// space before it departs from orig.
func elisionCut(orig, mutated string) int {
	// Equivalent: starting at 1 only skips comparing the first bytes, which (lines being trimmed) cannot be a space to cut at.
	shared := 0 //gooze:ignore numbers
	// Equivalent: shared counts up from 0, so != min is < min (<= is killed by a test).
	for shared < min(len(orig), len(mutated)) && orig[shared] == mutated[shared] { //gooze:ignore comparison
		shared++
	}

	cut := strings.LastIndexByte(orig[:shared], ' ') + 1
	// Equivalent: lines are trimmed, so a cut after a space is never 1 nor at the end of mutated.
	if cut > 0 && cut < len(mutated) && isWordByte(mutated[cut]) { //gooze:ignore comparison,numbers
		// A literal or name changed: keep the token before it as context
		// ("… > 1 {", not "… 1 {"). Operators read on their own.
		cut = strings.LastIndexByte(orig[:cut-1], ' ') + 1
	}

	return cut
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// change is one original line a mutation replaced or removed.
type change struct {
	line    int    // zero-based line in the original source
	indent  int    // byte column where the line's code starts
	end     int    // byte column where the line ends
	text    string // the original line, as gooze tested it
	mutated string // replacement text, trimmed; empty when the line was removed
}

// diffChanges extracts the original lines a unified diff ("--- original",
// "+++ mutated", "@@ -a,b +c,d @@" hunks) removes, pairing each with the added
// line that replaced it. gooze's hunks carry three lines of context, so the
// changed line is not the hunk's first line. Lines the diff removes and adds
// back unchanged (gooze rewrites a file's tail that way) are left out.
func diffChanges(diff string) []change {
	var changed []change

	for _, c := range diffLines(diff) {
		if strings.TrimSpace(c.text) != c.mutated {
			changed = append(changed, c)
		}
	}

	return changed
}

// diffLines is diffChanges before dropping unchanged lines.
func diffLines(diff string) []change {
	var (
		changes []change
		old     = -1 // next original line (one-based); -1 until the first hunk
		// pending indexes the first unpaired removal in changes. Equivalent
		// mutants: it is reset at the first hunk header, before any change.
		pending = 0 //gooze:ignore numbers
	)

	for l := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			old = hunkOldStart(l)
			pending = len(changes)
		// Equivalent: old is -1 before the first hunk and at least 1 after it.
		case old < 0 || strings.HasPrefix(l, "\\"): //gooze:ignore comparison,numbers
			// File headers before the first hunk and "\ No newline at end of file".
		case strings.HasPrefix(l, "-"):
			changes = append(changes, removedLine(old-1, l[1:]))
			old++
		case strings.HasPrefix(l, "+"):
			// Equivalent: pending never exceeds len(changes), so != is < (<= is killed by a test).
			if pending < len(changes) { //gooze:ignore comparison
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

	return change{line: max(line, 0), indent: indent, end: len(text), text: text}
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
