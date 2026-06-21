package goozecli

import (
	"context"
	"os/exec"
	"strings"
	"testing"
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
