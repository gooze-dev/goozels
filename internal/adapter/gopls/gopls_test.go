package gopls

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"
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
	if err := proc.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}
	_ = proc.Wait() // returns a "killed" error; we only care that it returns
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
	_ = proc.Kill()
	_ = proc.Wait()
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
