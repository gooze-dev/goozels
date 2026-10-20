package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/adapter/hookstate"
)

func hookPayload(t *testing.T, session, cwd, file string) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id": session, "cwd": cwd, "hook_event_name": "PostToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": file},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

// hookContext runs the hook with a fresh or given state and returns the
// additionalContext it replied with, or "" for no reply.
func hookContext(t *testing.T, agent string, in *bytes.Reader, state hookstate.Store) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := runHook([]string{agent}, in, &out, &errOut, state); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if out.Len() == 0 {
		return ""
	}
	var got hookOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not hook JSON: %v\n%s", err, out.String())
	}
	if got.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Errorf("hookEventName = %q", got.HookSpecificOutput.HookEventName)
	}
	return got.HookSpecificOutput.AdditionalContext
}

func noState() hookstate.Store { return hookstate.New("") }

func TestRunHookReportsDiagnostics(t *testing.T) {
	dir := workspace(t)
	ctx := hookContext(t, "claude", hookPayload(t, "s", dir, filepath.Join(dir, "gooze.yaml")), noState())
	if !strings.HasPrefix(ctx, "gooze results from the last run:\n") || !strings.Contains(ctx, "gooze.yaml:1:1: error") {
		t.Errorf("additionalContext = %q", ctx)
	}
}

func TestRunHookQuietCases(t *testing.T) {
	dir := workspace(t)
	cases := map[string]*bytes.Reader{
		"non-gooze file": hookPayload(t, "s", dir, filepath.Join(dir, "README.md")),
		"missing file":   hookPayload(t, "s", dir, filepath.Join(dir, "gone.go")),
		"not json":       bytes.NewReader([]byte("nope")),
	}
	for name, in := range cases {
		if ctx := hookContext(t, "claude", in, noState()); ctx != "" {
			t.Errorf("%s: additionalContext = %q, want no output", name, ctx)
		}
	}

	var out, errOut bytes.Buffer
	if code := runHook(nil, bytes.NewReader(nil), &out, &errOut, noState()); code != 2 {
		t.Errorf("missing agent name: exit = %d, want 2", code)
	}
}

func TestRunHookNoResults(t *testing.T) {
	dir := workspace(t)
	state := hookstate.New(t.TempDir())

	ctx := hookContext(t, "claude", hookPayload(t, "s", dir, filepath.Join(dir, "clean.go")), state)
	if ctx != "gooze results from the last run:\nno gooze results for clean.go yet; run: gooze run ." {
		t.Errorf("additionalContext = %q", ctx)
	}
	if ctx := hookContext(t, "claude", hookPayload(t, "s", dir, filepath.Join(dir, "clean.go")), state); ctx != "" {
		t.Errorf("the note should be sent once per session, got %q", ctx)
	}
}

