// Package goozecli runs the gooze command-line tool on behalf of editor
// commands. It is an interface adapter over an external program.
package goozecli

import (
	"context"
	"os/exec"
	"regexp"
)

// Runner executes the gooze binary.
type Runner struct {
	// Path is the gooze binary to invoke (defaults to "gooze" on PATH).
	Path string
}

// New returns a Runner for the given binary path, defaulting to "gooze".
func New(path string) Runner {
	if path == "" {
		path = "gooze"
	}

	return Runner{Path: path}
}

// Run executes gooze with args in dir and returns its combined output. A non-nil
// error means a non-zero exit (output still holds whatever was printed).
func (r Runner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	// The gooze path and args are operator-controlled (flags/env/commands), not
	// arbitrary user input.
	cmd := exec.CommandContext(ctx, r.Path, args...) //nolint:gosec
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	return string(out), err
}

// longFlag matches a flag line of gooze's help ("  -p, --parallel int ...")
// and captures the long name. Only line starts count, so flags a description
// mentions are not mistaken for supported ones.
var longFlag = regexp.MustCompile(`(?m)^\s*(?:-\w, )?--([a-z0-9][a-z0-9-]*)`)

// RunFlags lists the long flags `gooze run --help` prints, so callers can
// tell what the installed gooze supports, e.g. "run" for --run.
func (r Runner) RunFlags(ctx context.Context) (map[string]bool, error) {
	// The gooze path is operator-controlled (flags/env); the arguments are fixed.
	out, err := exec.CommandContext(ctx, r.Path, "run", "--help").Output() //nolint:gosec
	if err != nil {
		return nil, err
	}

	flags := map[string]bool{}
	for _, m := range longFlag.FindAllStringSubmatch(string(out), -1) {
		flags[m[1]] = true
	}

	return flags, nil
}
