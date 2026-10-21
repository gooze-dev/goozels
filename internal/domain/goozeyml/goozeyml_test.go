package goozeyml

import (
	"slices"
	"strings"
	"testing"

	"gooze.dev/pkg/gooze/pkg/config"
	"gooze.dev/pkg/gooze/pkg/mutagen"

	"gooze.dev/pkg/goozels/internal/model"
)

func newEditor() Editor { return New(config.NewRegistry()) }

// allMutagens lists every mutagen gooze has, as run.mutagens accepts them.
var allMutagens = strings.Join(mutagen.Names(), ", ")

func TestCompleteTopLevel(t *testing.T) {
	items := newEditor().Complete("ou", 0, 2)
	found := false
	for _, it := range items {
		if it.Label == "output" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'output' in top-level completions, got %+v", labels(items))
	}
}

func TestCompleteNested(t *testing.T) {
	text := "run:\n  "
	items := newEditor().Complete(text, 1, 2)
	want := map[string]bool{"parallel": false, "mutation_timeout": false, "coverage_profile": false}
	for _, it := range items {
		if _, ok := want[it.Label]; ok {
			want[it.Label] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("expected nested run child %q in completions, got %v", k, labels(items))
		}
	}
}

func TestCompleteUnderScalarKey(t *testing.T) {
	if items := newEditor().Complete("output: x\n  ", 1, 2); items != nil {
		t.Errorf("expected no completions under a scalar key, got %v", labels(items))
	}
}

func TestCompleteEnumValue(t *testing.T) {
	text := "log:\n  level: "
	line := "  level: "
	items := newEditor().Complete(text, 1, len(line))
	want := []string{"debug", "info", "warn", "warning", "error"}
	if got := labels(items); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("enum values = %v, want %v", got, want)
	}
}

func TestHover(t *testing.T) {
	text := "run:\n  parallel: 4\n"
	h, ok := newEditor().Hover(text, 1, 4)
	if !ok {
		t.Fatal("expected hover on run.parallel")
	}
	if !strings.Contains(h.Markdown, "run.parallel") || !strings.Contains(h.Markdown, "Default: `1`") {
		t.Errorf("hover = %q, want key path and default", h.Markdown)
	}
}

func TestHoverDefaults(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "output: x\n", want: "Default: `.gooze-reports`"},
		{text: "no-cache: true\n", want: "Default: `false`"},
		{text: "paths:\n  exclude: []\n", want: "Default: `[]`"},
	}
	for _, tt := range tests {
		lines := strings.Split(tt.text, "\n")
		line := len(lines) - 2
		h, ok := newEditor().Hover(tt.text, line, strings.Index(lines[line], ":")-1)
		if !ok || !strings.Contains(h.Markdown, tt.want) {
			t.Errorf("hover %q = %q, want %q", tt.text, h.Markdown, tt.want)
		}
	}

	h, ok := newEditor().Hover("run:\n  coverage_profile: c.out\n", 1, 4)
	if !ok || strings.Contains(h.Markdown, "Default") {
		t.Errorf("empty default should be omitted, hover = %q", h.Markdown)
	}
}

func TestDiagnoseBadValueAndUnknownKey(t *testing.T) {
	text := "log:\n  level: loud\nbogus: 1\n"
	diags := newEditor().Diagnose(text)
	var badValue, unknownKey bool
	for _, d := range diags {
		if strings.Contains(d.Message, "log.level") {
			badValue = true
		}
		if strings.Contains(d.Message, "unknown gooze config key") {
			unknownKey = true
		}
	}
	if !badValue {
		t.Error("expected invalid-value diagnostic for log.level")
	}
	if !unknownKey {
		t.Error("expected unknown-key diagnostic for bogus")
	}
}

func TestDiagnoseValidConfig(t *testing.T) {
	text := "output: .gooze-reports\nrun:\n  parallel: 4\n  coverage_profile: cover.out\nlog:\n  level: warning\n"
	if diags := newEditor().Diagnose(text); len(diags) != 0 {
		t.Errorf("valid config produced diagnostics: %+v", diags)
	}
}

func TestDiagnoseUsesRegistry(t *testing.T) {
	registry := config.NewMockRegistry(t)
	registry.EXPECT().Lookup("mode").Return(config.Field{Name: "mode", Kind: config.KindEnum, Enum: []string{"fast"}}, true)

	diags := New(registry).Diagnose("mode: slow\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "expected one of: fast") {
		t.Errorf("diagnostics = %+v, want one invalid-value diagnostic", diags)
	}
}

