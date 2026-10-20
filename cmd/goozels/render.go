package main

import (
	"strconv"
	"strings"

	"gooze.dev/pkg/goozels/internal/controller"
	"gooze.dev/pkg/goozels/internal/model"
)

// briefSuffix marks a summary shown without the survivors listed under it.
const briefSuffix = " (survivors as listed earlier in this session)"

// renderOptions caps what render prints. Zero means no limit.
type renderOptions struct {
	maxLines int  // diagnostic lines
	maxBytes int  // bytes of diagnostic lines
	hint     bool // add the //gooze:ignore hint whenever survivors are listed, not only when some are left out
}

// output is the rendered result of checking files.
type output struct {
	lines  []string // diagnostics, then a cap line and the //gooze:ignore hint when there is one
	notes  []string // files gooze has no results for
	errors bool     // an error diagnostic was printed
}

// line is one diagnostic ready to print.
type line struct {
	text     string
	err      bool
	survivor bool
}

// render formats the diagnostics of entries in order, within the limits of
// opts. When some are left out, a final line names the files to check for all
// of them.
func render(entries []entry, opts renderOptions) output {
	var (
		out output
		c   = capper{opts: opts, counts: map[string]int{}}
	)

	for _, e := range entries {
		if e.note != "" {
			out.notes = append(out.notes, e.note)
		}

		for _, l := range entryLines(e) {
			if c.add(e.shown, l) {
				out.lines = append(out.lines, l.text)
				out.errors = out.errors || l.err
			}
		}
	}

	if len(c.hidden) > 0 {
		out.lines = append(out.lines, capLine(c.hidden, c.counts))
	}

	if c.survived && (opts.hint || len(c.hidden) > 0) {
		out.lines = append(out.lines, controller.IgnoreHint)
	}

	return out
}

// capper tracks what render prints against its limits.
type capper struct {
	opts     renderOptions
	lines    int
	bytes    int
	hidden   []string       // files with diagnostics left out, in order
	counts   map[string]int // diagnostics left out per file
	survived bool           // a survivor was seen, printed or not
}

// add reports whether l fits, counting it as printed or as left out.
func (c *capper) add(file string, l line) bool {
	c.survived = c.survived || l.survivor

	if exceeds(c.opts, c.lines+1, c.bytes+len(l.text)+1) {
		// Equivalent: counts are never negative, so <= 0 is == 0.
		if c.counts[file] == 0 { //gooze:ignore comparison
			c.hidden = append(c.hidden, file)
		}

		c.counts[file]++

		return false
	}

	c.lines++
	c.bytes += len(l.text) + 1

	return true
}

func exceeds(opts renderOptions, lines, bytes int) bool {
	return (opts.maxLines > 0 && lines > opts.maxLines) || (opts.maxBytes > 0 && bytes > opts.maxBytes)
}

// capLine says how many diagnostics were left out and how to see them all:
// "… 59 more; run goozels check internal/domain/workflow.go".
func capLine(files []string, counts map[string]int) string {
	n := 0
	for _, f := range files {
		n += counts[f]
	}

	return "… " + strconv.Itoa(n) + " more; run goozels check " + strings.Join(files, " ")
}

// entryLines formats an entry's diagnostics: all of them, or for a brief
// entry its results' summary and its own problems.
func entryLines(e entry) []line {
	diags := e.check.Diagnostics()
	if e.brief {
		diags = e.check.Edit
	}

	// Equivalent: only the slice's initial capacity changes.
	lines := make([]line, 0, len(diags)+1) //gooze:ignore arithmetic,numbers

	if e.brief && len(e.check.Results) > 0 {
		summary := formatDiagnostic(e.shown, e.check.Results[0])
		// Equivalent: the enclosing condition has len > 0, so != 1 is > 1.
		if len(e.check.Results) > 1 { //gooze:ignore comparison
			summary += briefSuffix
		}

		lines = append(lines, line{text: summary})
	}

	survivors := map[model.Diagnostic]bool{}
	for _, d := range e.check.Results[min(1, len(e.check.Results)):] {
		survivors[d] = true
	}

	for _, d := range diags {
		lines = append(lines, line{
			text: formatDiagnostic(e.shown, d),
			// Equivalent: SeverityError is the lowest severity, so <= is ==.
			err:      d.Severity == model.SeverityError, //gooze:ignore comparison
			survivor: survivors[d],
		})
	}

	return lines
}
