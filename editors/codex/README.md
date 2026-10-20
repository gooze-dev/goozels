# Codex setup

Codex doesn't support language servers, so goozels plugs in as a **post-edit
hook**: after every `apply_patch`, Codex runs `goozels hook codex`, which reads
the patched file names from the hook payload and adds their gooze diagnostics
(mutation score, surviving mutants, `//gooze:ignore` and `gooze.yaml` errors) to
the model's context. Codex's own tooling is untouched.

Requires Codex **0.125 or later** (hooks are on by default from that release).

## 1. Install goozels

```bash
go install gooze.dev/pkg/goozels/cmd/goozels@latest
```

`goozels` (and `gooze`, to run mutation tests) must be on the `PATH` of the shell
you start `codex` from.

## 2. Add the hook

Copy [`hooks.json`](hooks.json) into your project:

```bash
mkdir -p .codex && cp /path/to/goozels/editors/codex/hooks.json .codex/hooks.json
```

Or, if you keep hooks in `.codex/config.toml`, add the same thing in TOML:

```toml
[[hooks.PostToolUse]]
matcher = "^apply_patch$"

[[hooks.PostToolUse.hooks]]
type = "command"
command = "goozels hook codex"
timeout = 60
statusMessage = "goozels check"
```

## 3. Trust it

Codex runs a project hook only after you review it. Start `codex`, open `/hooks`
and trust the goozels hook. Codex asks again whenever the hook changes, and loads
project hooks only when the project's `.codex/` folder is trusted.

## What the model sees

The hook replies with `additionalContext`, which Codex records next to the patch
result without blocking it:

```text
goozels (gooze mutation testing) diagnostics after this edit:
internal/domain/ignore.go:1:1: warning: gooze: 41/129 mutants survived, 2 not covered by any test — score 66%
internal/domain/ignore.go:208:2: warning: gooze: surviving comparison mutant — tests still pass with: if line == 0 || line > len(lineStarts) {
```

It prints nothing for files gooze has nothing to say about, and caps its output
at 40 lines.

> The hook sees `apply_patch` edits only. If Codex changes Go files with shell
> commands (`sed`, `gofmt -w`, …), it won't fire; the `AGENTS.md` snippet in the
> [main README](../../README.md#use-with-ai-coding-agents) tells the model to run
> `goozels check <file>` in that case.
