// Package goozeyml implements completion, hover and diagnostics for gooze's
// configuration file. It uses a lightweight, dependency-free indentation model of
// YAML mappings rather than a full parser, which is sufficient for the flat,
// shallow gooze schema. It is pure and LSP-agnostic.
package goozeyml

import (
	"strings"

	"gooze.dev/goozels/internal/domain/config"
	"gooze.dev/goozels/internal/model"
)

// entry is a parsed "key: value" mapping line.
type entry struct {
	line       int
	indent     int
	key        string
	keyStart   int // byte column of key token start
	keyEnd     int // byte column of key token end
	value      string
	valueStart int
}

// parseEntries extracts mapping-key lines from text, ignoring blanks, comments
// and sequence items.
func parseEntries(lines []string) []entry {
	var entries []entry

	for i, line := range lines {
		e, ok := parseEntry(i, line)
		if ok {
			entries = append(entries, e)
		}
	}

	return entries
}

func parseEntry(lineNum int, line string) (entry, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}

	rest := line[indent:]
	if rest == "" || rest[0] == '#' || rest[0] == '-' {
		return entry{}, false
	}

	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return entry{}, false
	}

	key := strings.TrimSpace(rest[:colon])
	if key == "" {
		return entry{}, false
	}

	valueStart := indent + colon + 1

	value := ""
	if valueStart <= len(line) {
		value = stripComment(strings.TrimSpace(line[valueStart:]))
	}

	return entry{
		line:       lineNum,
		indent:     indent,
		key:        key,
		keyStart:   indent,
		keyEnd:     indent + colon,
		value:      value,
		valueStart: valueStart,
	}, true
}

// pathAtIndent replays entries (assumed to precede the target) maintaining a
// nesting stack, then returns the key path of ancestors strictly less indented
// than indent, plus the direct-child sibling keys already present at indent.
func pathAtIndent(entries []entry, indent int) (path []string, siblings map[string]bool) {
	var stack []entry
	for _, e := range entries {
		for len(stack) > 0 && stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}

		stack = append(stack, e)
	}

	for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
		stack = stack[:len(stack)-1]
	}

	for _, e := range stack {
		path = append(path, e.key)
	}

	siblings = map[string]bool{}

	for _, e := range entries {
		if e.indent == indent {
			siblings[e.key] = true
		}
	}

	return path, siblings
}

// Complete returns config completions for a position on the given line: enum
// values when inside a "key:" value, otherwise config keys for the indent level.
func Complete(text string, lineNum, col int) []model.CompletionItem {
	lines := model.Lines(text)
	if lineNum < 0 || lineNum >= len(lines) {
		return nil
	}

	line := lines[lineNum]
	if items, ok := valueCompletion(lines, lineNum, line, col); ok {
		return items
	}

	return keyCompletion(lines, lineNum, line, col)
}

// valueCompletion offers enum values when col is in the value position of a
// "key:" line whose field is an enum. ok is false when it doesn't apply.
func valueCompletion(lines []string, lineNum int, line string, col int) ([]model.CompletionItem, bool) {
	e, ok := parseEntry(lineNum, line)
	if !ok || col <= e.valueStart-1 || e.keyEnd >= col {
		return nil, false
	}

	path, _ := pathAtIndent(parseEntries(lines[:lineNum]), e.indent)

	f, ok := config.Lookup(append(path, e.key))
	if !ok || f.Kind != config.KindEnum {
		return nil, false
	}

	return enumItems(f, line, e.valueStart, col), true
}

// keyCompletion offers config-key completions for the current indent level,
// excluding sibling keys already present.
func keyCompletion(lines []string, lineNum int, line string, col int) []model.CompletionItem {
	indent := leadingSpaces(line)
	if col < indent {
		return nil
	}
	// Don't offer keys once we're past a colon on this line.
	if c := strings.IndexByte(line, ':'); c >= 0 && col > c {
		return nil
	}

	path, siblings := pathAtIndent(parseEntries(lines[:lineNum]), indent)
	tokenStart, tokenEnd := wordBounds(line, indent, col)
	prefix := strings.ToLower(line[tokenStart:col])

	var items []model.CompletionItem

	for _, f := range config.ChildrenAt(path) {
		if siblings[f.Key] || !strings.HasPrefix(f.Key, prefix) {
			continue
		}

		items = append(items, model.CompletionItem{
			Label:        f.Key,
			Detail:       kindLabel(f),
			Doc:          f.Doc,
			InsertText:   insertText(f),
			ReplaceStart: tokenStart,
			ReplaceEnd:   tokenEnd,
		})
	}

	return items
}