func TestValidateScalar(t *testing.T) {
	level, _ := config.Lookup(config.KeyLogLevel)
	parallel, _ := config.Lookup(config.KeyRunParallel)
	noCache, _ := config.Lookup(config.KeyNoCache)
	output, _ := config.Lookup(config.KeyOutput)

	tests := []struct {
		field config.Field
		raw   string
		valid bool
	}{
		{field: level, raw: "info", valid: true},
		{field: level, raw: "WARNING", valid: true},
		{field: level, raw: "-4", valid: true},
		{field: level, raw: "loud"},
		{field: config.Field{Kind: config.KindEnum, Enum: []string{"a"}}, raw: "1"},
		{field: parallel, raw: "4", valid: true},
		{field: parallel, raw: "lots"},
		{field: noCache, raw: "TRUE", valid: true},
		{field: noCache, raw: "maybe"},
		{field: output, raw: "anything", valid: true},
		{field: parallel, raw: " ", valid: true},
	}
	for _, tt := range tests {
		if msg := validateScalar(tt.field, tt.raw); (msg == "") != tt.valid {
			t.Errorf("validateScalar(%s, %q) = %q, want valid=%v", tt.field.Name, tt.raw, msg, tt.valid)
		}
	}
}

func labels(items []model.CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

// goozeRepoConfig is gooze's own gooze.yaml with the typo that made gooze ignore
// the whole file: "run:r" is a plain scalar in YAML, not the key "run".
const goozeRepoConfig = `log:
    level: 0
output: .gooze-reports
paths:
    exclude: []
run:r
    mutation_timeout: 120
    parallel: 1
version: 1
`

func TestDiagnoseMissingSpaceAfterColon(t *testing.T) {
	diags := newEditor().Diagnose(goozeRepoConfig)
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want exactly 1 (children of the broken line are not re-reported): %+v", len(diags), diags)
	}
	d := diags[0]
	if d.Range.Start.Line != 5 || d.Severity != model.SeverityError || !strings.Contains(d.Message, "missing a space after") {
		t.Errorf("diagnostic = %+v", d)
	}
	if d.Range.Start.Column != 0 || d.Range.End.Column != len("run:r") {
		t.Errorf("range = %+v, want the whole \"run:r\" token", d.Range)
	}
}

func TestDiagnoseSectionWithScalarValue(t *testing.T) {
	diags := newEditor().Diagnose("run: fast\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "is a section") {
		t.Fatalf("diagnostics = %+v", diags)
	}
	if got := newEditor().Diagnose("run: {parallel: 4}\n"); len(got) != 0 {
		t.Errorf("flow mapping should be accepted, got %+v", got)
	}
}

func TestDiagnoseLineWithoutColon(t *testing.T) {
	diags := newEditor().Diagnose("output: .gooze-reports\njunk\n")
	if len(diags) != 1 || diags[0].Range.Start.Line != 1 || !strings.Contains(diags[0].Message, "not a valid") {
		t.Fatalf("diagnostics = %+v", diags)
	}
}

func TestParseEntryColonRules(t *testing.T) {
	cases := []struct {
		line, key, value string
		ok               bool
	}{
		{"output: a:b", "output", "a:b", true}, // later colons belong to the value
		{"run:", "run", "", true},              // key with nested lines below
		{"run:r", "", "", false},               // plain scalar, not a key
		{"url: http://x", "url", "http://x", true},
	}
	for _, c := range cases {
		e, ok := parseEntry(0, c.line)
		if ok != c.ok || e.key != c.key || e.value != c.value {
			t.Errorf("parseEntry(%q) = %q, %q, %v; want %q, %q, %v", c.line, e.key, e.value, ok, c.key, c.value, c.ok)
		}
	}
}

func TestDiagnoseIntegerBounds(t *testing.T) {
	cases := []struct {
		yaml, want string
		line       int
	}{
		{"version: 2\n", `invalid value for "version": 2 is out of range: must be 1`, 0},
		{"run:\n  parallel: 0\n", `invalid value for "run.parallel": 0 is out of range: must be at least 1`, 1},
		{"run:\n  mutation_timeout: 0\n", `invalid value for "run.mutation_timeout": 0 is out of range: must be at least 1`, 1},
	}
	for _, c := range cases {
		diags := newEditor().Diagnose(c.yaml)
		if len(diags) != 1 || diags[0].Message != c.want || diags[0].Range.Start.Line != c.line || diags[0].Severity != model.SeverityError {
			t.Errorf("Diagnose(%q) = %+v, want one error %q on line %d", c.yaml, diags, c.want, c.line)
		}
	}

	if diags := newEditor().Diagnose("version: 1\nrun:\n  parallel: 8\n  mutation_timeout: 120\n"); len(diags) != 0 {
		t.Errorf("in-range values should be accepted, got %+v", diags)
	}
}

