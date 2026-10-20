// Package command holds the gooze actions goozels can execute from the editor
// (running mutation testing, estimating it, viewing reports) and the code
// lenses that surface them. It is a
// pure domain package: it names commands and builds lens models, but does not run
// anything.
package command

import (
	"path/filepath"
	"strings"

	"gooze.dev/pkg/goozels/internal/model"
)

// Command identifiers advertised to the client and dispatched by the controller.
const (
	Run      = "gooze.run"
	Estimate = "gooze.estimate"
	View     = "gooze.view"

	// legacyList is the identifier goozels advertised for the removed
	// `gooze list` command. It still runs the estimate, which replaced it, so
	// editor configs that call it keep working.
	legacyList = "gooze.list"
)

// defaultPattern is the package pattern used when a command gets none.
const defaultPattern = "./..."

// Args returns the gooze CLI arguments a command identifier runs. pathArg is the
// package pattern for the commands that take one; empty means "./...". The
// second result is false for unknown identifiers.
func Args(id, pathArg string) ([]string, bool) {
	if pathArg == "" {
		pathArg = defaultPattern
	}

	switch id {
	case Run:
		return []string{"run", pathArg}, true
	case Estimate, legacyList:
		return []string{"run", "--estimate", pathArg}, true
	case View:
		return []string{"report", "view"}, true
	default:
		return nil, false
	}
}

// Names returns every command identifier goozels advertises.
func Names() []string { return []string{Run, Estimate, View} }

// IsGooze reports whether id is one of goozels' commands.
func IsGooze(id string) bool {
	_, ok := Args(id, "")
	return ok
}

// PackagePattern returns the gooze path pattern that selects the package of
// the Go file at absPath, relative to workdir (where gooze runs): "./calc" for
// workdir/calc/calc.go, "." for a file in workdir itself, and "./..." when the
// file is outside workdir.
func PackagePattern(workdir, absPath string) string {
	rel, err := filepath.Rel(workdir, filepath.Dir(absPath))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return defaultPattern
	}

	if rel == "." {
		return rel
	}

	return "./" + filepath.ToSlash(rel)
}

// RunCommandLine is the shell command that reruns gooze for a pattern.
func RunCommandLine(pattern string) string {
	return "gooze run " + pattern
}

// Lens titles, kept plain (no icons) so screen readers announce them cleanly.
const (
	RunTitle      = "gooze: run package"
	EstimateTitle = "gooze: estimate"
	ViewTitle     = "gooze: report"
)

// GoFileLenses returns the lenses shown at the top of a Go file. pathArg is the
// package pattern passed to gooze (see PackagePattern).
func GoFileLenses(pathArg string) []model.CodeLens {
	top := model.Range{
		Start: model.Position{Line: 0, Column: 0},
		End:   model.Position{Line: 0, Column: 0},
	}

	return []model.CodeLens{
		{Range: top, Command: model.Command{Title: RunTitle, Command: Run, Arguments: []any{pathArg}}},
		{Range: top, Command: model.Command{Title: EstimateTitle, Command: Estimate, Arguments: []any{pathArg}}},
		{Range: top, Command: model.Command{Title: ViewTitle, Command: View}},
	}
}
