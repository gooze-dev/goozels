// Package lsp holds the subset of LSP wire types goozels reads or writes, plus
// helpers to translate between LSP positions (UTF-16) and byte offsets. It is a
// protocol library with no project dependencies, so every layer may import it.
package lsp

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Position is an LSP position; Character is a UTF-16 code-unit offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is an LSP range.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextEdit replaces Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// MarkupContent is markdown (or plaintext) documentation.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// TextDocumentIdentifier references a document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier references a document version.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentPositionParams is the common shape of completion/hover requests.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CompletionItem is a single completion suggestion.
type CompletionItem struct {
	Label         string         `json:"label"`
	Kind          int            `json:"kind,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	Documentation *MarkupContent `json:"documentation,omitempty"`
	TextEdit      *TextEdit      `json:"textEdit,omitempty"`
	InsertText    string         `json:"insertText,omitempty"`
}

// CompletionList is the structured completion response.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Hover is a hover response.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// Diagnostic is a single reported problem.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// PublishDiagnosticsParams is the payload of textDocument/publishDiagnostics.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// TextDocumentItem is an opened document.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidOpenTextDocumentParams is textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// TextDocumentContentChangeEvent is one change (incremental when Range is set,
// otherwise a full replacement).
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// DidChangeTextDocumentParams is textDocument/didChange.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// DidCloseTextDocumentParams is textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// Command is an LSP command.
type Command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

// CodeLens is an LSP code lens.
type CodeLens struct {
	Range   Range    `json:"range"`
	Command *Command `json:"command,omitempty"`
}

// ExecuteCommandParams is the payload of workspace/executeCommand.
type ExecuteCommandParams struct {
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments"`
}

// ShowMessageParams is the payload of window/showMessage and window/logMessage.
type ShowMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

// ShowMessageRequestParams is the payload of window/showMessageRequest: a
// message with buttons the user can choose from.
type ShowMessageRequestParams struct {
	Type    int                 `json:"type"`
	Message string              `json:"message"`
	Actions []MessageActionItem `json:"actions,omitempty"`
}

// MessageActionItem is one button of a window/showMessageRequest, and the
// client's reply when the user chooses it.
type MessageActionItem struct {
	Title string `json:"title"`
}

// Message types (LSP MessageType).
const (
	MessageError = 1
	MessageWarn  = 2
	MessageInfo  = 3
	MessageLog   = 4
)

// Completion item kinds (LSP CompletionItemKind).
const (
	CompletionKindEnumMember = 20
	CompletionKindProperty   = 10
	CompletionKindValue      = 12
)

// URIPath converts a file:// URI to a filesystem path.
func URIPath(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Scheme == "file" {
		return u.Path
	}

	return strings.TrimPrefix(uri, "file://")
}

// IsGoURI reports whether the URI names a .go file.
func IsGoURI(uri string) bool { return strings.HasSuffix(URIPath(uri), ".go") }

// UTF16ToByte converts a UTF-16 character offset on a line to a byte offset.
func UTF16ToByte(line string, u16 int) int {
	// Equivalent: the loop below also returns 0 for u16 <= 0, so < and == only
	// move where that 0 comes from.
	if u16 <= 0 { //gooze:ignore comparison
		return 0
	}

	count := 0
	for i, r := range line {
		if count >= u16 {
			return i
		}

		if r > 0xFFFF {
			count += 2
		} else {
			count++
		}
	}

	return len(line)
}

// ByteToUTF16 converts a byte offset on a line to a UTF-16 character offset.
func ByteToUTF16(line string, b int) int {
	// Equivalent: the loop stops at b or counts the whole line, so clamping at
	// len(line) with >= or == gives the same count.
	if b > len(line) { //gooze:ignore comparison
		b = len(line)
	}

	count := 0

	for i, r := range line {
		if i >= b {
			break
		}

		if r > 0xFFFF {
			count += 2
		} else {
			count++
		}
	}

	return count
}

// PositionToOffset converts an LSP position to a byte offset within text,
// clamping positions past the end to len(text).
func PositionToOffset(text string, pos Position) int {
	line, i := 0, 0
	for line < pos.Line && i < len(text) {
		if text[i] == '\n' {
			line++
		}

		i++
	}

	end := i
	for end < len(text) && text[end] != '\n' {
		end++
	}

	off := min(i+UTF16ToByte(text[i:end], pos.Character), len(text))

	return off
}
