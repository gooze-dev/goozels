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

Download [`hooks.json`](hooks.json) into your project:

```bash
mkdir -p .codex && curl -fsSL -o .codex/hooks.json \
  https://raw.githubusercontent.com/gooze-dev/goozels/main/editors/codex/hooks.json
```

The hook runs `goozels hook codex` only when `goozels version` answers with a
line starting with `goozels`, so a machine without goozels (or with an older
build that lacks the command) just skips it instead of reporting a hook error.

Or, if you keep hooks in `.codex/config.toml`, add the same thing in TOML:

```toml
[[hooks.PostToolUse]]
matcher = "^apply_patch$"

[[hooks.PostToolUse.hooks]]
type = "command"
command = "sh -c 'v=$(goozels version 2>/dev/null); case \"$v\" in goozels*) exec goozels hook codex;; esac; exit 0'"
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
gooze results from the last run:
internal/domain/ignore.go:1:1: warning: gooze: score 67.65% — 69 killed, 32 survived, 1 not covered by any test
internal/domain/ignore.go:23:2: warning: gooze: 2 surviving mutants (comparison ×2) — tests still pass with `if len(r.names) == 0 {` → `if len(r.names) < 0 {`, `if len(r.names) <= 0 {`
internal/domain/ignore.go:40:2: warning: gooze: 3 surviving mutants (comparison ×2, logical) — tests still pass with `if dst.all || len(src.names) == 0 {` → `… < 0 {`, `… <= 0 {`, `… && len(src.names) == 0 {`
…
gooze: a mutant that cannot change behavior (an equivalent mutant) can be skipped with //gooze:ignore <mutagen> on its line
```

When the patch edits a test file, it also reports the source files that test
covers, since those are where the surviving mutants are. A Go file gooze has no
results for gets one line (`no gooze results for x.go yet; run: gooze run ./x`);
results from before the file or its tests changed are marked as out of date,
with the rerun command. The reply is capped at 40 diagnostics (about 4 KB), with
a last line naming the files to `goozels check` for the rest.

Within one Codex session, a file's full list is sent once and again only when it
changes; later edits of the same file repeat just its score line:

```text
gooze results from the last run:
internal/domain/ignore.go:1:1: warning: gooze: score 67.65% — 69 killed, 32 survived, 1 not covered by any test (survivors as listed earlier in this session)
```

> The hook sees `apply_patch` edits only. If Codex changes Go files with shell
> commands (`sed`, `gofmt -w`, …), it won't fire; the `AGENTS.md` snippet in the
> [main README](../../README.md#use-with-ai-coding-agents) tells the model to run
> `goozels check <file>` in that case.
