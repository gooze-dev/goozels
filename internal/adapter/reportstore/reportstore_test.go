package reportstore

import (
	"os"
	"path/filepath"
	"testing"

	"gooze.dev/goozels/internal/domain/report"
)

const sampleReport = `source:
  origin:
    shortpath: ./x.go
    fullpath: /proj/x.go
result:
- name: arithmetic
  mutations:
  - mutationid: a
    status: 0
  - mutationid: b
    status: 1
`

func writeReports(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go.yaml"), []byte(sampleReport), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	// _index.yaml must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "_index.yaml"), []byte("total_mutations: 2\n"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	return dir
}

func TestLoad(t *testing.T) {
	reports, err := New(writeReports(t)).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("loaded %d reports, want 1 (index must be skipped)", len(reports))
	}
	s := report.Summarize(reports[0])
	if s.Total != 2 || s.Killed != 1 || s.Survived != 1 {
		t.Errorf("summary = %+v", s)
	}
}

func TestLoadMissingDir(t *testing.T) {
	reports, err := New(filepath.Join(t.TempDir(), "nope")).Load()
	if err != nil || reports != nil {
		t.Errorf("missing dir = %v, %v; want nil, nil", reports, err)
	}
}

func TestReportForFile(t *testing.T) {
	store := New(writeReports(t))

	if _, ok := store.ReportForFile("/proj/x.go"); !ok {
		t.Error("exact FullPath match failed")
	}
	if _, ok := store.ReportForFile("/other/root/x.go"); !ok {
		t.Error("suffix (shortpath) match failed")
	}
	if _, ok := store.ReportForFile("/proj/other.go"); ok {
		t.Error("unrelated file should not match")
	}
}
