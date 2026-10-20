// Package reportcache caches parsed gooze reports per directory. It is an
// interface adapter that wraps any report.Reader: a large project has thousands
// of report files and the language server republishes diagnostics often, so
// re-reading the directory every time is too slow.
package reportcache

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gooze.dev/pkg/gooze/pkg/report"
)

// Reader is a report.Reader whose ReadDir results are reused until gooze
// changes the directory. WalkDir and ReadIndex pass through uncached.
type Reader struct {
	next report.Reader

	mu    sync.Mutex
	cache map[string]entry
}

type entry struct {
	stamp   stamp
	reports []report.Report
}

// stamp identifies one version of a reports directory: adding or removing a
// report changes the directory's mtime, and every gooze run that rewrites
// reports regenerates the index file.
type stamp struct {
	dirMod   time.Time
	indexMod time.Time
	indexLen int64
}

// New returns a caching Reader in front of next.
func New(next report.Reader) *Reader {
	return &Reader{next: next, cache: map[string]entry{}}
}

// ReadDir returns the reports in dir, from the cache when the directory has
// not changed since the last read.
func (r *Reader) ReadDir(dir string) ([]report.Report, error) {
	st, ok := stampOf(dir)
	if !ok {
		return r.next.ReadDir(dir)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if c, hit := r.cache[dir]; hit && c.stamp == st {
		return c.reports, nil
	}

	reports, err := r.next.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	r.cache[dir] = entry{stamp: st, reports: reports}

	return reports, nil
}

// WalkDir implements report.Reader without caching.
func (r *Reader) WalkDir(dir string, fn func(path string, rep report.Report) error) error {
	return r.next.WalkDir(dir, fn)
}

// ReadIndex implements report.Reader without caching.
func (r *Reader) ReadIndex(dir string) (report.Index, error) {
	return r.next.ReadIndex(dir)
}

func stampOf(dir string) (stamp, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return stamp{}, false
	}

	st := stamp{dirMod: info.ModTime()}
	if idx, err := os.Stat(filepath.Join(dir, report.IndexFileName)); err == nil {
		st.indexMod, st.indexLen = idx.ModTime(), idx.Size()
	}

	return st, true
}
