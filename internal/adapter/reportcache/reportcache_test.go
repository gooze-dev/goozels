package reportcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gooze.dev/pkg/gooze/pkg/report"
)

// countingReader records how often the wrapped reader is asked for a directory.
type countingReader struct {
	report.Reader
	reads int
}

func (c *countingReader) ReadDir(string) ([]report.Report, error) {
	c.reads++
	return []report.Report{{}}, nil
}

func TestReadDirCachesUntilIndexChanges(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, report.IndexFileName)
	if err := os.WriteFile(index, []byte("total_mutations: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	next := &countingReader{}
	r := New(next)

	for range 3 {
		if got, err := r.ReadDir(dir); err != nil || len(got) != 1 {
			t.Fatalf("ReadDir = %v, %v", got, err)
		}
	}
	if next.reads != 1 {
		t.Errorf("reads = %d, want 1 (later calls served from the cache)", next.reads)
	}

	// gooze regenerates _index.yaml after every run; that invalidates the cache.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(index, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadDir(dir); err != nil {
		t.Fatal(err)
	}
	if next.reads != 2 {
		t.Errorf("reads after index change = %d, want 2", next.reads)
	}
}

func TestReadDirMissingDirPassesThrough(t *testing.T) {
	next := &countingReader{}
	r := New(next)
	missing := filepath.Join(t.TempDir(), "nope")
	for range 2 {
		if _, err := r.ReadDir(missing); err != nil {
			t.Fatal(err)
		}
	}
	if next.reads != 2 {
		t.Errorf("reads = %d, want 2 (nothing to cache without a directory)", next.reads)
	}
}

func TestReadDirAgainstGoozeReader(t *testing.T) {
	dir := t.TempDir()
	r := New(report.NewDirReader())
	if got, err := r.ReadDir(dir); err != nil || len(got) != 0 {
		t.Errorf("empty dir = %v, %v; want no reports", got, err)
	}
}
