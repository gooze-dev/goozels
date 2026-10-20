package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/model"
)

// workspace writes a project with a gooze config typo and a bad directive.
func workspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":     "module x\n",
		"gooze.yaml": "run:r\n",
		"x.go":       "package x\n\nvar y = 1 //gooze:ignore bogus\n",
		"clean.go":   "package x\n",
		"README.md":  "run:r\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// projectWithReports copies testdata's sample project and the gooze reports a
// real run produced for it into a temp workspace.
func projectWithReports(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "project"))); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(dir, ".gooze-reports"), os.DirFS(filepath.Join("..", "..", "testdata", "reports"))); err != nil {
		t.Fatal(err)
	}
	return dir
}

func check(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runCheck(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunCheck(t *testing.T) {
	dir := workspace(t)
	args := []string{"-workdir", dir}
	for _, f := range []string{"gooze.yaml", "x.go", "clean.go", "README.md"} {
		args = append(args, filepath.Join(dir, f))
	}

	code, got, notes := check(t, args...)
	if code != 1 {
		t.Errorf("exit = %d, want 1 for the printed error diagnostics", code)
	}
	if !strings.Contains(got, "gooze.yaml:1:1: error: missing a space after") {
		t.Errorf("missing config error in:\n%s", got)
	}
	if !strings.Contains(got, "x.go:3:") || !strings.Contains(got, "bogus") {
		t.Errorf("missing directive error in:\n%s", got)
	}
	if strings.Contains(got, "clean.go") || strings.Contains(got, "README.md") {
		t.Errorf("clean and non-gooze files should print no diagnostics:\n%s", got)
	}
	if !strings.Contains(notes, "goozels: no gooze results for "+filepath.Join(dir, "clean.go")+" yet; run: ") ||
		strings.Contains(notes, "README.md") || strings.Contains(notes, "gooze.yaml") {
		t.Errorf("stderr should note the Go files without results only, got:\n%s", notes)
	}
}

func TestRunCheckExitsZeroForWarnings(t *testing.T) {
	dir := projectWithReports(t)
	if code, got, _ := check(t, filepath.Join(dir, "calc", "calc.go")); code != 0 || !strings.Contains(got, "warning: gooze: score") {
		t.Errorf("exit = %d, output:\n%s\nwant 0 and the warnings", code, got)
	}
}

func TestRunCheckErrors(t *testing.T) {
	if code, _, _ := check(t); code != 2 {
		t.Errorf("no files: exit = %d, want 2", code)
	}
	if code, _, _ := check(t, "-max", "-1", "x.go"); code != 2 {
		t.Errorf("negative -max: exit = %d, want 2", code)
	}
	if code, _, _ := check(t, filepath.Join(t.TempDir(), "missing.go")); code != 1 {
		t.Errorf("missing file: exit = %d, want 1", code)
	}
}

func TestRunCheckFindsWorkdir(t *testing.T) {
	dir := projectWithReports(t)
	t.Chdir(t.TempDir()) // not the project: the workdir comes from the file

	_, got, _ := check(t, filepath.Join(dir, "calc", "calc.go"))
	if !strings.Contains(got, "calc.go:1:1: warning: gooze: score 43.75% — 7 killed, 3 survived, 6 not covered by any test") {
		t.Errorf("check from another directory should find the reports, got:\n%s", got)
	}
}

func TestRunCheckTestFileShowsTestedSource(t *testing.T) {
	dir := projectWithReports(t)
	t.Chdir(dir)

	_, got, _ := check(t, filepath.Join("calc", "calc_test.go"))
	want := filepath.Join("calc", "calc.go") + ":1:1: warning: gooze: score 43.75%"
	if !strings.Contains(got, want) || !strings.Contains(got, "surviving") {
		t.Errorf("checking the test file should report calc.go's score and survivors, got:\n%s", got)
	}
}

func TestRunCheckStaleSource(t *testing.T) {
	dir := projectWithReports(t)
	t.Chdir(dir)
	src := filepath.Join("calc", "calc.go")
	text, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, append([]byte("// Package calc.\n"), text...), 0o600); err != nil {
		t.Fatal(err)
	}

	_, got, _ := check(t, src)
	want := src + ":1:1: warning: gooze: results are from before calc.go changed; score then 43.75%; rerun: gooze run ./calc\n"
	if got != want {
		t.Errorf("a changed source should print only the stale notice, got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRunCheckStaleTest(t *testing.T) {
	dir := projectWithReports(t)
	test := filepath.Join(dir, "calc", "calc_test.go")
	f, err := os.OpenFile(test, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\nfunc TestMore(t *testing.T) {}\n")
	_ = f.Close()
	t.Chdir(dir)

	_, got, _ := check(t, test)
	if !strings.Contains(got, "gooze: tests changed since the last run (calc_test.go); score then 43.75%; rerun: gooze run ./calc") {
		t.Errorf("an edited test should mark the results out of date, got:\n%s", got)
	}
}

func TestRunCheckMax(t *testing.T) {
	dir := projectWithReports(t)
	t.Chdir(dir)

	_, got, _ := check(t, "-max", "1", filepath.Join("calc", "calc.go"))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 || lines[1] != "… 2 more; run goozels check calc/calc.go" || !strings.Contains(lines[2], "//gooze:ignore <mutagen>") {
		t.Errorf("-max 1 output:\n%s\nwant one line, the cap line and the ignore hint", got)
	}
}

func TestFindWorkdir(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "mod", "a", "b")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := findWorkdir(deep, "fallback"); got != "fallback" {
		t.Errorf("no markers: %q, want the fallback", got)
	}
	if err := os.WriteFile(filepath.Join(root, "mod", "go.mod"), []byte("module m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := findWorkdir(deep, "fallback"); got != filepath.Join(root, "mod") {
		t.Errorf("go.mod only: %q, want its directory", got)
	}
	if err := os.Mkdir(filepath.Join(root, ".gooze-reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := findWorkdir(deep, "fallback"); got != root {
		t.Errorf("reports above go.mod: %q, want the reports' directory", got)
	}
}

func TestShownPath(t *testing.T) {
	for _, c := range []struct{ base, abs, want string }{
		{"/w", "/w/a/b.go", "a/b.go"},
		{"/w", "/other/b.go", "/other/b.go"},
		{"", "/w/b.go", "/w/b.go"},
	} {
		if got := shownPath(c.base, c.abs); got != filepath.FromSlash(c.want) {
			t.Errorf("shownPath(%q, %q) = %q, want %q", c.base, c.abs, got, c.want)
		}
	}
}

func TestFormatDiagnostic(t *testing.T) {
	d := model.Diagnostic{
		Range:    model.Range{Start: model.Position{Line: 4, Column: 2}},
		Severity: model.SeverityWarning,
		Message:  "m",
	}
	if got := formatDiagnostic("a/b.go", d); got != "a/b.go:5:3: warning: m" {
		t.Errorf("formatDiagnostic = %q", got)
	}
}

func TestRunCheckSourceAndItsTestOnce(t *testing.T) {
	dir := projectWithReports(t)
	t.Chdir(dir)

	_, got, _ := check(t, filepath.Join("calc", "calc.go"), filepath.Join("calc", "calc_test.go"))
	if n := strings.Count(got, "calc/calc.go:1:1:"); n != 1 {
		t.Errorf("calc.go is reported %d times, want once:\n%s", n, got)
	}
}

func TestRunCheckNoNoteWhenResultsExist(t *testing.T) {
	dir := projectWithReports(t)
	for _, f := range []string{"calc.go", "calc_test.go"} {
		if _, _, notes := check(t, filepath.Join(dir, "calc", f)); notes != "" {
			t.Errorf("%s has results, but stderr says:\n%s", f, notes)
		}
	}
}

func TestRunCheckCleanPrintsNothing(t *testing.T) {
	dir := workspace(t)
	if code, got, _ := check(t, filepath.Join(dir, "clean.go"), filepath.Join(dir, "missing.md")); code != 0 || got != "" {
		t.Errorf("exit = %d, stdout = %q; want 0 and nothing (other files are not even read)", code, got)
	}
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

func TestRunCheckWriteFailure(t *testing.T) {
	dir := projectWithReports(t)
	var errOut bytes.Buffer
	if code := runCheck([]string{filepath.Join(dir, "calc", "calc.go")}, failingWriter{}, &errOut); code != 1 {
		t.Errorf("exit = %d, want 1 when stdout cannot be written", code)
	}
}

func TestRunCheckWorkdirFlagWins(t *testing.T) {
	dir := projectWithReports(t)
	_, got, notes := check(t, "-workdir", t.TempDir(), filepath.Join(dir, "calc", "calc.go"))
	if got != "" || !strings.Contains(notes, "no gooze results for") {
		t.Errorf("-workdir without reports should win over the discovered one; stdout %q, stderr %q", got, notes)
	}
}

// writeReport adds a minimal gooze report for source, tested by test, to
// dir's reports directory.
func writeReport(t *testing.T, dir, name, source, test string) {
	t.Helper()
	body := "source:\n  origin:\n    shortpath: " + source + "\n  test:\n    shortpath: " + test +
		"\nresult:\n  - name: arithmetic\n    version: 1\n    mutations:\n      - mutationid: m\n        status: 0\n"
	if err := os.MkdirAll(filepath.Join(dir, ".gooze-reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gooze-reports", name+".yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunCheckTestFileCoversSourcesOfAnyName(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a_test.go", "b.go", "0.go"} { // sources sorting above and below the test
		if err := os.WriteFile(filepath.Join(dir, f), []byte("package x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeReport(t, dir, "1", "b.go", "a_test.go")
	writeReport(t, dir, "2", "0.go", "a_test.go")
	t.Chdir(dir)

	_, got, _ := check(t, "a_test.go")
	if !strings.Contains(got, "b.go:1:1: info: gooze: score 100.00% — 1 killed") || !strings.Contains(got, "0.go:1:1:") {
		t.Errorf("checking a_test.go should show both sources it tests:\n%s", got)
	}
}

func TestFindWorkdirNearestModule(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{root, inner} {
		if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module m\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := findWorkdir(inner, "fallback"); got != inner {
		t.Errorf("findWorkdir = %q, want the nearest module %q", got, inner)
	}
}

func TestShownPathOutsideBase(t *testing.T) {
	for _, c := range []struct{ base, abs, want string }{
		{"/w", "/w/-x/a.go", "-x/a.go"}, // "-x" sorts below ".."
		{"/w", "/", "/"},                // the parent itself
		{"/w", "/w/..x/a.go", "..x/a.go"},
	} {
		if got := shownPath(c.base, c.abs); got != filepath.FromSlash(c.want) {
			t.Errorf("shownPath(%q, %q) = %q, want %q", c.base, c.abs, got, c.want)
		}
	}
}

func TestRunCheckNoteForUnmutatedFileOfTestedPackage(t *testing.T) {
	dir := projectWithReports(t)
	if err := os.WriteFile(filepath.Join(dir, "calc", "types.go"), []byte("package calc\n\ntype T int\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, _, notes := check(t, filepath.Join("calc", "types.go"))
	want := "goozels: no gooze results for calc/types.go: gooze found nothing to mutate in it, or it is newer than the last run of ./calc\n"
	if notes != want {
		t.Errorf("stderr = %q, want %q", notes, want)
	}
}

func TestRunCheckNoteForNewTestFile(t *testing.T) {
	dir := projectWithReports(t)
	if err := os.WriteFile(filepath.Join(dir, "calc", "extra_test.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, _, notes := check(t, filepath.Join("calc", "extra_test.go"))
	if want := "goozels: no gooze results for calc/extra_test.go yet; run: gooze run ./calc\n"; notes != want {
		t.Errorf("stderr = %q, want %q", notes, want)
	}
}
