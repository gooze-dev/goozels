// Package directive parses gooze:ignore annotations out of a single line of Go
// source. It is a pure domain package: it works on plain text and byte offsets
// and knows nothing about LSP or gopls.
//
// The grammar mirrors gooze's own parser
// (gooze.dev/pkg/gooze/internal/domain/ignore.go): a comment marker ("//" or
// "/*"), optional whitespace, the literal "gooze:ignore", and an optional
// comma-separated list of mutagen names. An empty list means "ignore all".
package directive

// Name is a single mutagen name token within a directive, with its byte span on
// the line ([Start, End)).
type Name struct {
	Text  string
	Start int
	End   int
}

// Directive is a parsed gooze:ignore annotation located on a line.
type Directive struct {
	// CommentStart is the byte offset of the "//" or "/*" marker on the line.
	CommentStart int
	// ListStart is the byte offset where the mutagen list begins (just after
	// "gooze:ignore"); names are parsed from here.
	ListStart int
	// ListEnd is the byte offset where the mutagen list ends (end of line, or
	// the "*/" of a block comment).
	ListEnd int
	// Names are the parsed mutagen tokens, in source order.
	Names []Name
	// IgnoreAll is true when no names are listed (the directive ignores every
	// mutagen).
	IgnoreAll bool
}

const keyword = "gooze:ignore"

// ParseLine finds a gooze:ignore directive on the given line and returns it.
// The second result is false when the line contains no directive.
func ParseLine(line string) (Directive, bool) {
	kw := indexKeywordAfterComment(line)
	if kw < 0 {
		return Directive{}, false
	}

	d := Directive{
		CommentStart: commentStartBefore(line, kw),
		ListStart:    kw + len(keyword),
	}

	d.ListEnd = listEnd(line, d.ListStart)
	d.Names = parseNames(line, d.ListStart, d.ListEnd)
	d.IgnoreAll = len(d.Names) == 0

	return d, true
}

// indexKeywordAfterComment returns the byte offset of "gooze:ignore" when it is
// preceded (allowing whitespace) by a comment marker on the same line, or -1.
func indexKeywordAfterComment(line string) int {
	for from := 0; ; {
		i := indexFrom(line, keyword, from)
		if i < 0 {
			return -1
		}

		if commentStartBefore(line, i) >= 0 {
			return i
		}

		from = i + 1
	}
}

// commentStartBefore checks that the text immediately left of kw (ignoring
// spaces and tabs) is a "//" or "/*" marker, returning the marker's offset or -1.
func commentStartBefore(line string, kw int) int {
	j := kw
	for j > 0 && (line[j-1] == ' ' || line[j-1] == '\t') {
		j--
	}

	if j >= 2 && (line[j-2:j] == "//" || line[j-2:j] == "/*") {
		return j - 2
	}

	return -1
}

// listEnd returns the offset at which the mutagen list ends: the start of a
// closing "*/" if present, otherwise the end of the line.
func listEnd(line string, from int) int {
	if i := indexFrom(line, "*/", from); i >= 0 {
		return i
	}

	return len(line)
}

func parseNames(line string, start, end int) []Name {
	var names []Name

	i := start
	for i < end {
		// Skip separators (whitespace and commas).
		for i < end && isSep(line[i]) {
			i++
		}

		if i >= end {
			break
		}

		tokStart := i
		for i < end && !isSep(line[i]) {
			i++
		}

		names = append(names, Name{Text: line[tokStart:i], Start: tokStart, End: i})
	}

	return names
}

// Cursor describes where a position sits relative to a directive's mutagen list,
// for driving completion.
type Cursor struct {
	// InList is true when the position is within the mutagen-list region.
	InList bool
	// TokenStart and TokenEnd bound the name token under the cursor (the range a
	// completion should replace). When the cursor is on empty space they are
	// equal to the cursor offset.
	TokenStart int
	TokenEnd   int
	// Prefix is the already-typed text of the token up to the cursor.
	Prefix string
}

// CursorAt reports the completion context for byte offset col on the line. It
// returns false when col is not inside a directive's mutagen list.
func CursorAt(line string, col int) (Cursor, bool) {
	d, ok := ParseLine(line)
	if !ok {
		return Cursor{}, false
	}

	if !cursorInList(d, line, col) {
		return Cursor{}, false
	}

	start, end := tokenBounds(line, d.ListStart, d.ListEnd, col)

	return Cursor{
		InList:     true,
		TokenStart: start,
		TokenEnd:   end,
		Prefix:     line[start:col],
	}, true
}

// cursorInList reports whether col sits inside the directive's mutagen-list
// region (everything after "gooze:ignore", requiring a separator before the
// first token).
func cursorInList(d Directive, line string, col int) bool {
	if col < d.ListStart || col > d.ListEnd {
		return false
	}
	// A name immediately abutting the keyword ("gooze:ignorefoo") is not an entry.
	if col == d.ListStart && d.ListStart < d.ListEnd && !isSep(line[d.ListStart]) {
		return false
	}

	return true
}

// tokenBounds returns the [start, end) span of the non-separator token covering
// col, clamped to [lo, hi).
func tokenBounds(line string, lo, hi, col int) (int, int) {
	start := col
	for start > lo && !isSep(line[start-1]) {
		start--
	}

	end := col
	for end < hi && !isSep(line[end]) {
		end++
	}

	return start, end
}

func isSep(c byte) bool { return c == ' ' || c == '\t' || c == ',' }

// indexFrom returns the index of sub in s at or after from, or -1.
func indexFrom(s, sub string, from int) int {
	if from < 0 {
		from = 0
	}

	if from > len(s) {
		return -1
	}

	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}
