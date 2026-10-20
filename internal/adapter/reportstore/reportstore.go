// Package reportstore reads gooze's on-disk mutation reports (the YAML files gooze
// writes into its output directory) and converts them to the domain's report
// model. It is an interface adapter over the filesystem.
//
// gooze writes one report file per mutation, named after a hash of the mutation,
// plus an _index.yaml summary. This package groups those files by source file.
package reportstore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"gooze.dev/pkg/goozels/internal/domain/report"
)

const indexFileName = "_index.yaml"

// DirName is gooze's default reports directory, relative to the workspace.
const DirName = ".gooze-reports"

// Workspace finds reports under a workspace's default reports directory. It
// implements the controller's Reports port. Parsed reports are cached per
// directory until gooze rewrites the directory or its index, because a large
// project has thousands of report files and diagnostics are republished often.
type Workspace struct {
	mu    sync.Mutex
	cache map[string]cachedDir
}

type cachedDir struct {
	stamp   dirStamp
	reports []report.FileReport
}

// dirStamp identifies one version of a reports directory: adding or removing a
// report changes the directory's mtime, and every gooze run that rewrites
// reports regenerates _index.yaml.
type dirStamp struct {
	dirMod   time.Time
	indexMod time.Time
	indexLen int64
}

// NewWorkspace returns a Workspace with an empty cache.
func NewWorkspace() *Workspace { return &Workspace{cache: map[string]cachedDir{}} }

// ReportForFile returns the merged report for the Go file at absPath, read from
// <workdir>/.gooze-reports.
func (w *Workspace) ReportForFile(workdir, absPath string) (report.FileReport, bool) {
	return findReport(w.load(filepath.Join(workdir, DirName)), absPath)
}

func (w *Workspace) load(dir string) []report.FileReport {
	stamp, ok := stampOf(dir)
	if !ok {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if c, hit := w.cache[dir]; hit && c.stamp == stamp {
		return c.reports
	}

	reports, err := New(dir).Load()
	if err != nil {
		return nil
	}

	w.cache[dir] = cachedDir{stamp: stamp, reports: reports}

	return reports
}

func stampOf(dir string) (dirStamp, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return dirStamp{}, false
	}

	stamp := dirStamp{dirMod: info.ModTime()}
	if idx, err := os.Stat(filepath.Join(dir, indexFileName)); err == nil {
		stamp.indexMod, stamp.indexLen = idx.ModTime(), idx.Size()
	}

	return stamp, true
}

// Store loads reports from a gooze reports directory.
type Store struct {
	Dir string
}

// New returns a Store reading from dir (typically <workspace>/.gooze-reports).
func New(dir string) Store { return Store{Dir: dir} }

// fileYAML mirrors gooze's model.File serialization.
type fileYAML struct {
	ShortPath string `yaml:"shortpath"`
	FullPath  string `yaml:"fullpath"`
}

type sourceYAML struct {
	Origin *fileYAML `yaml:"origin"`
}

type mutationYAML struct {
	MutationID string `yaml:"mutationid"`
	Status     int    `yaml:"status"`
}

type entryYAML struct {
	Name      string         `yaml:"name"`
	Mutations []mutationYAML `yaml:"mutations"`
}

type reportYAML struct {
	Source sourceYAML  `yaml:"source"`
	Result []entryYAML `yaml:"result"`
	Diff   *[]byte     `yaml:"diff"`
}

// Load reads every report file in the directory and merges them into one report
// per source file. A missing directory yields no reports and no error.
func (s Store) Load() ([]report.FileReport, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	paths := make([]string, 0, len(entries))

	for _, e := range entries {
		if !e.IsDir() && e.Name() != indexFileName && isYAML(e.Name()) {
			paths = append(paths, filepath.Join(s.Dir, e.Name()))
		}
	}

	return merge(readAll(paths)), nil
}

// ReportForFile returns the merged report whose source matches absPath, if any.
func (s Store) ReportForFile(absPath string) (report.FileReport, bool) {
	reports, err := s.Load()
	if err != nil {
		return report.FileReport{}, false
	}

	return findReport(reports, absPath)
}

func findReport(reports []report.FileReport, absPath string) (report.FileReport, bool) {
	for _, r := range reports {
		if matches(r, absPath) {
			return r, true
		}
	}

	return report.FileReport{}, false
}

// readAll parses report files concurrently; YAML decoding dominates the cost of
// loading a large reports directory. Unreadable files come back as ok=false.
func readAll(paths []string) []parsed {
	out := make([]parsed, len(paths))
	next := make(chan int)

	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(paths)) {
		wg.Go(func() {
			for i := range next {
				out[i].report, out[i].ok = read(paths[i])
			}
		})
	}

	for i := range paths {
		next <- i
	}

	close(next)
	wg.Wait()

	return out
}

type parsed struct {
	report report.FileReport
	ok     bool
}

// merge groups per-mutation reports by source file, keeping the first result
// for each mutation ID. Files keep the order in which they first appear.
func merge(all []parsed) []report.FileReport {
	var reports []report.FileReport

	byPath := map[string]int{}
	seen := map[string]bool{}

	for _, p := range all {
		if !p.ok {
			continue
		}

		key := p.report.FullPath + "\x00" + p.report.ShortPath

		i, found := byPath[key]
		if !found {
			i = len(reports)
			byPath[key] = i

			reports = append(reports, report.FileReport{ShortPath: p.report.ShortPath, FullPath: p.report.FullPath})
		}

		for _, mu := range p.report.Mutations {
			if id := key + "\x00" + mu.ID; !seen[id] {
				seen[id] = true

				reports[i].Mutations = append(reports[i].Mutations, mu)
			}
		}
	}

	return reports
}

func read(path string) (report.FileReport, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is within the configured reports dir
	if err != nil {
		return report.FileReport{}, false
	}

	var raw reportYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return report.FileReport{}, false
	}

	if raw.Source.Origin == nil {
		return report.FileReport{}, false
	}

	fr := report.FileReport{
		ShortPath: raw.Source.Origin.ShortPath,
		FullPath:  raw.Source.Origin.FullPath,
	}

	var diff []byte
	if raw.Diff != nil {
		diff = *raw.Diff
	}

	for _, entry := range raw.Result {
		for _, mu := range entry.Mutations {
			fr.Mutations = append(fr.Mutations, report.Mutation{
				ID:      mu.MutationID,
				Mutagen: entry.Name,
				Status:  report.Status(mu.Status),
				Diff:    diff,
			})
		}
	}

	return fr, true
}

// matches reports whether r describes the file at absPath: an exact FullPath
// match, or (for reports produced on another machine) the ShortPath as a whole
// trailing path segment of absPath.
func matches(r report.FileReport, absPath string) bool {
	if r.FullPath == absPath {
		return true
	}

	abs := filepath.ToSlash(absPath)

	short := strings.TrimPrefix(filepath.ToSlash(r.ShortPath), "./")
	if short != "" {
		return abs == short || strings.HasSuffix(abs, "/"+short)
	}

	return r.FullPath != "" && filepath.Base(r.FullPath) == filepath.Base(absPath)
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
