package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gooze.dev/pkg/gooze/pkg/config"
	"gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/adapter/osfs"
	"gooze.dev/pkg/goozels/internal/adapter/reportcache"
	"gooze.dev/pkg/goozels/internal/controller"
	"gooze.dev/pkg/goozels/internal/model"
)

const checkUsage = "usage: goozels check [-workdir DIR] [-max N] FILE..."

// newChecker wires the checker to gooze's report reader and config schema, as
// the language server does. Rerun commands are given for a reader in cwd.
func newChecker(cwd string) controller.Checker {
	return controller.Checker{
		Reports: reportcache.New(report.NewDirReader()),
		Config:  config.NewRegistry(),
		Files:   osfs.FS{},
		Cwd:     cwd,
	}
}

// runCheck implements `goozels check [-workdir DIR] [-max N] FILE...`: it
// prints gooze diagnostics as "path:line:col: severity: message" lines. A Go
// file gooze has no results for gets a note on stderr. It returns 2 on usage
// errors, 1 when an error diagnostic was printed or a file cannot be read, and
// 0 otherwise.
func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workdir := fs.String("workdir", "", "workspace root containing gooze's "+config.DefaultReportsDir+
		" directory (default: the nearest directory above each file that has one, else its go.mod directory)")
	maxLines := fs.Int("max", 0, "print at most N diagnostics, then a line naming the files with more (0: no limit)")

	// Equivalent: NArg counts arguments and is never negative, so <= 0 is == 0.
	if err := fs.Parse(args); err != nil || fs.NArg() == 0 || *maxLines < 0 { //gooze:ignore comparison
		_, _ = fmt.Fprintln(stderr, checkUsage)

		return 2
	}

	cwd, _ := os.Getwd()
	checker := newChecker(cwd)
	status := 0

	var entries []entry

	for _, file := range fs.Args() {
		found, err := checkFile(checker, *workdir, cwd, file)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "goozels: %v\n", err)

			status = 1
		}

		entries = append(entries, found...)
	}

	out := render(uniqueEntries(entries), renderOptions{maxLines: *maxLines})
	for _, n := range out.notes {
		_, _ = fmt.Fprintln(stderr, "goozels: "+n)
	}

	if len(out.lines) > 0 {
		if _, err := fmt.Fprintln(stdout, strings.Join(out.lines, "\n")); err != nil {
			return 1
		}
	}

	if out.errors {
		status = 1
	}

	return status
}

// entry is what checking found for one file: its diagnostics, or a note that
// gooze has no results for it.
type entry struct {
	abs   string
	shown string
	check controller.FileCheck
	note  string
	brief bool // show only the summary: the full results were already shown
}

// checkFile checks one file, plus the source files it tests when it is a
// _test.go file. Paths are shown relative to base when they are inside it.
// workdir, when empty, is found from the file's directory (see findWorkdir).
func checkFile(checker controller.Checker, workdir, base, file string) ([]entry, error) {
	abs, err := filepath.Abs(file)
	if err != nil || !checker.Handles(abs) {
		return nil, err
	}

	text, err := os.ReadFile(abs) //nolint:gosec // the caller names the file to check
	if err != nil {
		return nil, err
	}

	root := workdir
	if root == "" {
		root = findWorkdir(filepath.Dir(abs), base)
	}

	if root, err = filepath.Abs(root); err != nil {
		return nil, err
	}

	e := entry{abs: abs, shown: shownPath(base, abs), check: checker.Check(root, abs, string(text))}
	if !strings.HasSuffix(abs, ".go") {
		return []entry{e}, nil
	}

	tested := testedSources(checker, root, base, abs)
	if e.check.Results == nil && !hasResults(tested) {
		e.note = noResultsNote(e)
	}

	return append([]entry{e}, tested...), nil
}

// noResultsNote says that gooze has no results for a Go file: not yet, or,
// when other files of its package have results, none because it had nothing
// to mutate or is newer than the run.
func noResultsNote(e entry) string {
	if e.check.PackageTested && !strings.HasSuffix(e.abs, "_test.go") {
		return "no gooze results for " + e.shown + ": gooze found nothing to mutate in it, or it is newer than the last run of " +
			e.check.Package
	}

	return "no gooze results for " + e.shown + " yet; run: " + e.check.Rerun
}

// testedSources checks the source files a test file covers, since gooze
// reports scores and surviving mutants per source file. Writing tests is how
// mutants get killed, so these are the targets for the file just edited.
// Unreadable sources are skipped.
func testedSources(checker controller.Checker, root, base, abs string) []entry {
	var entries []entry

	for _, src := range checker.SourcesForTest(root, abs) {
		if src == abs || strings.HasSuffix(src, "_test.go") {
			continue
		}

		if found, err := checkFile(checker, root, base, src); err == nil {
			entries = append(entries, found...)
		}
	}

	return entries
}

// uniqueEntries drops repeated checks of the same file, such as a source named
// next to its test, keeping the first.
func uniqueEntries(entries []entry) []entry {
	seen := map[string]bool{}
	unique := entries[:0]

	for _, e := range entries {
		if !seen[e.abs] {
			seen[e.abs] = true

			unique = append(unique, e)
		}
	}

	return unique
}

func hasResults(entries []entry) bool {
	for _, e := range entries {
		if e.check.Results != nil {
			return true
		}
	}

	return false
}

// findWorkdir returns the nearest directory at or above dir that holds gooze's
// reports directory, else the nearest one with a go.mod, else fallback.
func findWorkdir(dir, fallback string) string {
	module := ""

	for d := dir; ; d = filepath.Dir(d) {
		if isDir(filepath.Join(d, config.DefaultReportsDir)) {
			return d
		}

		if module == "" && exists(filepath.Join(d, "go.mod")) {
			module = d
		}

		// Equivalent: a parent's path is a prefix of its child's and sorts below it, so >= holds only at the root, like ==.
		if filepath.Dir(d) == d { //gooze:ignore comparison
			break
		}
	}

	if module != "" {
		return module
	}

	return fallback
}

func isDir(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// shownPath is abs relative to base when it lies inside base, else abs.
func shownPath(base, abs string) string {
	// Equivalent: an empty base that gets past this reaches filepath.Rel, which rejects it for an absolute path, so abs is returned either way.
	if base == "" { //gooze:ignore comparison
		return abs
	}

	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}

	return rel
}

// formatDiagnostic renders a diagnostic in the compiler-style form editors and
// agents already parse, with one-based line and column.
func formatDiagnostic(path string, d model.Diagnostic) string {
	return path + ":" + strconv.Itoa(d.Range.Start.Line+1) + ":" + strconv.Itoa(d.Range.Start.Column+1) +
		": " + d.Severity.String() + ": " + d.Message
}
