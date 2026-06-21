package lsp

import "gooze.dev/pkg/goozels/internal/model"

// ByteColumn converts an LSP UTF-16 character offset on a line to a byte column,
// for feeding request positions into the domain.
func ByteColumn(line string, character int) int { return utf16ToByte(line, character) }

// CompletionResult builds an LSP completion list from domain items, all of which
// apply to lineNum/line. kind is the LSP CompletionItemKind to tag them with.
func CompletionResult(lineNum int, line string, kind int, items []model.CompletionItem) CompletionList {
	out := CompletionList{Items: make([]CompletionItem, 0, len(items))}
	for _, it := range items {
		ci := CompletionItem{
			Label:      it.Label,
			Kind:       kind,
			Detail:     it.Detail,
			InsertText: it.InsertText,
			TextEdit: &TextEdit{
				Range: Range{
					Start: Position{Line: lineNum, Character: byteToUTF16(line, it.ReplaceStart)},
					End:   Position{Line: lineNum, Character: byteToUTF16(line, it.ReplaceEnd)},
				},
				NewText: it.InsertText,
			},
		}
		if it.Doc != "" {
			ci.Documentation = &MarkupContent{Kind: "markdown", Value: it.Doc}
		}

		out.Items = append(out.Items, ci)
	}

	return out
}

// HoverResult builds an LSP hover from a domain hover, using lines to convert
// byte columns to UTF-16.
func HoverResult(lines []string, h model.Hover) Hover {
	rng := rangeFromModel(lines, h.Range)

	return Hover{
		Contents: MarkupContent{Kind: "markdown", Value: h.Markdown},
		Range:    &rng,
	}
}

// Diagnostics converts domain diagnostics to LSP diagnostics for a document.
func Diagnostics(text string, ds []model.Diagnostic) []Diagnostic {
	lines := model.Lines(text)

	out := make([]Diagnostic, 0, len(ds))
	for _, d := range ds {
		out = append(out, Diagnostic{
			Range:    rangeFromModel(lines, d.Range),
			Severity: int(d.Severity),
			Source:   d.Source,
			Message:  d.Message,
		})
	}

	return out
}

// CodeLenses converts domain code lenses to LSP code lenses for a document.
func CodeLenses(lines []string, ls []model.CodeLens) []CodeLens {
	out := make([]CodeLens, 0, len(ls))
	for _, l := range ls {
		out = append(out, CodeLens{
			Range: rangeFromModel(lines, l.Range),
			Command: &Command{
				Title:     l.Command.Title,
				Command:   l.Command.Command,
				Arguments: l.Command.Arguments,
			},
		})
	}

	return out
}

func rangeFromModel(lines []string, r model.Range) Range {
	return Range{
		Start: positionFromModel(lines, r.Start),
		End:   positionFromModel(lines, r.End),
	}
}

func positionFromModel(lines []string, p model.Position) Position {
	line := ""
	if p.Line >= 0 && p.Line < len(lines) {
		line = lines[p.Line]
	}

	return Position{Line: p.Line, Character: byteToUTF16(line, p.Column)}
}
