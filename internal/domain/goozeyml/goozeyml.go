// Package goozeyml implements completion, hover and diagnostics for gooze's
// configuration file. It uses a lightweight, dependency-free indentation model of
// YAML mappings rather than a full parser, which is sufficient for the flat,
// shallow gooze schema. It is pure and LSP-agnostic.
package goozeyml

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gooze.dev/pkg/gooze/pkg/config"

	"gooze.dev/pkg/goozels/internal/domain/document"
	"gooze.dev/pkg/goozels/internal/model"
)

// Editor serves completion, hover and diagnostics for the config file described
// by a registry.
type Editor struct {
	registry config.Registry
}

// New returns an Editor for the configuration schema of registry.
func New(registry config.Registry) Editor {
	return Editor{registry: registry}
}

// source labels every diagnostic goozels reports for gooze config files.
const source = "gooze"

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

	colon := separatorColon(rest)
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
func (ed Editor) Complete(text string, lineNum, col int) []model.CompletionItem {
	lines := document.Lines(text)
	if lineNum < 0 || lineNum >= len(lines) {
		return nil
	}

	line := lines[lineNum]
	if _, _, isItem := listItem(line); isItem {
		return ed.itemCompletion(lines, lineNum, line, col)
	}

	if items, ok := ed.valueCompletion(lines, lineNum, line, col); ok {
		return items
	}

	return ed.keyCompletion(lines, lineNum, line, col)
}

// valueCompletion offers enum values when col is in the value position of a
// "key:" line whose field is an enum. ok is false when it doesn't apply.
func (ed Editor) valueCompletion(lines []string, lineNum int, line string, col int) ([]model.CompletionItem, bool) {
	e, ok := parseEntry(lineNum, line)
	if !ok || col <= e.valueStart-1 || e.keyEnd >= col {
		return nil, false
	}

	path, _ := pathAtIndent(parseEntries(lines[:lineNum]), e.indent)

	f, ok := ed.lookup(append(path, e.key))
	if !ok || f.Kind != config.KindEnum {
		return nil, false
	}

	return enumItems(f, line, e.valueStart, col), true
}

// itemCompletion offers the allowed values of a list key on one of its
// "- item" lines, or none.
func (ed Editor) itemCompletion(lines []string, lineNum int, line string, col int) []model.CompletionItem {
	start, _, _ := listItem(line)
	if col < start {
		return nil
	}

	entries := parseEntries(lines[:lineNum])
	if len(entries) == 0 {
		return nil
	}

	owner := entries[len(entries)-1]
	path, _ := pathAtIndent(entries, owner.indent)

	return enumItems(ed.itemField(owner, append(path, owner.key), line), line, start, col)
}

