package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func TestEnvOr(t *testing.T) {
	const key = "GOOZELS_TEST_ENVOR"
	t.Setenv(key, "set-value")
	if got := envOr(key, "fallback"); got != "set-value" {
		t.Errorf("envOr with set var = %q, want set-value", got)
	}
	t.Setenv(key, "")
	if got := envOr(key, "fallback"); got != "fallback" {
		t.Errorf("envOr with empty var = %q, want fallback", got)
	}
}

func TestParseServe(t *testing.T) {
	t.Setenv("GOOZELS_GOPLS", "")
	t.Setenv("GOOZELS_GOOZE", "/env/gooze")

	f, err := parseServe([]string{"-gopls", "/x/gopls", "serve", "--", "-rpc.trace", "serve"}, io.Discard)
	if err != nil || f.gopls != "/x/gopls" || f.gooze != "/env/gooze" || strings.Join(f.goplsArgs, " ") != "-rpc.trace serve" {
		t.Errorf("parseServe = %+v, %v", f, err)
	}

	for _, args := range [][]string{{"bogus"}, {"serve", "extra"}, {"-unknown"}} {
		if _, err := parseServe(args, io.Discard); err == nil {
			t.Errorf("parseServe(%q) should fail", args)
		}
	}
	if _, err := parseServe([]string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: err = %v, want flag.ErrHelp", err)
	}
}

func TestIsTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if isTerminal(r) {
		t.Error("a pipe is not a terminal")
	}
}

func TestRunDispatch(t *testing.T) {
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"help"}, 0},
		{[]string{"--help"}, 0},
		{[]string{"version"}, 0},
		{[]string{"check"}, 2},             // no files
		{[]string{"hook"}, 2},              // no agent
		{[]string{"bogus"}, 2},             // unknown argument
		{[]string{"-gopls", "x", "-h"}, 0}, // help from the server's flags
		{[]string{"-gopls", "/nonexistent/gopls-xyz"}, 1},
	} {
		if got := run(c.args); got != c.want {
			t.Errorf("run(%q) = %d, want %d", c.args, got, c.want)
		}
	}
}

func TestParseServeSeparatorAndNames(t *testing.T) {
	f, err := parseServe([]string{"--", "-rpc.trace"}, io.Discard)
	if err != nil || strings.Join(f.goplsArgs, " ") != "-rpc.trace" {
		t.Errorf("a leading -- should pass everything to gopls: %+v, %v", f, err)
	}
	for _, arg := range []string{"aaa", "zzz"} { // sorting below and above "serve"
		if _, err := parseServe([]string{arg}, io.Discard); err == nil {
			t.Errorf("parseServe(%q) should fail", arg)
		}
	}
}

func TestIsTerminalCharDevice(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = f.Close() }()
	if !isTerminal(f) {
		t.Errorf("%s is a character device", os.DevNull)
	}
	_ = f.Close()
	if isTerminal(f) {
		t.Error("a file that cannot be stat'ed is not a terminal")
	}
}