// Hover returns documentation for the config key under the cursor.
func Hover(text string, lineNum, col int) (model.Hover, bool) {
	lines := model.Lines(text)
	if lineNum < 0 || lineNum >= len(lines) {
		return model.Hover{}, false
	}

	line := lines[lineNum]

	e, ok := parseEntry(lineNum, line)
	if !ok || col < e.keyStart || col > e.keyEnd {
		return model.Hover{}, false
	}

	path, _ := pathAtIndent(parseEntries(lines[:lineNum]), e.indent)

	f, ok := config.Lookup(append(path, e.key))
	if !ok {
		return model.Hover{}, false
	}

	md := "**" + strings.Join(append(path, e.key), ".") + "** _(" + kindLabel(f) + ")_\n\n" + f.Doc
	if f.Default != "" {
		md += "\n\nDefault: `" + f.Default + "`"
	}

	return model.Hover{
		Markdown: md,
		Range: model.Range{
			Start: model.Position{Line: lineNum, Column: e.keyStart},
			End:   model.Position{Line: lineNum, Column: e.keyEnd},
		},
	}, true
}

// Diagnose validates the whole config document: unknown keys and bad scalar values.
func Diagnose(text string) []model.Diagnostic {
	lines := model.Lines(text)
	entries := parseEntries(lines)

	var (
		diags []model.Diagnostic
		stack []entry
	)

	for _, e := range entries {
		for len(stack) > 0 && stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}

		path := make([]string, 0, len(stack)+1)
		for _, s := range stack {
			path = append(path, s.key)
		}

		path = append(path, e.key)
		stack = append(stack, e)

		f, ok := config.Lookup(path)
		if !ok {
			diags = append(diags, model.Diagnostic{
				Range:    keyRange(e),
				Severity: model.SeverityWarning,
				Message:  "unknown gooze config key \"" + strings.Join(path, ".") + "\"",
				Source:   "gooze",
			})

			continue
		}

		if e.value == "" {
			continue
		}

		if msg := f.ValidateScalar(stripQuotes(e.value)); msg != "" {
			diags = append(diags, model.Diagnostic{
				Range: model.Range{
					Start: model.Position{Line: e.line, Column: e.valueStart},
					End:   model.Position{Line: e.line, Column: len(lines[e.line])},
				},
				Severity: model.SeverityError,
				Message:  "invalid value for \"" + strings.Join(path, ".") + "\": " + msg,
				Source:   "gooze",
			})
		}
	}

	return diags
}

func enumItems(f config.Field, line string, valStart, col int) []model.CompletionItem {
	tokenStart, tokenEnd := wordBounds(line, valStart, col)
	for tokenStart < len(line) && line[tokenStart] == ' ' {
		tokenStart++
	}

	if tokenStart > col {
		tokenStart = col
	}

	prefix := strings.ToLower(strings.TrimSpace(line[minInt(tokenStart, col):col]))

	var items []model.CompletionItem

	for _, v := range f.Enum {
		if !strings.HasPrefix(v, prefix) {
			continue
		}

		items = append(items, model.CompletionItem{
			Label:        v,
			Detail:       "value",
			InsertText:   v,
			ReplaceStart: tokenStart,
			ReplaceEnd:   tokenEnd,
		})
	}

	return items
}

func keyRange(e entry) model.Range {
	return model.Range{
		Start: model.Position{Line: e.line, Column: e.keyStart},
		End:   model.Position{Line: e.line, Column: e.keyEnd},
	}
}

func insertText(f config.Field) string {
	if f.Kind == config.KindObject {
		return f.Key + ":"
	}

	return f.Key + ": "
}

func kindLabel(f config.Field) string {
	switch f.Kind {
	case config.KindObject:
		return "object"
	case config.KindString:
		return "string"
	case config.KindBool:
		return "bool"
	case config.KindInt:
		return "int"
	case config.KindStringList:
		return "string[]"
	case config.KindEnum:
		return "enum: " + strings.Join(f.Enum, "|")
	}

	return ""
}

func leadingSpaces(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}

	return n
}

func wordBounds(line string, lo, col int) (int, int) {
	start := col
	for start > lo && isWordByte(line[start-1]) {
		start--
	}

	end := col
	for end < len(line) && isWordByte(line[end]) {
		end++
	}

	return start, end
}

func isWordByte(c byte) bool {
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func stripComment(s string) string {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}

	return s
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}

	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
