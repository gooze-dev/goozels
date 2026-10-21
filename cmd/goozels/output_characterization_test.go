package main

import (
	"os"
	"path/filepath"
	"testing"

	"gooze.dev/pkg/goozels/internal/adapter/hookstate"
)

type charCase struct{ name, stdout, stderr string }

// charCases runs goozels check and the hook on the testdata project the ways
// their output varies: a source, a capped source, a test file, a stale source,
// and the hook's first and repeated reply.
func charCases(t *testing.T) []charCase {
	t.Helper()

	var cases []charCase

	run := func(name string, args ...string) {
		_, out, notes := check(t, args...)
		cases = append(cases, charCase{name, out, notes})
	}

	// Run from inside each project, so paths print relative and stay the same.
	// Both are copied first: the copy reads testdata relative to this package.
	dir, stale := projectWithReports(t), projectWithReports(t)
	t.Chdir(dir)

	src := filepath.Join("calc", "calc.go")

	run("source", src)
	run("capped", "-max", "2", src)
	run("test file", filepath.Join("calc", "calc_test.go"))

	state := hookstate.New(t.TempDir())
	hook := func(name string) {
		cases = append(cases, charCase{name, hookContext(t, "claude", hookPayload(t, "s", dir, src), state), ""})
	}
	hook("hook first")
	hook("hook again")

	t.Chdir(stale)

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, append(data, []byte("\n// edited\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	run("stale source", src)

	return cases
}

// TestOutputCharacterization pins goozels check and hook output byte for byte,
// so refactoring how results are assembled cannot change what users see.
func TestOutputCharacterization(t *testing.T) {
	const (
		summary = "calc/calc.go:1:1: warning: gooze: score 43.75% — 7 killed, 3 survived, 6 not covered by any test"
		line11  = "calc/calc.go:11:2: warning: gooze: 3 surviving mutants (comparison ×2, numbers) — tests still pass with `return n > 0` → `return n != 0`, `return n >= 0`, `return n > 1`"
		line16  = "calc/calc.go:16:2: warning: gooze: 6 mutants not covered by any test (arithmetic ×4, numbers ×2) — `return n * 2` → `return n % 2`, `return n + 2`, `return n - 2`, `return n / 2`, `return n * 0` and 1 more"
		hint    = "gooze: a mutant that cannot change behavior (an equivalent mutant) can be skipped with //gooze:ignore <mutagen> on its line"
	)

	want := map[string]string{
		"source":       summary + "\n" + line11 + "\n" + line16 + "\n",
		"capped":       summary + "\n" + line11 + "\n… 1 more; run goozels check calc/calc.go\n" + hint + "\n",
		"test file":    summary + "\n" + line11 + "\n" + line16 + "\n",
		"hook first":   "gooze results from the last run:\n" + summary + "\n" + line11 + "\n" + line16 + "\n" + hint,
		"hook again":   "gooze results from the last run:\n" + summary + briefSuffix,
		"stale source": "calc/calc.go:1:1: warning: gooze: results are from before calc.go changed; score then 43.75%; rerun: gooze run ./calc\n" + line11 + "\n" + line16 + "\n",
	}

	for _, c := range charCases(t) {
		if c.stdout != want[c.name] {
			t.Errorf("%s: output changed:\n%s\nwant:\n%s", c.name, c.stdout, want[c.name])
		}

		if c.stderr != "" {
			t.Errorf("%s: unexpected notes: %q", c.name, c.stderr)
		}
	}
}
