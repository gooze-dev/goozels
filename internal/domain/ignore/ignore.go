// Package ignore implements the gooze:ignore feature logic: completion, hover
// and diagnostics for //gooze:ignore directives in Go source. It is pure and
// LSP-agnostic, depending only on the directive and mutagen domain packages.
package ignore

import (
	"strings"

	"gooze.dev/pkg/goozels/internal/domain/directive"
	"gooze.dev/pkg/goozels/internal/domain/mutagen"
	"gooze.dev/pkg/goozels/internal/model"
)

const generalDoc = "`gooze:ignore` skips mutation generation. With no names it ignores every mutagen; otherwise list comma-separated mutagen names to skip."

// Complete returns mutagen completions when col sits inside a directive's name
// list on the given line. It returns nil otherwise.
func Complete(line string, col int) []model.CompletionItem {
	cur, ok := directive.CursorAt(line, col)
	if !ok {
		return nil
	}

	prefix := strings.ToLower(cur.Prefix)

	var items []model.CompletionItem

	for _, mg := range mutagen.All {
		if !strings.HasPrefix(mg.Name, prefix) {
			continue
		}

		items = append(items, model.CompletionItem{
			Label:        mg.Name,
			Detail:       mg.Detail,
			Doc:          mg.Doc,
			InsertText:   mg.Name,
			ReplaceStart: cur.TokenStart,
			ReplaceEnd:   cur.TokenEnd,
		})
	}

	return items
}

// Hover returns documentation when col is over a directive keyword or one of its
// mutagen names on the given line.
func Hover(line string, lineNum, col int) (model.Hover, bool) {
	d, ok := directive.ParseLine(line)
	if !ok {
		return model.Hover{}, false
	}

	for _, n := range d.Names {
		if col < n.Start || col > n.End {
			continue
		}

		rng := model.Range{
			Start: model.Position{Line: lineNum, Column: n.Start},
			End:   model.Position{Line: lineNum, Column: n.End},
		}
		if mg, ok := mutagen.Lookup(n.Text); ok {
			return model.Hover{Markdown: "**gooze mutagen `" + mg.Name + "`**\n\n" + mg.Doc, Range: rng}, true
		}

		return model.Hover{
			Markdown: "Unknown gooze mutagen `" + n.Text + "`.\n\nValid mutagens: " + strings.Join(mutagen.Names(), ", "),
			Range:    rng,
		}, true
	}
	// Over the keyword itself (between the comment marker and the name list).
	if col >= d.CommentStart && col <= d.ListStart {
		return model.Hover{
			Markdown: generalDoc,
			Range: model.Range{
				Start: model.Position{Line: lineNum, Column: d.CommentStart},
				End:   model.Position{Line: lineNum, Column: d.ListStart},
			},
		}, true
	}

	return model.Hover{}, false
}

// Diagnose scans the whole document for directives that reference unknown
// mutagen names and returns a warning for each.
func Diagnose(text string) []model.Diagnostic {
	var diags []model.Diagnostic

	for lineNum, line := range model.Lines(text) {
		d, ok := directive.ParseLine(line)
		if !ok {
			continue
		}

		for _, n := range d.Names {
			if mutagen.IsValid(n.Text) {
				continue
			}

			diags = append(diags, model.Diagnostic{
				Range: model.Range{
					Start: model.Position{Line: lineNum, Column: n.Start},
					End:   model.Position{Line: lineNum, Column: n.End},
				},
				Severity: model.SeverityWarning,
				Message:  "unknown gooze mutagen \"" + n.Text + "\"; valid: " + strings.Join(mutagen.Names(), ", "),
				Source:   "gooze",
			})
		}
	}

	return diags
}
