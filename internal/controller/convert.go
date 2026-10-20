package controller

import (
	"path"

	"gooze.dev/pkg/goozels/internal/domain/config"
	"gooze.dev/pkg/goozels/internal/domain/document"
	"gooze.dev/pkg/goozels/internal/model"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// isConfigURI reports whether the URI names a gooze config file.
func isConfigURI(uri string) bool { return config.IsConfigFile(path.Base(lsp.URIPath(uri))) }

// lspCompletion builds an LSP completion list from domain items, all of which
// apply to lineNum/line. kind is the LSP CompletionItemKind to tag them with.
func lspCompletion(lineNum int, line string, kind int, items []model.CompletionItem) lsp.CompletionList {
	out := lsp.CompletionList{Items: make([]lsp.CompletionItem, 0, len(items))}
	for _, it := range items {
		ci := lsp.CompletionItem{
			Label:      it.Label,
			Kind:       kind,
			Detail:     it.Detail,
			InsertText: it.InsertText,
			TextEdit: &lsp.TextEdit{
				Range: lsp.Range{
					Start: lsp.Position{Line: lineNum, Character: lsp.ByteToUTF16(line, it.ReplaceStart)},
					End:   lsp.Position{Line: lineNum, Character: lsp.ByteToUTF16(line, it.ReplaceEnd)},
				},
				NewText: it.InsertText,
			},
		}
		if it.Doc != "" {
			ci.Documentation = &lsp.MarkupContent{Kind: "markdown", Value: it.Doc}
		}

		out.Items = append(out.Items, ci)
	}

	return out
}

// lspHover builds an LSP hover from a domain hover, using lines to convert
// byte columns to UTF-16.
func lspHover(lines []string, h model.Hover) lsp.Hover {
	rng := rangeFromModel(lines, h.Range)

	return lsp.Hover{
		Contents: lsp.MarkupContent{Kind: "markdown", Value: h.Markdown},
		Range:    &rng,
	}
}

// lspDiagnostics converts domain diagnostics to LSP diagnostics for a document.
func lspDiagnostics(text string, ds []model.Diagnostic) []lsp.Diagnostic {
	lines := document.Lines(text)

	out := make([]lsp.Diagnostic, 0, len(ds))
	for _, d := range ds {
		out = append(out, lsp.Diagnostic{
			Range:    rangeFromModel(lines, d.Range),
			Severity: int(d.Severity),
			Source:   d.Source,
			Message:  d.Message,
		})
	}

	return out
}

// lspCodeLenses converts domain code lenses to LSP code lenses for a document.
func lspCodeLenses(lines []string, ls []model.CodeLens) []lsp.CodeLens {
	out := make([]lsp.CodeLens, 0, len(ls))
	for _, l := range ls {
		out = append(out, lsp.CodeLens{
			Range: rangeFromModel(lines, l.Range),
			Command: &lsp.Command{
				Title:     l.Command.Title,
				Command:   l.Command.Command,
				Arguments: l.Command.Arguments,
			},
		})
	}

	return out
}

func rangeFromModel(lines []string, r model.Range) lsp.Range {
	return lsp.Range{
		Start: positionFromModel(lines, r.Start),
		End:   positionFromModel(lines, r.End),
	}
}

func positionFromModel(lines []string, p model.Position) lsp.Position {
	line := ""
	if p.Line >= 0 && p.Line < len(lines) {
		line = lines[p.Line]
	}

	return lsp.Position{Line: p.Line, Character: lsp.ByteToUTF16(line, p.Column)}
}
