package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The opencode plugins are run with node (which strips TypeScript types)
// against a stand-in goozels, to check what they append to a tool result.

// fakeGoozels is a goozels stand-in that records its arguments and prints
// $OUT on stdout and $ERR on stderr.
const fakeGoozels = `#!/bin/sh
echo "$*" > "$ARGS_FILE"
[ -n "$OUT" ] && printf '%s\n' "$OUT"
[ -n "$ERR" ] && printf '%s\n' "$ERR" >&2
exit 0
`

// v1Driver loads a v1 plugin with a stand-in for Bun's $ shell, whose awaited
// result has stdout and stderr buffers, and runs its hook after an edit.
const v1Driver = `
import { execFile } from "node:child_process"
const mod = await import(process.argv[2])
const $ = (parts, ...vals) => {
  const argv = parts.flatMap((p, i) => [...p.trim().split(/\s+/).filter(Boolean), ...(i < vals.length ? [vals[i]].flat() : [])])
  const run = () => new Promise((resolve) => execFile(argv[0], argv.slice(1), (_e, stdout, stderr) =>
    resolve({ stdout: Buffer.from(stdout), stderr: Buffer.from(stderr), text: () => stdout })))
  const p = { cwd: () => p, nothrow: () => p, quiet: () => p, text: () => run().then((r) => r.text()), then: (a, b) => run().then(a, b) }
  return p
}
const hooks = await mod.Goozels({ $, directory: process.cwd() })
const output = { output: "edited", metadata: {} }
await hooks["tool.execute.after"]({ tool: "edit", args: { filePath: "a.go" } }, output)
process.stdout.write(output.output)
`

// v2Driver loads a v2 plugin with a stand-in plugin context and runs its hook
// after an edit.
const v2Driver = `
const mod = await import(process.argv[2])
let hook
await mod.default.setup({ location: { directory: process.cwd() }, tool: { hook: (_name, fn) => { hook = fn } } })
const event = { status: "completed", tool: "edit", input: { path: "a.go" }, result: { content: "edited" } }
await hook(event)
const content = event.result.content
process.stdout.write(typeof content === "string" ? content : content.map((c) => c.text).join("\n---\n"))
`

func runPlugin(t *testing.T, version, out, errText string) (appended, args string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "goozels"), []byte(fakeGoozels), 0o700); err != nil { //nolint:gosec // test stand-in
		t.Fatal(err)
	}
	driver := map[string]string{"v1": v1Driver, "v2": v2Driver}[version]
	if err := os.WriteFile(filepath.Join(dir, "driver.mjs"), []byte(driver), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin, err := filepath.Abs(filepath.Join("..", "..", "editors", "opencode", version, "goozels.ts"))
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(node, filepath.Join(dir, "driver.mjs"), plugin)
	cmd.Dir = dir
	argsFile := filepath.Join(dir, "args")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ARGS_FILE="+argsFile, "OUT="+out, "ERR="+errText, "NODE_NO_WARNINGS=1")
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s plugin: %v\n%s", version, err, got)
	}
	recorded, _ := os.ReadFile(argsFile)
	return string(got), strings.TrimSpace(string(recorded))
}

func TestOpencodePluginsAppendResults(t *testing.T) {
	const note = "goozels: no gooze results for a.go yet; run: gooze run ."
	const diag = "a.go:1:1: warning: gooze: score 50.00% — 1 killed, 1 survived"
	for _, version := range []string{"v1", "v2"} {
		got, args := runPlugin(t, version, diag, note)
		if !strings.HasPrefix(args, "check --max 40 ") || !strings.HasSuffix(args, "a.go") {
			t.Errorf("%s: goozels args = %q, want check --max 40 FILE", version, args)
		}
		if !strings.Contains(got, hookHeader+"\n"+note+"\n"+diag) {
			t.Errorf("%s: appended %q, want the header, the stderr note, then the diagnostics", version, got)
		}

		if got, _ := runPlugin(t, version, "", note); !strings.Contains(got, hookHeader+"\n"+note) {
			t.Errorf("%s: a stderr note alone should be appended, got %q", version, got)
		}
		if got, _ := runPlugin(t, version, "", ""); strings.Contains(got, hookHeader) {
			t.Errorf("%s: nothing should be appended when goozels prints nothing, got %q", version, got)
		}
	}
}