// keyCompletion offers config-key completions for the current indent level,
// excluding sibling keys already present.
func (ed Editor) keyCompletion(lines []string, lineNum int, line string, col int) []model.CompletionItem {
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

	for _, f := range ed.childrenAt(path) {
		if siblings[f.Name] || !strings.HasPrefix(f.Name, prefix) {
			continue
		}

		items = append(items, model.CompletionItem{
			Label:        f.Name,
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
func (ed Editor) Hover(text string, lineNum, col int) (model.Hover, bool) {
	lines := document.Lines(text)
	if lineNum < 0 || lineNum >= len(lines) {
		return model.Hover{}, false
	}

	line := lines[lineNum]

	e, ok := parseEntry(lineNum, line)
	if !ok || col < e.keyStart || col > e.keyEnd {
		return model.Hover{}, false
	}

	path, _ := pathAtIndent(parseEntries(lines[:lineNum]), e.indent)

	f, ok := ed.lookup(append(path, e.key))
	if !ok {
		return model.Hover{}, false
	}

	md := "**" + strings.Join(append(path, e.key), ".") + "** _(" + kindLabel(f) + ")_\n\n" + f.Doc
	if def := defaultText(f.Default); def != "" {
		md += "\n\nDefault: `" + def + "`"
	}

	return model.Hover{
		Markdown: md,
		Range: model.Range{
			Start: model.Position{Line: lineNum, Column: e.keyStart},
			End:   model.Position{Line: lineNum, Column: e.keyEnd},
		},
	}, true
}

// separatorColon returns the index of the colon that ends a mapping key: in
// YAML a colon separates key from value only when followed by whitespace or the
// end of the line, so "run:r" is a plain scalar, not the key "run". It returns
// -1 when the text has no such colon.
func separatorColon(text string) int {
	for i := range len(text) {
		if text[i] == ':' && (i+1 == len(text) || text[i+1] == ' ' || text[i+1] == '\t') {
			return i
		}
	}

	return -1
}

// Diagnose validates the whole config document: malformed lines, unknown keys
// and bad scalar values.
func (ed Editor) Diagnose(text string) []model.Diagnostic {
	lines := document.Lines(text)

	var (
		diags  []model.Diagnostic
		stack  []entry
		broken = -1 // indent of the last malformed line; deeper lines are its children
	)

	var (
		owner     entry    // the last entry, which owns the "- item" lines below it
		ownerPath []string // owner's key path; nil before the first entry
	)

	for i, line := range lines {
		e, ok := parseEntry(i, line)
		if !ok {
			if d, bad := ed.diagnoseItem(i, line, owner, ownerPath); bad {
				diags = append(diags, d)
			}

			if d, bad := malformedLine(i, line); bad {
				diags = append(diags, d)
				broken = d.Range.Start.Column
			}

			continue
		}

		if broken >= 0 && e.indent > broken {
			continue // the parent is unreadable, so this key's path is unknown
		}

		broken = -1

		for len(stack) > 0 && stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}

		path := entryPath(stack, e)
		stack = append(stack, e)
		owner, ownerPath = e, path

		if d, bad := ed.diagnoseEntry(lines, path, e); bad {
			diags = append(diags, d)
		}
	}

	return diags
}

// diagnoseEntry checks one mapping entry against the config schema.
func (ed Editor) diagnoseEntry(lines []string, path []string, e entry) (model.Diagnostic, bool) {
	valueRange := model.Range{
		Start: model.Position{Line: e.line, Column: e.valueStart},
		End:   model.Position{Line: e.line, Column: len(lines[e.line])},
	}

	if open := unclosedFlow(e.value); open != "" {
		return model.Diagnostic{
			Range:    valueRange,
			Severity: model.SeverityError,
			Message:  "invalid YAML: \"" + open + "\" is never closed; gooze will ignore this whole file and use its defaults",
			Source:   source,
		}, true
	}

	key := strings.Join(path, ".")

	f, ok := ed.lookup(path)
	if !ok {
		msg := "unknown gooze config key \"" + key + "\""
		if s := ed.suggest(key); s != "" {
			msg += "; did you mean \"" + s + "\"?"
		}

		return model.Diagnostic{Range: keyRange(e), Severity: model.SeverityWarning, Message: msg, Source: source}, true
	}

	if e.value == "" {
		return model.Diagnostic{}, false
	}

	msg := validateScalar(f, stripQuotes(e.value))
	if msg == "" {
		return model.Diagnostic{}, false
	}

	return model.Diagnostic{
		Range:    valueRange,
		Severity: model.SeverityError,
		Message:  "invalid value for \"" + key + "\": " + msg,
		Source:   source,
	}, true
}

// diagnoseItem checks a block-list item ("- value") against the allowed
// values of the list key it belongs to (see itemField). owner is the last
// entry above the line, at ownerPath; ok is false without one.
func (ed Editor) diagnoseItem(lineNum int, line string, owner entry, ownerPath []string) (model.Diagnostic, bool) {
	start, end, isItem := listItem(line)
	if !isItem {
		return model.Diagnostic{}, false
	}

	msg := validateItem(ed.itemField(owner, ownerPath, line), stripQuotes(line[start:end]))
	if msg == "" {
		return model.Diagnostic{}, false
	}

	return model.Diagnostic{
		Range: model.Range{
			Start: model.Position{Line: lineNum, Column: start},
			End:   model.Position{Line: lineNum, Column: end},
		},
		Severity: model.SeverityError,
		Message:  "invalid value for \"" + strings.Join(ownerPath, ".") + "\": " + msg,
		Source:   source,
	}, true
}

// itemField returns the list field a "- item" line belongs to, or the zero
// Field (which allows any item) when it belongs to none. owner, the entry
// above the line, must have no inline value and sit at the item's indent or
// less (YAML allows a sequence at its key's indent).
func (ed Editor) itemField(owner entry, ownerPath []string, line string) config.Field {
	if owner.value != "" || owner.indent > leadingSpaces(line) {
		return config.Field{}
	}

	// Equivalent: kinds below KindStringList have no allowed values, so > is !=.
	if f, ok := ed.lookup(ownerPath); ok && f.Kind == config.KindStringList { //gooze:ignore comparison
		return f
	}

	return config.Field{}
}

// listItem returns the byte range of a block-list item's value ("- value #
// note" gives value's range), and false when line is not a list item.
func listItem(line string) (start, end int, ok bool) {
	indent := leadingSpaces(line)
	rest, ok := strings.CutPrefix(line[indent:], "-")

	// An item's dash ends the line or is followed by a space or a tab; the
	// appended space covers the bare "-".
	if !ok || !strings.HasPrefix(rest+" ", " ") && !strings.HasPrefix(rest, "\t") {
		// Equivalent: callers ignore the range when ok is false.
		return 0, 0, false //gooze:ignore numbers
	}

	start = len(line) - len(strings.TrimLeft(rest, " \t"))
	value := strings.TrimRight(stripComment(line[start:]), " ")

	return start, start + len(value), true
}

// malformedLine reports a non-blank line that is neither a comment, a list item
// nor a "key: value" entry. gooze cannot parse such a file and falls back to its
// defaults for every setting, so this is an error.
func malformedLine(lineNum int, line string) (model.Diagnostic, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if strings.TrimSpace(trimmed) == "" || trimmed[0] == '#' || trimmed[0] == '-' {
		return model.Diagnostic{}, false
	}

	indent := len(line) - len(trimmed)
	msg := "not a valid \"key: value\" line; gooze will ignore this whole file and use its defaults"

	if strings.Contains(trimmed, ":") {
		msg = "missing a space after \":\"; gooze will ignore this whole file and use its defaults"
	}

	return model.Diagnostic{
		Range: model.Range{
			Start: model.Position{Line: lineNum, Column: indent},
			End:   model.Position{Line: lineNum, Column: len(strings.TrimRight(line, " \t"))},
		},
		Severity: model.SeverityError,
		Message:  msg,
		Source:   source,
	}, true
}

// entryPath is the dotted key path of e under its enclosing mapping entries.
func entryPath(stack []entry, e entry) []string {
	path := make([]string, 0, len(stack)+1)
	for _, s := range stack {
		path = append(path, s.key)
	}

	return append(path, e.key)
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

// lookup resolves a key path to its field.
func (ed Editor) lookup(path []string) (config.Field, bool) {
	return ed.registry.Lookup(strings.Join(path, "."))
}

// childrenAt returns the fields valid at a key path (empty path = top level),
// or nil when the path does not resolve to an object.
func (ed Editor) childrenAt(path []string) []config.Field {
	if len(path) == 0 {
		return ed.registry.Schema()
	}

	f, ok := ed.lookup(path)
	if !ok || f.Kind != config.KindObject {
		return nil
	}

	return f.Children
}

// validateScalar checks a raw (unquoted, trimmed) scalar value against a field's
// kind, returning a human-readable problem or "" when valid. Strings and lists
// are not validated.
func validateScalar(f config.Field, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	switch f.Kind {
	case config.KindBool:
		if !isBoolWord(raw) {
			return "expected a boolean (true or false)"
		}
	case config.KindInt, config.KindFloat:
		return validateNumber(f, raw)
	case config.KindEnum:
		return validateEnum(f, raw)
	case config.KindObject:
		// A flow mapping ("run: {parallel: 4}") is valid YAML; anything else
		// replaces the section's settings with a plain value.
		if !strings.HasPrefix(raw, "{") {
			return "is a section; put its settings on indented lines below it"
		}
	case config.KindStringList:
		return validateItems(f, raw)
	case config.KindString:
	}

	return ""
}

// validateItems checks the items of a list value ("[a, b]", or a single
// item) against the field's allowed values, when it has any.
func validateItems(f config.Field, raw string) string {
	items := []string{raw}
	if inner, ok := strings.CutPrefix(raw, "["); ok {
		items = strings.Split(strings.TrimSuffix(inner, "]"), ",")
	}

	for _, item := range items {
		if msg := validateItem(f, stripQuotes(strings.TrimSpace(item))); msg != "" {
			return msg
		}
	}

	return ""
}

// validateItem checks one list item against the field's allowed values, when
// it has any.
func validateItem(f config.Field, item string) string {
	if len(f.Enum) == 0 || item == "" || slices.Contains(f.Enum, strings.ToLower(item)) {
		return ""
	}

	return "unknown item \"" + item + "\"; expected one of: " + strings.Join(f.Enum, ", ")
}

// validateNumber checks raw is a number of the field's kind, within its
// bounds.
func validateNumber(f config.Field, raw string) string {
	if f.Kind == config.KindFloat {
		return validateFloat(f, raw)
	}

	return validateInt(f, raw)
}

// validateInt checks raw is an integer within the schema's bounds (e.g.
// run.parallel >= 1), the same bounds gooze run enforces.
func validateInt(f config.Field, raw string) string {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return "expected an integer"
	}

	if err := f.CheckInt(n); err != nil {
		return err.Error()
	}

	return ""
}

// validateFloat checks raw is a number within the schema's bounds (e.g. a
// threshold between 0 and 100), the same bounds gooze enforces.
func validateFloat(f config.Field, raw string) string {
	x, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return "expected a number"
	}

	if err := f.CheckFloat(x); err != nil {
		return err.Error()
	}

	return ""
}

// validateEnum checks raw against an enum field's allowed values, accepting an
// integer when the field allows one.
func validateEnum(f config.Field, raw string) string {
	if f.IntEnum {
		if _, err := strconv.Atoi(raw); err == nil {
			return ""
		}
	}

	if !slices.Contains(f.Enum, strings.ToLower(raw)) {
		return "expected one of: " + strings.Join(f.Enum, ", ")
	}

	return ""
}

func isBoolWord(raw string) bool {
	switch strings.ToLower(raw) {
	case "true", "false":
		return true
	}

	return false
}

// defaultText renders a field's default value, or "" when it has none.
func defaultText(v any) string {
	switch d := v.(type) {
	case nil:
		return ""
	case string:
		return d
	case []string:
		return "[" + strings.Join(d, ", ") + "]"
	default:
		return fmt.Sprint(d)
	}
}

func insertText(f config.Field) string {
	if f.Kind == config.KindObject {
		return f.Name + ":"
	}

	return f.Name + ": "
}

func kindLabel(f config.Field) string {
	switch {
	case f.Kind == config.KindEnum:
		return "enum: " + strings.Join(f.Enum, "|")
	// Equivalent: of the higher kinds, KindEnum returned above and KindFloat
	// has no allowed values.
	case f.Kind == config.KindStringList && len(f.Enum) > 0: //gooze:ignore comparison
		return "list: " + strings.Join(f.Enum, "|")
	}

	return f.Kind.String()
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
	if before, _, ok := strings.Cut(s, "#"); ok {
		return strings.TrimSpace(before)
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