func TestDiagnoseThresholds(t *testing.T) {
	cases := []struct {
		yaml, want string
		line       int
	}{
		{"threshold:\n  total: 100.5\n", `invalid value for "threshold.total": 100.5 is out of range: must be between 0 and 100`, 1},
		{"threshold:\n  diff:\n    file: -1\n", `invalid value for "threshold.diff.file": -1 is out of range: must be between 0 and 100`, 2},
		{"threshold:\n  average: high\n", `invalid value for "threshold.average": expected a number`, 1},
	}
	for _, c := range cases {
		diags := newEditor().Diagnose(c.yaml)
		if len(diags) != 1 || diags[0].Message != c.want || diags[0].Range.Start.Line != c.line || diags[0].Severity != model.SeverityError {
			t.Errorf("Diagnose(%q) = %+v, want one error %q on line %d", c.yaml, diags, c.want, c.line)
		}
	}

	valid := "threshold:\n  total: 80\n  average: 87.5\n  file: 0\n  diff:\n    total: 100\n"
	if diags := newEditor().Diagnose(valid); len(diags) != 0 {
		t.Errorf("percents should be accepted, got %+v", diags)
	}

	items := newEditor().Complete("threshold:\n  ", 1, 2)
	var names []string
	for _, item := range items {
		names = append(names, item.Label+" "+item.Detail)
	}
	if !slices.Contains(names, "total float") || !slices.Contains(names, "diff object") {
		t.Errorf("threshold completions = %v, want total (float) and diff (object)", names)
	}
}

func TestNewerGoozeKeysAreKnown(t *testing.T) {
	text := "no-update-check: true\nrun:\n  diff: origin/main\n  mutagens: [comparison]\n"
	if diags := newEditor().Diagnose(text); len(diags) != 0 {
		t.Errorf("keys of the gooze in use must be valid, got %+v", diags)
	}

	items := newEditor().Complete("run:\n  ", 1, 2)
	for _, want := range []string{"diff", "mutagens"} {
		if !strings.Contains(strings.Join(labels(items), " "), want) {
			t.Errorf("expected run.%s in completions, got %v", want, labels(items))
		}
	}
}

func TestDiagnoseListItems(t *testing.T) {
	const valid = "comparison, logical"

	cases := map[string]struct {
		text string
		want string // "" for valid
	}{
		"flow list ok":             {"run:\n  mutagens: [" + valid + "]\n", ""},
		"flow list unknown":        {"run:\n  mutagens: [comparison, bogus]\n", `invalid value for "run.mutagens": unknown item "bogus"; expected one of: `+allMutagens},
		"flow list quoted":         {"run:\n  mutagens: [\"comparison\", 'unary']\n", ""},
		"single value":             {"run:\n  mutagens: comparison\n", ""},
		"single value bad":         {"run:\n  mutagens: bogus\n", `invalid value for "run.mutagens": unknown item "bogus"; expected one of: `+allMutagens},
		"off by default":           {"run:\n  mutagens: statement\n", ""},
		"empty flow list":          {"run:\n  mutagens: []\n", ""},
		"block list ok":            {"run:\n  mutagens:\n    - comparison\n    - logical # why\n", ""},
		"block list unknown":       {"run:\n  mutagens:\n    - comparison\n    - bogus\n", `invalid value for "run.mutagens": unknown item "bogus"; expected one of: `+allMutagens},
		"block list at key indent": {"run:\n  mutagens:\n  - bogus\n", `invalid value for "run.mutagens": unknown item "bogus"; expected one of: `+allMutagens},
		"other lists unchecked":    {"paths:\n  exclude:\n    - anything\n  exclude2: x\n", `unknown gooze config key "paths.exclude2"`},
	}

	for name, tc := range cases {
		diags := newEditor().Diagnose(tc.text)

		switch {
		case tc.want == "" && len(diags) != 0:
			t.Errorf("%s: want no diagnostics, got %+v", name, diags)
		case tc.want != "" && (len(diags) != 1 || !strings.HasPrefix(diags[0].Message, tc.want)):
			t.Errorf("%s: want %q, got %+v", name, tc.want, diags)
		}
	}
}

func TestDiagnoseBlockListItemRange(t *testing.T) {
	diags := newEditor().Diagnose("run:\n  mutagens:\n    - bogus   \n")
	want := model.Range{Start: model.Position{Line: 2, Column: 6}, End: model.Position{Line: 2, Column: 11}}

	if len(diags) != 1 || diags[0].Range != want || diags[0].Severity != model.SeverityError {
		t.Errorf("diagnostics = %+v, want one error at %+v", diags, want)
	}
}

