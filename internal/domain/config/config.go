// Package config models the schema of gooze's configuration file. It is a pure
// domain package describing valid keys, value kinds, enums, defaults and docs;
// it performs no YAML parsing and has no LSP dependency.
//
// The schema is pinned from gooze's own viper keys
// (gooze.dev/pkg/gooze/cmd/config.go). Note that gooze's canonical config file
// is named gooze.yaml (viper config name "gooze", type yaml); the README's
// ".gooze.yml" is an alias we also recognise.
package config

import (
	"strconv"
	"strings"
)

// Kind is the value type expected for a scalar field.
type Kind int

const (
	// KindObject is a mapping with named child fields (no scalar value).
	KindObject Kind = iota
	// KindString is a free-form string.
	KindString
	// KindBool is a boolean.
	KindBool
	// KindInt is an integer.
	KindInt
	// KindStringList is a YAML sequence of strings.
	KindStringList
	// KindEnum is a string constrained to Field.Enum (also accepts an integer
	// when IntEnumOK is set, matching gooze's numeric slog levels).
	KindEnum
)

// Field describes one configuration key.
type Field struct {
	Key       string
	Kind      Kind
	Doc       string
	Default   string
	Enum      []string // for KindEnum
	IntEnumOK bool     // KindEnum also accepts a bare integer
	Children  []Field  // for KindObject
}

// Schema is the set of top-level configuration fields.
var Schema = []Field{
	{Key: "version", Kind: KindInt, Default: "1", Doc: "Config schema version."},
	{Key: "output", Kind: KindString, Default: ".gooze-reports", Doc: "Directory where mutation reports are written."},
	{Key: "no-cache", Kind: KindBool, Default: "false", Doc: "Disable incremental caching and force a full re-run."},
	{Key: "paths", Kind: KindObject, Doc: "Path selection settings.", Children: []Field{
		{Key: "exclude", Kind: KindStringList, Default: "[]", Doc: "Regular-expression patterns of files to skip (e.g. '^vendor/')."},
	}},
	{Key: "run", Kind: KindObject, Doc: "Mutation run settings.", Children: []Field{
		{Key: "parallel", Kind: KindInt, Default: "1", Doc: "Number of concurrent mutation workers."},
		{Key: "mutation_timeout", Kind: KindInt, Default: "120", Doc: "Per-mutation timeout in seconds."},
	}},
	{Key: "log", Kind: KindObject, Doc: "Logging settings.", Children: []Field{
		{Key: "filename", Kind: KindString, Default: ".gooze.log", Doc: "Log file path."},
		{Key: "level", Kind: KindEnum, Default: "info", IntEnumOK: true, Enum: []string{"debug", "info", "warn", "error"}, Doc: "Log severity. Accepts debug/info/warn/error or a numeric slog level."},
		{Key: "verbose", Kind: KindBool, Default: "false", Doc: "Enable debug-level logging."},
		{Key: "max_size", Kind: KindInt, Default: "10", Doc: "Max log size in MiB before rotation."},
		{Key: "max_backups", Kind: KindInt, Default: "3", Doc: "Number of rotated log files to retain."},
		{Key: "max_age", Kind: KindInt, Default: "28", Doc: "Days to keep rotated logs."},
		{Key: "compress", Kind: KindBool, Default: "true", Doc: "Gzip rotated logs."},
	}},
}

// ConfigFileNames are the basenames gooze (and this server) recognise as the
// configuration file.
var ConfigFileNames = []string{"gooze.yaml", "gooze.yml", ".gooze.yaml", ".gooze.yml"}

// IsConfigFile reports whether the given file basename is a gooze config file.
func IsConfigFile(base string) bool {
	for _, n := range ConfigFileNames {
		if base == n {
			return true
		}
	}

	return false
}

// ChildrenAt returns the valid child fields at the given key path (empty path =
// top level). It returns nil when the path does not resolve to an object.
func ChildrenAt(path []string) []Field {
	fields := Schema
	for _, key := range path {
		f, ok := find(fields, key)
		if !ok || f.Kind != KindObject {
			return nil
		}

		fields = f.Children
	}

	return fields
}

// Lookup resolves a full key path to its field.
func Lookup(path []string) (Field, bool) {
	if len(path) == 0 {
		return Field{}, false
	}

	fields := Schema
	for i, key := range path {
		f, ok := find(fields, key)
		if !ok {
			return Field{}, false
		}

		if i == len(path)-1 {
			return f, true
		}

		if f.Kind != KindObject {
			return Field{}, false
		}

		fields = f.Children
	}

	return Field{}, false
}

func find(fields []Field, key string) (Field, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f, true
		}
	}

	return Field{}, false
}

// ValidateScalar checks a raw (unquoted, trimmed) scalar value against a field's
// kind, returning a human-readable problem or "" when valid. It is only meaningful
// for scalar kinds; objects and lists return "".
func (f Field) ValidateScalar(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	switch f.Kind {
	case KindBool:
		if !isBool(raw) {
			return "expected a boolean (true or false)"
		}
	case KindInt:
		if _, err := strconv.Atoi(raw); err != nil {
			return "expected an integer"
		}
	case KindEnum:
		return f.validateEnum(raw)
	case KindObject, KindString, KindStringList:
		// No scalar validation applies to these kinds.
	}

	return ""
}

// validateEnum checks raw against an enum field's allowed values, accepting a
// bare integer when the field permits numeric values.
func (f Field) validateEnum(raw string) string {
	if f.IntEnumOK {
		if _, err := strconv.Atoi(raw); err == nil {
			return ""
		}
	}

	if !contains(f.Enum, strings.ToLower(raw)) {
		return "expected one of: " + strings.Join(f.Enum, ", ")
	}

	return ""
}

func isBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false":
		return true
	}

	return false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}

	return false
}
