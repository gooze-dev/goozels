// Package goozecli runs the gooze command-line tool on behalf of editor
// commands. It is an interface adapter over an external program.
package goozecli

import (
	"context"
	"os/exec"
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