func TestCompleteListItem(t *testing.T) {
	text := "run:\n  mutagens:\n    - co"

	items := newEditor().Complete(text, 2, 8)
	if got := strings.Join(labels(items), " "); got != "comparison" {
		t.Errorf("completions = %q, want comparison", got)
	}

	if len(items) == 1 && (items[0].ReplaceStart != 6 || items[0].ReplaceEnd != 8) {
		t.Errorf("replace range = %d..%d, want 6..8", items[0].ReplaceStart, items[0].ReplaceEnd)
	}

	all := newEditor().Complete("run:\n  mutagens:\n    - ", 2, 6)
	if len(all) != len(mutagen.All()) {
		t.Errorf("an empty item offers every mutagen, got %v", labels(all))
	}

	if got := newEditor().Complete("paths:\n  exclude:\n    - ", 2, 6); len(got) != 0 {
		t.Errorf("a list without values offers none, got %v", labels(got))
	}
}

func TestKindLabelOfAListWithValues(t *testing.T) {
	items := newEditor().Complete("run:\n  mutag", 1, 7)
	if len(items) != 1 || items[0].Detail != "list: "+strings.Join(mutagen.Names(), "|") {
		t.Errorf("completion detail = %+v", items)
	}
}

func TestListItemEdges(t *testing.T) {
	ed := newEditor()

	quiet := map[string]string{
		"item without a list key above":       "- bogus\n",
		"dash without a space is not an item": "run:\n  mutagens:\n    -bogus\n",
		"owner with an inline value":          "run:\n  mutagens: [comparison]\n    - bogus\n",
		"owner less indented than item?":      "run:\n  mutagens:\n- bogus\n",
		"items under an enum key":             "log:\n  level:\n    - bogus\n",
	}
	for name, text := range quiet {
		for _, d := range ed.Diagnose(text) {
			if strings.Contains(d.Message, "unknown item") {
				t.Errorf("%s: unexpected %q", name, d.Message)
			}
		}
	}

	if diags := ed.Diagnose("run:\n  mutagens:\n    -\tbogus\n"); len(diags) != 1 || !strings.Contains(diags[0].Message, `unknown item "bogus"`) {
		t.Errorf("a tab after the dash still starts an item, got %+v", diags)
	}
}

func TestCompleteListItemEdges(t *testing.T) {
	ed := newEditor()
	has := func(items []model.CompletionItem) bool { return slices.Contains(labels(items), "comparison") }

	if !has(ed.Complete("run:\n  mutagens:\n  - co", 2, 6)) {
		t.Error("an item at its key's indent completes")
	}

	for name, c := range map[string]struct {
		text      string
		line, col int
	}{
		"not an item line":        {"run:\n  mutagens:\n    co", 2, 6},
		"before the dash":         {"run:\n  mutagens:\n    - co", 2, 4},
		"no list key above":       {"- co", 0, 4},
		"owner with inline value": {"run:\n  mutagens: [x]\n    - co", 2, 8},
		"items under an enum key": {"log:\n  level:\n    - co", 2, 8},
	} {
		if items := ed.Complete(c.text, c.line, c.col); has(items) {
			t.Errorf("%s: offered %v", name, labels(items))
		}
	}

	if got := ed.Complete("log:\n  level:\n    - de", 2, 8); len(got) != 0 {
		t.Errorf("an enum key's values are not list items: %v", labels(got))
	}
}

func TestKindLabel(t *testing.T) {
	cases := map[string]struct {
		f    config.Field
		want string
	}{
		"enum":                {config.Field{Kind: config.KindEnum, Enum: []string{"a", "b"}}, "enum: a|b"},
		"list with values":    {config.Field{Kind: config.KindStringList, Enum: []string{"a", "b"}}, "list: a|b"},
		"list with one value": {config.Field{Kind: config.KindStringList, Enum: []string{"a"}}, "list: a"},
		"list without values": {config.Field{Kind: config.KindStringList}, "string list"},
		"string with values":  {config.Field{Kind: config.KindString, Enum: []string{"a"}}, "string"},
		"int":                 {config.Field{Kind: config.KindInt}, "int"},
		"float":               {config.Field{Kind: config.KindFloat}, "float"},
	}

	for name, c := range cases {
		if got := kindLabel(c.f); got != c.want {
			t.Errorf("%s: kindLabel = %q, want %q", name, got, c.want)
		}
	}
}

func TestDiagnoseIgnoresBlankAndCommentLines(t *testing.T) {
	text := "run:\n\n  # how many\n  parallel: 2\n\n  mutagens:\n    # the fast ones\n    - comparison\n\n"
	if diags := newEditor().Diagnose(text); len(diags) != 0 {
		t.Errorf("blank and comment lines are fine, got %+v", diags)
	}
}

func TestCompleteBareDash(t *testing.T) {
	if got := newEditor().Complete("run:\n  mutagens:\n    -", 2, 5); len(got) != len(mutagen.All()) {
		t.Errorf("a bare dash starts an item, got %v", labels(got))
	}
}
