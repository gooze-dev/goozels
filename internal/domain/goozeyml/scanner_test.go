package goozeyml

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gooze.dev/pkg/gooze/pkg/config"

	"gooze.dev/pkg/goozels/internal/model"
)

// TestMain fails the package fast when a scan never returns: completion and
// diagnostics walk a nesting stack, and a stack that stops shrinking would
// otherwise hang the test binary until an outer timeout.
func TestMain(m *testing.M) {
	watchdog := time.AfterFunc(30*time.Second, func() {
		fmt.Fprintln(os.Stderr, "goozeyml tests did not finish within 30s")
		os.Exit(2)
	})

	code := m.Run()

	watchdog.Stop()
	os.Exit(code)
}

// within runs f and fails the test if it doesn't return in time, so a scan
// that loops forever fails fast instead of hanging the test run.
func within(t *testing.T, f func()) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)
		f()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not return within 2s")
	}
}

func complete(t *testing.T, text string, line, col int) []model.CompletionItem {
	t.Helper()

	var items []model.CompletionItem

	within(t, func() { items = newEditor().Complete(text, line, col) })

	return items
}

func diagnose(t *testing.T, text string) []model.Diagnostic {
	t.Helper()

	var diags []model.Diagnostic

	within(t, func() { diags = newEditor().Diagnose(text) })

	return diags
}

func messages(diags []model.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Message)
	}

	return out
}

func TestIsWordByte(t *testing.T) {
	const word = "-_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	for c := range 256 {
		want := strings.IndexByte(word, byte(c)) >= 0
		if got := isWordByte(byte(c)); got != want {
			t.Errorf("isWordByte(%q) = %v, want %v", rune(c), got, want)
		}
	}
}

