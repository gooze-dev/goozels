package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var gotArgs []string
	gopls := func(stdout, _ io.Writer, args ...string) error {
		gotArgs = args
		_, _ = io.WriteString(stdout, `{"GoVersion":"go1.25"}`)
		return nil
	}

	var out, errOut bytes.Buffer
	if code := runVersion(nil, &out, &errOut, gopls); code != 0 || !strings.HasPrefix(out.String(), "goozels ") || strings.Count(out.String(), "\n") != 1 {
		t.Errorf("version: exit %d, output %q; want one line starting with \"goozels \"", code, out.String())
	}

	out.Reset()
	if code := runVersion([]string{"-json"}, &out, &errOut, gopls); code != 0 || out.String() != `{"GoVersion":"go1.25"}` || strings.Join(gotArgs, " ") != "-json" {
		t.Errorf("version -json: exit %d, output %q, gopls args %q", code, out.String(), gotArgs)
	}

	failing := func(io.Writer, io.Writer, ...string) error { return errors.New("no gopls") }
	if code := runVersion([]string{"-json"}, &out, &errOut, failing); code != 1 {
		t.Errorf("version -json without gopls: exit %d, want 1", code)
	}
	if code := runVersion([]string{"extra"}, &out, &errOut, gopls); code != 2 {
		t.Errorf("version extra: exit %d, want 2", code)
	}
}

func TestUsageListsModes(t *testing.T) {
	for _, want := range []string{"goozels check", "goozels hook claude|codex", "goozels version", "GOOZELS_GOPLS", "GOOZELS_GOOZE", "-max N"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
	if version() == "" {
		t.Error("version is empty")
	}
}

func TestRunVersionRejectsOtherArgs(t *testing.T) {
	gopls := func(io.Writer, io.Writer, ...string) error { return nil }
	for _, args := range [][]string{{"-json", "x"}, {"-a"}, {"-z"}} {
		var out, errOut bytes.Buffer
		if code := runVersion(args, &out, &errOut, gopls); code != 2 {
			t.Errorf("version %q: exit %d, want 2", args, code)
		}
	}
}

func TestVersionInTests(t *testing.T) {
	// Test binaries carry no module version, so the fallback applies.
	if got := version(); got != "(devel)" {
		t.Errorf("version() = %q, want (devel)", got)
	}
}
