// Package reportstore reads gooze's on-disk mutation reports (the YAML files gooze
// writes into its output directory) and converts them to the domain's report
// model. It is an interface adapter over the filesystem.
package reportstore

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"gooze.dev/goozels/internal/domain/report"
)

const indexFileName = "_index.yaml"

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

// Load reads every report file in the directory. A missing directory yields no
// reports and no error.
func (s Store) Load() ([]report.FileReport, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	var reports []report.FileReport

	for _, e := range entries {
		if e.IsDir() || e.Name() == indexFileName || !isYAML(e.Name()) {
			continue
		}

		r, ok := s.read(filepath.Join(s.Dir, e.Name()))
		if ok {
			reports = append(reports, r)
		}
	}

	return reports, nil
}

// ReportForFile returns the report whose source matches absPath, if any.
func (s Store) ReportForFile(absPath string) (report.FileReport, bool) {
	reports, err := s.Load()
	if err != nil {
		return report.FileReport{}, false
	}

	for _, r := range reports {
		if matches(r, absPath) {
			return r, true
		}
	}

	return report.FileReport{}, false
}

func (s Store) read(path string) (report.FileReport, bool) {
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
	if raw.Diff != nil {
		fr.Diff = *raw.Diff
	}

	for _, entry := range raw.Result {
		for _, mu := range entry.Mutations {
			fr.Mutations = append(fr.Mutations, report.Mutation{
				ID:     mu.MutationID,
				Status: report.Status(mu.Status),
			})
		}
	}

	return fr, true
}

func matches(r report.FileReport, absPath string) bool {
	if r.FullPath == absPath {
		return true
	}

	short := strings.TrimPrefix(r.ShortPath, "./")
	if short != "" && strings.HasSuffix(filepath.ToSlash(absPath), short) {
		return true
	}

	return r.FullPath != "" && filepath.Base(r.FullPath) == filepath.Base(absPath) && short == ""
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
