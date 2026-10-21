// Package command holds the gooze actions goozels can execute from the editor
// (running mutation testing, estimating it, viewing reports) and the code
// lenses that surface them. It is a
// pure domain package: it names commands and builds lens models, but does not run
// anything.
package command

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"gooze.dev/pkg/goozels/internal/model"
)

// Command identifiers advertised to the client and dispatched by the controller.
const (
	Run      = "gooze.run"
	Estimate = "gooze.estimate"
	View     = "gooze.view"
	// RunFunction runs gooze on the functions a `--run` pattern matches.
	RunFunction = "gooze.runFunction"

	// legacyList is the identifier goozels advertised for the removed
	// `gooze list` command. It still runs the estimate, which replaced it, so
	// editor configs that call it keep working.
	legacyList = "gooze.list"
)

// ErrBelowThreshold is the error of a gooze command that completed, but with
// a score below a threshold set in gooze.yaml (gooze's exit code 3). Its
// reports are written, so they are shown like a success's.
var ErrBelowThreshold = errors.New("score below a gooze threshold")

// defaultPattern is the package pattern used when a command gets none.
const defaultPattern = "./..."

// runSubcommand is gooze's subcommand for mutation runs and estimates.
const runSubcommand = "run"

// Args returns the gooze CLI arguments a command identifier runs. The first of
// args is the package pattern for the commands that take one (empty means
// "./..."); RunFunction also needs a --run pattern second. The second result is
// false for unknown identifiers and missing patterns.
func Args(id string, args ...string) ([]string, bool) {
	pathArg := defaultPattern
	if len(args) > 0 && args[0] != "" {
		pathArg = args[0]
	}

	switch id {
	case RunFunction:
		if len(args) < 2 || args[1] == "" {
			return nil, false
		}

		return []string{runSubcommand, "--run", args[1], pathArg}, true
	case Run:
		return []string{runSubcommand, pathArg}, true
	case Estimate, legacyList:
		return []string{runSubcommand, "--estimate", pathArg}, true
	case View:
		return []string{"report", "view"}, true
	default:
		return nil, false
	}
}

// Names returns every command identifier goozels advertises.
func Names() []string { return []string{Run, Estimate, View, RunFunction} }

// IsGooze reports whether id is one of goozels' commands, including the
// legacy gooze.list.
func IsGooze(id string) bool {
	return id == legacyList || slices.Contains(Names(), id)
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
	RunTitle         = "gooze: run package"
	EstimateTitle    = "gooze: estimate"
	ViewTitle        = "gooze: report"
	RunFunctionTitle = "gooze: run function"
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

// MergeTitle is the button that folds a function run's reports into the main
// reports.
const MergeTitle = "Merge into reports"

// RunReportsDir is where a narrowed gooze run (a function run among them)
// leaves its reports, inside the reports directory, until they are merged.
const RunReportsDir = "run"

// MergeArgs are the gooze CLI arguments that fold the run directory (and any
// shards) into the main reports.
func MergeArgs() []string { return []string{"report", "merge"} }
