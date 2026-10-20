package controller

import (
	"path/filepath"
	"strings"

	"gooze.dev/pkg/goozels/internal/domain/config"
	"gooze.dev/pkg/goozels/internal/domain/goozeyml"
	"gooze.dev/pkg/goozels/internal/domain/ignore"
	"gooze.dev/pkg/goozels/internal/domain/report"
	"gooze.dev/pkg/goozels/internal/model"
)

// Checker computes the gooze diagnostics the language server would publish for a
// file, for callers outside an LSP session: the `goozels check` command and
// coding-agent hooks that run after an edit.
type Checker struct {
	Reports Reports
}

// Check returns gooze diagnostics for the file at absPath whose contents are
// text. workdir is the workspace root holding gooze's reports directory. Files
// that are neither Go sources nor gooze config files yield nil.
func (c Checker) Check(workdir, absPath, text string) []model.Diagnostic {
	switch {
	case config.IsConfigFile(filepath.Base(absPath)):
		return goozeyml.Diagnose(text)
	case strings.HasSuffix(absPath, ".go"):
		return append(ignore.Diagnose(text), reportDiagnostics(c.Reports, workdir, absPath)...)
	default:
		return nil
	}
}

// Handles reports whether Check produces diagnostics for files like absPath, so
// callers can skip reading files it would ignore.
func (Checker) Handles(absPath string) bool {
	return strings.HasSuffix(absPath, ".go") || config.IsConfigFile(filepath.Base(absPath))
}

// reportDiagnostics returns gooze report-derived diagnostics for a .go file: a
// summary plus a marker per surviving or uncovered mutant. It returns nil when
// no report exists.
func reportDiagnostics(reports Reports, workdir, absPath string) []model.Diagnostic {
	rep, ok := reports.ReportForFile(workdir, absPath)
	if !ok {
		return nil
	}

	survivors := report.SurvivorDiagnostics(rep)
	diags := make([]model.Diagnostic, 0, len(survivors)+1)
	diags = append(diags, report.SummaryDiagnostic(rep))

	return append(diags, survivors...)
}