func TestRunHookCapsOutput(t *testing.T) {
	dir := t.TempDir()
	var src strings.Builder
	src.WriteString("package x\n")
	for range maxHookLines + 5 {
		src.WriteString("var _ = 1 //gooze:ignore bogus\n")
	}
	file := filepath.Join(dir, "many.go")
	if err := os.WriteFile(file, []byte(src.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := hookContext(t, "claude", hookPayload(t, "s", dir, file), noState())
	if len(ctx) > maxHookBytes+500 {
		t.Errorf("additionalContext is %d bytes; want about %d at most", len(ctx), maxHookBytes)
	}
	lines := strings.Split(ctx, "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "… ") || !strings.HasSuffix(last, " more; run goozels check many.go") {
		t.Errorf("last line = %q, want the cap line naming the file", last)
	}
}

func TestRunHookSessionState(t *testing.T) {
	dir := projectWithReports(t)
	src := filepath.Join(dir, "calc", "calc.go")
	state := hookstate.New(t.TempDir())
	edit := func(session string) string {
		return hookContext(t, "claude", hookPayload(t, session, dir, src), state)
	}

	first := edit("s1")
	if !strings.Contains(first, "surviving") || !strings.Contains(first, "//gooze:ignore <mutagen>") {
		t.Fatalf("the first report should list survivors and the ignore hint:\n%s", first)
	}
	if len(first) > maxHookBytes+500 {
		t.Errorf("first report is %d bytes", len(first))
	}

	again := edit("s1")
	want := "gooze results from the last run:\ncalc/calc.go:1:1: warning: gooze: score 43.75% — 7 killed, 3 survived, 6 not covered by any test" + briefSuffix
	if again != want {
		t.Errorf("unchanged results should be summarized:\n%s\nwant:\n%s", again, want)
	}
	if other := edit("s2"); other != first {
		t.Errorf("another session should get the full list again:\n%s", other)
	}

	// Editing the test makes the results out of date: the set changed, so
	// the full list is sent again with the stale notice.
	test := filepath.Join(dir, "calc", "calc_test.go")
	if err := os.WriteFile(test, []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if stale := edit("s1"); !strings.Contains(stale, "tests changed since the last run") || !strings.Contains(stale, "surviving") {
		t.Errorf("a changed result set should be sent in full:\n%s", stale)
	}
}

func TestRunHookTestFileEdit(t *testing.T) {
	dir := projectWithReports(t)
	ctx := hookContext(t, "claude", hookPayload(t, "", dir, filepath.Join(dir, "calc", "calc_test.go")), noState())
	if !strings.Contains(ctx, "calc/calc.go:1:1") {
		t.Errorf("additionalContext = %q, want calc.go's summary", ctx)
	}
}

// codexPayload builds a Codex PostToolUse payload for an apply_patch call; the
// shape follows codex-rs hooks/schema post-tool-use.command.input.
func codexPayload(t *testing.T, cwd string, command any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id": "c", "cwd": cwd, "hook_event_name": "PostToolUse", "tool_name": "apply_patch", "model": "m",
		"tool_input": map[string]any{"command": command}, "tool_response": "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

const codexPatch = `*** Begin Patch
*** Update File: x.go
@@
-var y = 2
+var y = 1 //gooze:ignore bogus
*** Add File: gooze.yaml
+run:r
*** Delete File: old.go
*** Update File: a.go
*** Move to: clean.go
*** End Patch
`

func TestPatchedFiles(t *testing.T) {
	got := strings.Join(patchedFiles(codexPatch), ",")
	if got != "x.go,gooze.yaml,a.go,clean.go" {
		t.Errorf("patchedFiles = %s; want added, updated and moved-to files, not deleted ones", got)
	}
}

func TestRunHookCodex(t *testing.T) {
	dir := workspace(t)
	for name, command := range map[string]any{"string": codexPatch, "argv": []string{"apply_patch", codexPatch}} {
		ctx := hookContext(t, "codex", codexPayload(t, dir, command), noState())
		if !strings.Contains(ctx, "x.go:") || !strings.Contains(ctx, "gooze.yaml:1:1: error") {
			t.Errorf("%s: additionalContext = %q, want diagnostics for x.go and gooze.yaml", name, ctx)
		}
	}

	if ctx := hookContext(t, "codex", codexPayload(t, dir, "ls -la"), noState()); ctx != "" {
		t.Errorf("a shell command without a patch should print nothing, got %q", ctx)
	}
}

func TestRunHookCodexPatchOfSourceAndTest(t *testing.T) {
	dir := projectWithReports(t)
	patch := "*** Begin Patch\n*** Update File: calc/calc.go\n*** Update File: calc/calc_test.go\n*** End Patch\n"
	ctx := hookContext(t, "codex", codexPayload(t, dir, patch), noState())
	if n := strings.Count(ctx, "calc/calc.go:1:1:"); n != 1 {
		t.Errorf("calc.go is reported %d times, want once:\n%s", n, ctx)
	}
}

func TestRunHookRejectsOtherAgents(t *testing.T) {
	for _, agent := range []string{"aaa", "zzz", "cl"} {
		var out, errOut bytes.Buffer
		if code := runHook([]string{agent}, bytes.NewReader([]byte("{}")), &out, &errOut, noState()); code != 2 {
			t.Errorf("agent %q: exit = %d, want 2", agent, code)
		}
	}
}

func TestRunHookKeepsCodeReadable(t *testing.T) {
	dir := projectWithReports(t)
	var out, errOut bytes.Buffer
	runHook([]string{"claude"}, hookPayload(t, "", dir, filepath.Join(dir, "calc", "calc.go")), &out, &errOut, noState())
	if !strings.Contains(out.String(), "return n > 0") {
		t.Errorf("code in the reply should not be HTML-escaped:\n%s", out.String())
	}
}

func TestRunHookWriteFailure(t *testing.T) {
	dir := workspace(t)
	var errOut bytes.Buffer
	code := runHook([]string{"claude"}, hookPayload(t, "", dir, filepath.Join(dir, "gooze.yaml")), failingWriter{}, &errOut, noState())
	if code != 0 || !strings.HasPrefix(errOut.String(), "goozels: ") {
		t.Errorf("exit = %d, stderr = %q; want 0 and the error on stderr", code, errOut.String())
	}
}

func TestMarkSeen(t *testing.T) {
	e := sampleEntry("a.go")
	d := digest(&e)
	for name, c := range map[string]struct {
		seen  string
		brief bool
	}{
		"new file":          {"", false},
		"same results":      {d, true},
		"changed, sorts up": {"ffffffff", false},
		"changed, sorts lo": {"00000000", false},
	} {
		entries := []entry{sampleEntry("a.go")}
		seen := map[string]string{}
		if c.seen != "" {
			seen[entries[0].abs] = c.seen
		}
		markSeen(entries, seen)
		if entries[0].brief != c.brief || seen[entries[0].abs] != d {
			t.Errorf("%s: brief = %v, seen = %q; want brief %v and the new digest", name, entries[0].brief, seen[entries[0].abs], c.brief)
		}
	}
}

func TestDigest(t *testing.T) {
	if d := digest(&entry{}); d != "" {
		t.Errorf("an entry without results has no digest, got %q", d)
	}
	short := entry{note: "x"}
	if d := digest(&short); len(d) != 32 {
		t.Errorf("digest of a one-byte note = %q, want 32 hex digits", d)
	}
	other := entry{note: "y"}
	if digest(&short) == digest(&other) {
		t.Error("different notes should have different digests")
	}
}

func TestPatchedFilesOnce(t *testing.T) {
	got := patchedFiles("*** Update File: a.go\n*** Update File: a.go\n*** Move to: a.go\n")
	if strings.Join(got, ",") != "a.go" {
		t.Errorf("patchedFiles = %v, want a.go once", got)
	}
}

func TestRunHookRejectsExtraArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runHook([]string{"claude", "x"}, bytes.NewReader([]byte("{}")), &out, &errOut, noState()); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}
