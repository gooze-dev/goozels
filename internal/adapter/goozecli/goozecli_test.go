package goozecli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/domain/command"
)

func TestNewDefaultsToGooze(t *testing.T) {
	if New("").Path != "gooze" {
		t.Errorf("New(\"\").Path = %q, want gooze", New("").Path)
	}
	if New("/custom/gooze").Path != "/custom/gooze" {
		t.Error("New did not keep custom path")
	}
}

func TestRunCapturesOutput(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not available")
	}
	out, err := New("echo").Run(context.Background(), t.TempDir(), "hello", "world")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Errorf("Run output = %q, want 'hello world'", out)
	}
}

func TestRunErrorOnMissingBinary(t *testing.T) {
	if _, err := New("/nonexistent/gooze-xyz").Run(context.Background(), t.TempDir(), "list"); err == nil {
		t.Error("expected error running a missing binary")
	}
}

// writeHelpStub writes a gooze stand-in that prints help for `run --help`
// and records its arguments.
func writeHelpStub(t *testing.T, help string) (path, argsFile string) {
	t.Helper()

	dir := t.TempDir()
	path = filepath.Join(dir, "gooze")
	argsFile = filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$@\" >" + argsFile + "\ncat <<'EOF'\n" + help + "\nEOF\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // the stub must be executable
		t.Fatal(err)
	}

	return path, argsFile
}

func TestRunFlagsParsesHelp(t *testing.T) {
	help := `Usage:
  gooze run [paths...] [flags]

Flags:
      --diff string               git ref to diff against (requires git)
      --estimate                  list source files and applicable mutation counts
      --mutagens strings          mutagens to run, comma-separated
  -p, --parallel int              number of parallel workers (default 1)
      --run string                mutate only functions matching this regular expression, as for go test -run

Global Flags:
      --no-cache                  disable cached incremental runs`

	path, argsFile := writeHelpStub(t, help)

	flags, err := New(path).RunFlags(context.Background())
	if err != nil {
		t.Fatalf("RunFlags: %v", err)
	}

	for _, name := range []string{"diff", "estimate", "mutagens", "parallel", "run", "no-cache"} {
		if !flags[name] {
			t.Errorf("flag %q not found in %v", name, flags)
		}
	}

	if flags["p"] || flags["string"] || flags[""] {
		t.Errorf("only long flag names are listed, got %v", flags)
	}

	if args, _ := os.ReadFile(argsFile); strings.TrimSpace(string(args)) != "run --help" {
		t.Errorf("gooze was called with %q, want \"run --help\"", args)
	}
}

func TestRunFlagsOfAnOlderGooze(t *testing.T) {
	path, _ := writeHelpStub(t, "Flags:\n      --estimate   list files\n  -p, --parallel int   workers")

	flags, err := New(path).RunFlags(context.Background())
	if err != nil {
		t.Fatalf("RunFlags: %v", err)
	}

	if flags["run"] {
		t.Errorf("an older gooze has no --run, got %v", flags)
	}
}

func TestRunFlagsMissingBinary(t *testing.T) {
	if _, err := New("/nonexistent/gooze-xyz").RunFlags(context.Background()); err == nil {
		t.Error("expected an error for a missing gooze")
	}
}

func TestRunBelowAThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooze")
	script := "#!/bin/sh\necho 'Mutation score: 50.00% (1 killed, 1 survived)'\necho 'Error: mutation score 50.00% is below threshold.total 90%' >&2\nexit 3\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // the stub must be executable
		t.Fatal(err)
	}

	out, err := New(path).Run(context.Background(), dir, "run", "./...")
	if !errors.Is(err, command.ErrBelowThreshold) {
		t.Errorf("exit 3 should be ErrBelowThreshold, got %v", err)
	}
	if !strings.Contains(out, "Mutation score: 50.00%") || !strings.Contains(out, "below threshold.total 90%") {
		t.Errorf("output should be kept, got %q", out)
	}

	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil { //nolint:gosec // the stub must be executable
		t.Fatal(err)
	}
	if _, err := New(path).Run(context.Background(), dir, "run"); err == nil || errors.Is(err, command.ErrBelowThreshold) {
		t.Errorf("other exits are plain failures, got %v", err)
	}
}
