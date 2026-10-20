// Package gopls launches and manages the gopls child process whose LSP stream
// goozels proxies. It is an interface adapter over an external program.
package gopls

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// Process is a running gopls instance with its stdio streams exposed.
type Process struct {
	cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
}

// Start launches gopls at the given path (default "gopls" when empty) with the
// given extra args, wiring its stderr to the parent's for diagnostics.
func Start(path string, args ...string) (*Process, error) {
	if path == "" {
		path = "gopls"
	}

	// The gopls path and args are operator-controlled (flags/env), not user input.
	cmd := exec.CommandContext(context.Background(), path, args...) //nolint:gosec
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &Process{cmd: cmd, Stdin: stdin, Stdout: stdout}, nil
}

// Wait blocks until gopls exits.
func (p *Process) Wait() error { return p.cmd.Wait() }

// Kill terminates gopls.
func (p *Process) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}

	return p.cmd.Process.Kill()
}
