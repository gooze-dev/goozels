package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/internal/model"
)

// workspace writes a project with a gooze config typo and a bad directive.
func workspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"gooze.yaml": "run:r\n",
		"x.go":       "package x\n\nvar y = 1 //gooze:ignore bogus\n",
		"clean.go":   "package x\n",
		"README.md":  "run:r\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunCheck(t *testing.T) {
	dir := workspace(t)
	var out, errOut bytes.Buffer
	files := []string{"-workdir", dir}
	for _, f := range []string{"gooze.yaml", "x.go", "clean.go", "README.md"} {
		files = append(files, filepath.Join(dir, f))
	}

	if code := runCheck(files, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "gooze.yaml:1:1: error: missing a space after") {
		t.Errorf("missing config error in:\n%s", got)
	}
	if !strings.Contains(got, "x.go:3:") || !strings.Contains(got, "bogus") {
		t.Errorf("missing directive error in:\n%s", got)
	}
	if strings.Contains(got, "clean.go") || strings.Contains(got, "README.md") {
		t.Errorf("clean and non-gooze files should print nothing:\n%s", got)
	}
}

func TestRunCheckErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runCheck(nil, &out, &errOut); code != 2 {
		t.Errorf("no files: exit = %d, want 2", code)
	}
	if code := runCheck([]string{filepath.Join(t.TempDir(), "missing.go")}, &out, &errOut); code != 1 {
		t.Errorf("missing file: exit = %d, want 1", code)
	}
}

func TestFormatDiagnostic(t *testing.T) {
	d := model.Diagnostic{
		Range:    model.Range{Start: model.Position{Line: 4, Column: 2}},
		Severity: model.SeverityWarning,
		Message:  "m",
	}
	if got := formatDiagnostic("a/b.go", d); got != "a/b.go:5:3: warning: m" {
		t.Errorf("formatDiagnostic = %q", got)
	}
}

func hookPayload(t *testing.T, cwd, file string) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"cwd": cwd, "hook_event_name": "PostToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": file},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

func TestRunHookReportsDiagnostics(t *testing.T) {
	dir := workspace(t)
	var out, errOut bytes.Buffer
	if code := runHook([]string{"claude"}, hookPayload(t, dir, filepath.Join(dir, "gooze.yaml")), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}

	var got hookOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not hook JSON: %v\n%s", err, out.String())
	}
	if got.HookSpecificOutput.HookEventName != "PostToolUse" ||
		!strings.Contains(got.HookSpecificOutput.AdditionalContext, "gooze.yaml:1:1: error") {
		t.Errorf("hook output = %+v", got)
	}
}

func TestRunHookQuietCases(t *testing.T) {
	dir := workspace(t)
	cases := map[string]*bytes.Reader{
		"non-gooze file": hookPayload(t, dir, filepath.Join(dir, "README.md")),
		"clean go file":  hookPayload(t, dir, filepath.Join(dir, "clean.go")),
		"missing file":   hookPayload(t, dir, filepath.Join(dir, "gone.go")),
		"not json":       bytes.NewReader([]byte("nope")),
	}
	for name, in := range cases {
		var out, errOut bytes.Buffer
		if code := runHook([]string{"claude"}, in, &out, &errOut); code != 0 || out.Len() != 0 {
			t.Errorf("%s: exit = %d, output = %q; want 0 and no output", name, code, out.String())
		}
	}

	var out, errOut bytes.Buffer
	if code := runHook(nil, bytes.NewReader(nil), &out, &errOut); code != 2 {
		t.Errorf("missing agent name: exit = %d, want 2", code)
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

	var out, errOut bytes.Buffer
	runHook([]string{"claude"}, hookPayload(t, dir, file), &out, &errOut)

	var got hookOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got.HookSpecificOutput.AdditionalContext, "\n")
	if len(lines) != maxHookLines+2 || !strings.Contains(lines[len(lines)-1], "5 more") {
		t.Errorf("got %d lines ending %q; want header + %d + summary", len(lines), lines[len(lines)-1], maxHookLines)
	}
}

// codexPayload builds a Codex PostToolUse payload for an apply_patch call; the
// shape follows codex-rs hooks/schema post-tool-use.command.input.
func codexPayload(t *testing.T, cwd string, command any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"cwd": cwd, "hook_event_name": "PostToolUse", "tool_name": "apply_patch", "model": "m",
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
		var out, errOut bytes.Buffer
		if code := runHook([]string{"codex"}, codexPayload(t, dir, command), &out, &errOut); code != 0 {
			t.Fatalf("%s: exit = %d, stderr = %q", name, code, errOut.String())
		}

		var got hookOutput
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("%s: output is not hook JSON: %v\n%s", name, err, out.String())
		}
		ctx := got.HookSpecificOutput.AdditionalContext
		if !strings.Contains(ctx, "x.go:") || !strings.Contains(ctx, "gooze.yaml:1:1: error") {
			t.Errorf("%s: additionalContext = %q, want diagnostics for x.go and gooze.yaml", name, ctx)
		}
	}

	var out, errOut bytes.Buffer
	if runHook([]string{"codex"}, codexPayload(t, dir, "ls -la"), &out, &errOut); out.Len() != 0 {
		t.Errorf("a shell command without a patch should print nothing, got %q", out.String())
	}
}
