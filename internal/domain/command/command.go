// Package command holds the gooze actions goozels can execute from the editor
// (running and listing mutations) and the code lenses that surface them. It is a
// pure domain package: it names commands and builds lens models, but does not run
// anything.
package command

import "gooze.dev/goozels/internal/model"

// Command identifiers advertised to the client and dispatched by the controller.
const (
	Run  = "gooze.run"
	List = "gooze.list"
	View = "gooze.view"
)

// Subcommand maps a command identifier to the gooze CLI subcommand it runs.
// The second result is false for unknown identifiers.
func Subcommand(id string) (string, bool) {
	switch id {
	case Run:
		return "run", true
	case List:
		return "list", true
	case View:
		return "view", true
	}

	return "", false
}

// Names returns every command identifier goozels handles.
func Names() []string { return []string{Run, List, View} }

// IsGooze reports whether id is one of goozels' commands.
func IsGooze(id string) bool {
	_, ok := Subcommand(id)
	return ok
}

// GoFileLenses returns the lenses shown at the top of a Go file. pathArg is the
// package pattern passed to gooze (e.g. "./...").
func GoFileLenses(pathArg string) []model.CodeLens {
	top := model.Range{
		Start: model.Position{Line: 0, Column: 0},
		End:   model.Position{Line: 0, Column: 0},
	}

	return []model.CodeLens{
		{Range: top, Command: model.Command{Title: "▶ gooze run", Command: Run, Arguments: []any{pathArg}}},
		{Range: top, Command: model.Command{Title: "gooze list", Command: List, Arguments: []any{pathArg}}},
		{Range: top, Command: model.Command{Title: "📊 gooze report", Command: View}},
	}
}