func TestStripQuotes(t *testing.T) {
	tests := map[string]string{
		"":        "",
		"a":       "a",
		"'":       "'",
		`"`:       `"`,
		"''":      "",
		`""`:      "",
		"'a'":     "a",
		`"a"`:     "a",
		`'a"`:     `'a"`,
		`"a'`:     `"a'`,
		"'ab":     "'ab",
		"ab'":     "ab'",
		`"ab`:     `"ab`,
		`ab"`:     `ab"`,
		"!a'":     "!a'", // '!' sorts before both quotes
		"'a!":     "'a!",
		`!a"`:     `!a"`,
		`"a!`:     `"a!`,
		`'a b'`:   "a b",
		`"a'b"`:   "a'b",
		"abc":     "abc",
		"'''":     "'",
		`"x" # c`: `"x" # c`,
	}

	for in, want := range tests {
		if got := stripQuotes(in); got != want {
			t.Errorf("stripQuotes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMinInt(t *testing.T) {
	tests := []struct{ a, b, want int }{
		{1, 2, 1},
		{2, 1, 1},
		{2, 2, 2},
		{-1, 0, -1},
	}

	for _, tt := range tests {
		if got := minInt(tt.a, tt.b); got != tt.want {
			t.Errorf("minInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSeparatorColon(t *testing.T) {
	tests := map[string]int{
		"":         -1,
		"a":        -1,
		":":        0,
		"a:":       1,
		"a: ":      1,
		"a:\t":     1,
		"a: b":     1,
		"a:\tb":    1,
		"a:b":      -1,
		"a :b":     -1,
		"a:b: c":   3,
		"a: b: c":  1,
		"a::":      2,
		"1 a: b":   3,  // digits sort before ':'
		"a:\rb":    -1, // control characters below ' ' and '\t' don't end a key
		"a:\x01b":  -1,
		"http://x": -1,
	}

	for in, want := range tests {
		if got := separatorColon(in); got != want {
			t.Errorf("separatorColon(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseEntryEdges(t *testing.T) {
	tests := []struct {
		line   string
		ok     bool
		indent int
		key    string
		value  string
	}{
		{line: "\tkey: 1", ok: true, indent: 0, key: "key", value: "1"}, // only spaces indent
		{line: `"output": x`, ok: true, key: `"output"`, value: "x"},
		{line: "a#b: 1", ok: true, key: "a#b", value: "1"},
		{line: "a-b: 1", ok: true, key: "a-b", value: "1"},
		{line: "!x: 1", ok: true, key: "!x", value: "1"},
		{line: "key:", ok: true, key: "key", value: ""},
		{line: "key: v # note", ok: true, key: "key", value: "v"},
		{line: "    deep: 1", ok: true, indent: 4, key: "deep", value: "1"},
		{line: "x", ok: false},
		{line: "#", ok: false},
		{line: "-", ok: false},
		{line: ": x", ok: false},
		{line: "  : x", ok: false},
		{line: "\t: x", ok: false},
		{line: "", ok: false},
		{line: "   ", ok: false},
	}

	for _, tt := range tests {
		e, ok := parseEntry(0, tt.line)
		if ok != tt.ok {
			t.Errorf("parseEntry(%q) ok = %v, want %v (entry %+v)", tt.line, ok, tt.ok, e)
			continue
		}

		if !ok {
			continue
		}

		if e.indent != tt.indent || e.key != tt.key || e.value != tt.value || e.keyStart != tt.indent {
			t.Errorf("parseEntry(%q) = %+v, want indent %d key %q value %q", tt.line, e, tt.indent, tt.key, tt.value)
		}
	}
}

func TestLeadingSpaces(t *testing.T) {
	tests := map[string]int{"": 0, "x": 0, "  x": 2, "   ": 3, "\tx": 0, " \tx": 1}
	for in, want := range tests {
		if got := leadingSpaces(in); got != want {
			t.Errorf("leadingSpaces(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestWordBounds(t *testing.T) {
	tests := []struct {
		line           string
		lo, col        int
		wantS, wantEnd int
	}{
		{"outp", 0, 2, 0, 4},
		{"outp", 0, 4, 0, 4},
		{"a-b_c9 x", 0, 3, 0, 6},
		{"x  yz", 3, 4, 3, 5},
		{"ab cd", 3, 3, 3, 5},
		{"ab: cd", 0, 3, 3, 3},
	}

	for _, tt := range tests {
		s, e := wordBounds(tt.line, tt.lo, tt.col)
		if s != tt.wantS || e != tt.wantEnd {
			t.Errorf("wordBounds(%q, %d, %d) = %d, %d, want %d, %d", tt.line, tt.lo, tt.col, s, e, tt.wantS, tt.wantEnd)
		}
	}
}

func TestCompleteAndHoverLineOutOfRange(t *testing.T) {
	text := "output: x\n"
	n := len(strings.Split(text, "\n"))

	for _, line := range []int{-1, n, n + 1} {
		if items := complete(t, text, line, 0); items != nil {
			t.Errorf("Complete line %d = %v, want nil", line, labels(items))
		}

		if h, ok := newEditor().Hover(text, line, 0); ok {
			t.Errorf("Hover line %d = %+v, want none", line, h)
		}
	}
}

func TestCompleteKeysAfterNestingAndDedent(t *testing.T) {
	tests := []struct {
		name string
		text string
		line int
		col  int
		want []string // must be offered
		not  []string // must not be offered
	}{
		{
			name: "child of a section after another section",
			text: "run:\n  parallel: 1\nlog:\n  le",
			line: 3, col: 4,
			want: []string{"level"},
		},
		{
			name: "third child after two siblings",
			text: "run:\n  parallel: 1\n  mutation_timeout: 5\n  co",
			line: 3, col: 4,
			want: []string{"coverage_profile"},
		},
		{
			name: "top level after a nested child",
			text: "run:\n  parallel: 1\nou",
			line: 2, col: 2,
			want: []string{"output"},
		},
		{
			name: "top level right after an empty section",
			text: "run:\nno",
			line: 1, col: 2,
			want: []string{"no-cache", "no-update-check"},
		},
		{
			name: "sibling already present is left out",
			text: "run:\n  parallel: 1\n  pa",
			line: 2, col: 4,
			not: []string{"parallel"},
		},
		{
			name: "same name at a shallower indent is not a sibling",
			text: "threshold:\n  total: 1\n  diff:\n    to",
			line: 3, col: 6,
			want: []string{"total"},
		},
		{
			name: "same name at a deeper indent is not a sibling",
			text: "threshold:\n  diff:\n    total: 1\n  to",
			line: 3, col: 4,
			want: []string{"total"},
		},
		{
			name: "three levels deep",
			text: "threshold:\n  diff:\n    total: 1\n    av",
			line: 3, col: 6,
			want: []string{"average"},
		},
		{
			name: "same name in another section is not a sibling",
			text: "run:\n  diff: origin/main\nthreshold:\n  di",
			line: 3, col: 4,
			want: []string{"diff"},
		},
		{
			name: "same names in a sibling section are not siblings",
			text: "threshold:\n  diff:\n    total: 1\n    average: 1\n    file: 1\n  ",
			line: 5, col: 2,
			want: []string{"total", "average", "file"},
			not:  []string{"diff"},
		},
		{
			name: "a repeated section only counts its own entries",
			text: "run:\n  parallel: 1\nlog:\n  level: info\nrun:\n  pa",
			line: 5, col: 4,
			want: []string{"parallel"},
		},
		{
			name: "sibling before a nested child is still left out",
			text: "threshold:\n  total: 1\n  diff:\n    total: 1\n  to",
			line: 4, col: 4,
			not: []string{"total"},
		},
		{
			name: "top level after a dedent keeps top-level siblings",
			text: "output: x\nrun:\n  parallel: 1\nou",
			line: 3, col: 2,
			not: []string{"output"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labels(complete(t, tt.text, tt.line, tt.col))
			for _, w := range tt.want {
				if !slices.Contains(got, w) {
					t.Errorf("completions = %v, want %q among them", got, w)
				}
			}

			for _, n := range tt.not {
				if slices.Contains(got, n) {
					t.Errorf("completions = %v, want no %q", got, n)
				}
			}
		})
	}
}

func TestCompleteKeyBeforeAndAfterAColon(t *testing.T) {
	// "ou:x" is not a "key: value" entry (no space after the colon), so key
	// completion applies before the colon and nothing after it.
	tests := []struct {
		text string
		col  int
		want bool
	}{
		{"ou:x", 1, true},
		{"ou:x", 2, true},
		{"ou:x", 3, false},
		{"ou:x", 4, false},
		{":o", 2, false},
	}

	for _, tt := range tests {
		got := slices.Contains(labels(complete(t, tt.text, 0, tt.col)), "output")
		if got != tt.want {
			t.Errorf("Complete(%q, col %d) offers output = %v, want %v", tt.text, tt.col, got, tt.want)
		}
	}
}

func TestCompleteEnumValueEdges(t *testing.T) {
	enum := []string{"debug", "info", "warn", "warning", "error"}

	t.Run("right after the colon", func(t *testing.T) {
		got := labels(complete(t, "log:\n  level:", 1, len("  level:")))
		if !slices.Equal(got, enum) {
			t.Errorf("completions = %v, want %v", got, enum)
		}
	})

	t.Run("between spaces before the value", func(t *testing.T) {
		items := complete(t, "log:\n  level:  info", 1, 9)
		if !slices.Equal(labels(items), enum) {
			t.Fatalf("completions = %v, want %v", labels(items), enum)
		}

		for _, it := range items {
			if it.ReplaceStart != 9 || it.ReplaceEnd != 9 {
				t.Errorf("%s replaces %d..%d, want 9..9", it.Label, it.ReplaceStart, it.ReplaceEnd)
			}
		}
	})

	t.Run("prefix replaces the whole word", func(t *testing.T) {
		items := complete(t, "log:\n  level: warx", 1, len("  level: wa"))
		if got := labels(items); !slices.Equal(got, []string{"warn", "warning"}) {
			t.Fatalf("completions = %v, want [warn warning]", got)
		}

		for _, it := range items {
			if it.ReplaceStart != 9 || it.ReplaceEnd != 13 {
				t.Errorf("%s replaces %d..%d, want 9..13", it.Label, it.ReplaceStart, it.ReplaceEnd)
			}
		}
	})

	t.Run("an inline list value gets no completion", func(t *testing.T) {
		// Allowed list items are offered on "- item" lines only.
		if items := complete(t, "run:\n  mutagens: co", 1, len("  mutagens: co")); items != nil {
			t.Errorf("completions = %v, want none", labels(items))
		}
	})
}

func TestHoverRange(t *testing.T) {
	text := "output: x"
	tests := []struct {
		col  int
		want bool
	}{
		{0, true},
		{3, true},
		{6, true}, // right after the key, before the colon
		{7, false},
		{8, false},
	}

	for _, tt := range tests {
		var ok bool

		within(t, func() { _, ok = newEditor().Hover(text, 0, tt.col) })

		if ok != tt.want {
			t.Errorf("Hover(%q, col %d) = %v, want %v", text, tt.col, ok, tt.want)
		}
	}

	if _, ok := newEditor().Hover("bogus: 1", 0, 1); ok {
		t.Error("hover on an unknown key, want none")
	}

	if _, ok := newEditor().Hover("x", 0, 0); ok {
		t.Error("hover on a line that isn't an entry, want none")
	}

	if h, ok := newEditor().Hover("run:\n  parallel: 1\nlog:\n  level: info", 3, 3); !ok || !strings.Contains(h.Markdown, "log.level") {
		t.Errorf("hover after a dedent = %q, %v, want log.level", h.Markdown, ok)
	}
}

func TestDiagnoseAroundMalformedLines(t *testing.T) {
	const missing = "not a valid \"key: value\" line; gooze will ignore this whole file and use its defaults"

	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "indented first line is still checked",
			text: "  bogus: 1",
			want: []string{`unknown gooze config key "bogus"`},
		},
		{
			name: "children of a malformed line are skipped",
			text: "run:\n  bad\n    bogus: 1",
			want: []string{missing},
		},
		{
			name: "a sibling of a malformed line is checked",
			text: "run:\n  bad\n  bogus: 1",
			want: []string{missing, `unknown gooze config key "run.bogus"`},
		},
		{
			name: "a line after a malformed one at a lower indent is checked",
			text: "run:\n  bad\nbogus: 1",
			want: []string{missing, `unknown gooze config key "bogus"`},
		},
		{
			name: "quoted line without a colon is malformed",
			text: `"oops"`,
			want: []string{missing},
		},
		{
			name: "an unknown sibling after a known one",
			text: "run:\n  parallel: 1\n  bogus: 2",
			want: []string{`unknown gooze config key "run.bogus"`},
		},
		{
			name: "an integer key rejects a fraction",
			text: "run:\n  parallel: 1.5",
			want: []string{`invalid value for "run.parallel": expected an integer`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := messages(diagnose(t, tt.text))
			if !slices.Equal(got, tt.want) {
				t.Errorf("diagnostics = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiagnoseMalformedLineRange(t *testing.T) {
	diags := diagnose(t, "    x")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want 1", messages(diags))
	}

	if r := diags[0].Range; r.Start.Column != 4 || r.End.Column != 5 {
		t.Errorf("range = %+v, want columns 4..5", r)
	}
}

func TestInsertTextByKind(t *testing.T) {
	reg := config.NewRegistry()
	tests := map[string]string{
		"run":             "run:",
		"output":          "output: ",
		"no-cache":        "no-cache: ",
		"run.parallel":    "parallel: ",
		"run.mutagens":    "mutagens: ",
		"log.level":       "level: ",
		"threshold.total": "total: ",
	}

	for key, want := range tests {
		f, ok := reg.Lookup(key)
		if !ok {
			t.Fatalf("schema has no %q", key)
		}

		if got := insertText(f); got != want {
			t.Errorf("insertText(%s) = %q, want %q", key, got, want)
		}
	}
}
