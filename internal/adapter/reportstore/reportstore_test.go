package reportstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gooze.dev/pkg/goozels/internal/domain/report"
)

// testdata/reports holds real gooze output (one file per mutation, plus
// _index.yaml) with the workspace root rewritten to /proj: three mutations of
// internal/adapter/test_runner_proc_unix.go (1 survived, 2 not covered) and one
// survived mutation of internal/domain/ignore.go.
const (
	procUnix = "/proj/internal/adapter/test_runner_proc_unix.go"
	ignoreGo = "/proj/internal/domain/ignore.go"
)

// copyFixtures copies testdata/reports into a temp dir so tests can modify it.
func copyFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir("testdata/reports")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("testdata/reports", e.Name()))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	return dir
}

func TestLoadMergesPerMutationReports(t *testing.T) {
	reports, err := New("testdata/reports").Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("loaded %d file reports, want 2 (one per source file, index skipped)", len(reports))
	}

	r, ok := findReport(reports, procUnix)
	if !ok {
		t.Fatal("no report for test_runner_proc_unix.go")
	}
	s := report.Summarize(r)
	if s.Total != 3 || s.Survived != 1 || s.NotCovered != 2 {
		t.Errorf("summary = %+v, want 3 mutations: 1 survived, 2 not covered", s)
	}
	for _, mu := range r.Mutations {
		if mu.Mutagen == "" || len(mu.Diff) == 0 {
			t.Errorf("mutation %s lost its mutagen or diff: %+v", mu.ID, mu)
		}
	}
}

func TestSurvivorMarkersFromRealReports(t *testing.T) {
	r, ok := New("testdata/reports").ReportForFile(procUnix)
	if !ok {
		t.Fatal("no report")
	}

	lines := map[int]string{}
	for _, d := range report.SurvivorDiagnostics(r) {
		lines[d.Range.Start.Line] = d.Message
	}
	// The boolean mutant flips Setpgid on 1-based line 11; both unary mutants
	// change the sign on line 13 (hunks start three lines earlier).
	if _, ok := lines[10]; !ok {
		t.Errorf("missing marker on 0-based line 10: %v", lines)
	}
	if _, ok := lines[12]; !ok {
		t.Errorf("missing marker on 0-based line 12: %v", lines)
	}
	if len(lines) != 2 {
		t.Errorf("markers on %d lines, want 2: %v", len(lines), lines)
	}
}

func TestLoadDeduplicatesMutations(t *testing.T) {
	dir := copyFixtures(t)
	data, err := os.ReadFile(filepath.Join(dir, "1b62976940c4e038.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "copy.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	r, ok := New(dir).ReportForFile(ignoreGo)
	if !ok || len(r.Mutations) != 1 {
		t.Errorf("ignore.go mutations = %d, want 1 (same mutation ID in two files)", len(r.Mutations))
	}
}

func TestLoadSkipsUnreadableFiles(t *testing.T) {
	dir := copyFixtures(t)
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("source: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}

	reports, err := New(dir).Load()
	if err != nil || len(reports) != 2 {
		t.Errorf("Load = %d reports, %v; want 2, nil", len(reports), err)
	}
}

func TestLoadMissingDir(t *testing.T) {
	reports, err := New(filepath.Join(t.TempDir(), "nope")).Load()
	if err != nil || reports != nil {
		t.Errorf("missing dir = %v, %v; want nil, nil", reports, err)
	}
}

func TestReportForFileMatching(t *testing.T) {
	store := New("testdata/reports")

	cases := []struct {
		path string
		want bool
	}{
		{procUnix, true}, // exact FullPath
		{"/elsewhere/checkout/internal/adapter/test_runner_proc_unix.go", true}, // ShortPath on another machine
		{"/proj/internal/adapter/xtest_runner_proc_unix.go", false},             // suffix without a path boundary
		{"/proj/internal/adapter/other.go", false},
	}
	for _, c := range cases {
		if _, ok := store.ReportForFile(c.path); ok != c.want {
			t.Errorf("ReportForFile(%q) found = %v, want %v", c.path, ok, c.want)
		}
	}
}

func TestWorkspaceCachesUntilIndexChanges(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, DirName)
	if err := os.Rename(copyFixtures(t), dir); err != nil {
		t.Fatal(err)
	}
	ws := NewWorkspace()

	if r, ok := ws.ReportForFile(root, ignoreGo); !ok || report.Summarize(r).Survived != 1 {
		t.Fatalf("initial load = %+v, %v", r, ok)
	}

	// Rewrite the survivor as killed without touching the index or directory:
	// the cached result is still served.
	path := filepath.Join(dir, "1b62976940c4e038.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	killed := strings.Replace(string(data), "status: 1", "status: 0", 1)
	if err := os.WriteFile(path, []byte(killed), 0o600); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, dirInfo.ModTime(), dirInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if r, _ := ws.ReportForFile(root, ignoreGo); report.Summarize(r).Survived != 1 {
		t.Error("expected the cached report before the index changes")
	}

	// gooze regenerates _index.yaml after every run; that invalidates the cache.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(dir, indexFileName), later, later); err != nil {
		t.Fatal(err)
	}
	if r, _ := ws.ReportForFile(root, ignoreGo); report.Summarize(r).Killed != 1 {
		t.Errorf("after index change summary = %+v, want the mutation killed", report.Summarize(r))
	}
}
