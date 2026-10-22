package gopls

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStartErrorOnMissingBinary(t *testing.T) {
	if _, err := Start("/nonexistent/gopls-xyz"); err == nil {
		t.Error("expected error starting a missing binary")
	}
}

func TestStartAndKill(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	// cat with no args blocks reading stdin, giving us a live process to kill.
	proc, err := Start("cat")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc.Stdin == nil || proc.Stdout == nil {
		t.Fatal("Start did not wire stdio pipes")
	}
	killAndWait(t, proc)
}

func TestStartDefaultsBinaryName(t *testing.T) {
	// An empty path defaults to "gopls"; if gopls is absent Start still attempts
	// it and returns an error rather than panicking.
	if _, err := exec.LookPath("gopls"); err != nil {
		if _, startErr := Start(""); startErr == nil {
			t.Error("expected error when gopls is absent")
		}
		return
	}
	proc, err := Start("")
	if err != nil {
		t.Fatalf("Start(\"\") with gopls present: %v", err)
	}
	killAndWait(t, proc)
}

func TestRun(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("echo not available")
	}
	var out bytes.Buffer
	if err := Run(context.Background(), "echo", &out, io.Discard, "version", "-json"); err != nil || out.String() != "version -json\n" {
		t.Errorf("Run = %q, %v", out.String(), err)
	}
	if err := Run(context.Background(), "/nonexistent/gopls-xyz", io.Discard, io.Discard); err == nil {
		t.Error("expected an error running a missing binary")
	}
}

func TestRunDefaultsBinaryName(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH")
	}
	var out bytes.Buffer
	if err := Run(context.Background(), "", &out, io.Discard, "version"); err != nil || !bytes.Contains(out.Bytes(), []byte("gopls")) {
		t.Errorf("Run with an empty path should run gopls: %q, %v", out.String(), err)
	}
}

// fakeGopls puts a "gopls" script that prints its arguments and then copies
// stdin to stdout on a PATH of its own, so an empty path must resolve to it.
func fakeGopls(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dir := t.TempDir()
	script := "#!/bin/sh\necho \"fake-gopls $*\"\nexec cat\n"
	if err := os.WriteFile(filepath.Join(dir, "gopls"), []byte(script), 0o755); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestStartEmptyPathRunsGoplsFromPath(t *testing.T) {
	fakeGopls(t)

	proc, err := Start("", "serve")
	if err != nil {
		t.Fatalf("Start(\"\"): %v", err)
	}
	defer killAndWait(t, proc)

	line, err := bufio.NewReader(proc.Stdout).ReadString('\n')
	if err != nil || line != "fake-gopls serve\n" {
		t.Errorf("Start(\"\") ran %q, %v; want the gopls on PATH", line, err)
	}
}

func TestRunEmptyPathRunsGoplsFromPath(t *testing.T) {
	fakeGopls(t)

	var out bytes.Buffer
	if err := Run(context.Background(), "", &out, io.Discard, "version"); err != nil || out.String() != "fake-gopls version\n" {
		t.Errorf("Run(\"\") = %q, %v; want the gopls on PATH", out.String(), err)
	}
}

// killAndWait kills proc and fails the test if it is still running after a
// few seconds, instead of blocking forever in Wait.
func killAndWait(t *testing.T, proc *Process) {
	t.Helper()

	if err := proc.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = proc.Wait() // returns a "killed" error; we only care that it returns
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = proc.cmd.Process.Kill()
		t.Fatal("Wait did not return after Kill: the process is still running")
	}
}

func TestKillNeverStartedProcess(t *testing.T) {
	proc := &Process{cmd: exec.CommandContext(context.Background(), "gopls")}
	if err := proc.Kill(); err != nil {
		t.Errorf("Kill on a process that never started = %v, want nil", err)
	}
}
