# goozels

Editor support for [**gooze**](https://github.com/gooze-dev/gooze), the Go
mutation-testing tool. goozels adds autocompletion, hovers, live error-checking,
and one-click mutation runs to your editor — for both `//gooze:ignore` comments in
your Go code and your `gooze.yaml` config file.

It works by sitting in front of `gopls` (the standard Go language server), so you
keep **all** your normal Go features and simply get gooze ones on top.

---

## What you get

**In your `.go` files**

- Autocomplete the mutagen names in a `//gooze:ignore` comment:

  ```go
  x := a + b //gooze:ignore ari⌶      →  arithmetic, …
  ```
- Hover a mutagen name to see what it does.
- A warning squiggle if you mistype one:

  ```go
  y := a - b //gooze:ignore bogus     ⚠ unknown gooze mutagen "bogus"
  ```
- **gooze: run package**, **gooze: estimate** and **gooze: report** code lenses
  at the top of every Go file — run mutation testing on the file's package,
  preview what it would mutate, or show the report (`gooze report view`)
  without leaving the editor. When a run finishes, the notification gives the
  score.
- **Report results, in your editor.** After a run, goozels reads gooze's reports
  and shows each file's mutation score at the top — e.g. *"gooze: score 87.50% —
  14 killed, 2 survived"* — and marks each line where a mutant survived, with
  the change your tests missed (*"surviving comparison mutant — tests still pass
  with `if n < lo {` → `if n <= lo {`"*). Several mutants of one line share one
  marker. Mutants on lines no test runs are marked as *not covered by any test*.
- **Out-of-date results are marked as such.** gooze records a hash of each
  source and test file. When the file (or its test) has changed since the run,
  the top line says so and gives the rerun command — *"gooze: results are from
  before calc.go changed; score then 87.50%; rerun: gooze run ./calc"* — and
  only markers on lines that still read as gooze tested them are kept.

**In your `gooze.yaml`**

- Autocomplete config keys (`output`, `run.parallel`, `log.level`, …) and their
  allowed values.
- Hover a key to see its type, meaning, and default.
- Live validation: unknown keys (with a *did you mean "run.parallel"?*
  suggestion), bad values (e.g. `log.level: loud`) and invalid YAML (an unclosed
  `[`) are flagged as you type.

The mutagens you can reference in `//gooze:ignore` are: `arithmetic`, `boolean`,
`numbers`, `comparison`, `logical`, `unary`, `branch`, `statement`, `loop`.
An empty `//gooze:ignore` skips them all.

---

## Requirements

- **Go** (to install goozels) — <https://go.dev/dl/>
- **gopls** — the Go language server: `go install golang.org/x/tools/gopls@latest`
- **gooze** — to produce the results goozels shows, and for the code lenses:
  `go install gooze.dev/pkg/gooze@latest`

goozels finds `gopls` and `gooze` on your `PATH` automatically.

## Install

```bash
go install gooze.dev/pkg/goozels/cmd/goozels@latest
```

This puts a `goozels` binary in your Go bin directory (usually `~/go/bin` — make
sure it's on your `PATH`). Check it:

```bash
goozels version   # prints "goozels <version>"
goozels help      # lists the modes: language server, check, hook, version
```

`goozels version` always prints one line starting with `goozels ` (scripts
and the agent integrations rely on that to tell goozels is installed and
recent enough; `goozels version -json` instead prints the proxied gopls's
version, as editors expect from the binary they start as gopls). Run with no
arguments, goozels is a language server waiting on stdin; in a terminal it
says so on stderr.

## Set up your editor

Pick your editor — each guide is a couple of lines of config:

| Editor | Guide |
|--------|-------|
| **VS Code** | [editors/vscode/README.md](editors/vscode/README.md) — easiest: add `"go.alternateTools": { "gopls": "goozels" }` to settings |
| **Neovim** | [editors/neovim.md](editors/neovim.md) |
| **GoLand / IntelliJ** | [editors/jetbrains.md](editors/jetbrains.md) |
| **Claude Code** | The `gooze` plugin from [gooze-dev/claude-plugin](https://github.com/gooze-dev/claude-plugin), which works alongside `gopls-lsp`; see [Use with AI coding agents](#use-with-ai-coding-agents) |
| **Codex** | [editors/codex/](editors/codex/README.md) — a post-edit hook |
| **opencode** | [editors/opencode/](editors/opencode/README.md) — a plugin for v1 and v2 |
| **Other LSP editors** | Launch `goozels` over stdio wherever you'd launch `gopls`, and also attach it to `gooze.yaml` files. |

> In an editor, goozels **replaces** gopls (it runs gopls for you behind the
> scenes). You don't need to configure gopls separately, and you shouldn't run
> both for the same files. Coding agents are different: they keep their own Go
> support, and goozels adds gooze diagnostics after each edit.

## Try it

Open the examples in [`testdata/`](testdata/):

- [`testdata/example.go`](testdata/example.go) — put your cursor after
  `//gooze:ignore ` and trigger completion; hover the mutagen names; notice the
  warning on the intentionally-wrong `bogus`.
- [`testdata/gooze.yaml`](testdata/gooze.yaml) — add a new key and let it complete;
  change `log.level` to something invalid and watch it get flagged.

## Options

goozels needs no configuration, but you can point it at specific binaries:

| Flag | Environment variable | Default | Purpose |
|------|----------------------|---------|---------|
| `--gopls PATH` | `GOOZELS_GOPLS` | `gopls` on `PATH` | The Go language server to run behind goozels |
| `--gooze PATH` | `GOOZELS_GOOZE` | `gooze` on `PATH` | The gooze binary the code lenses run |

Arguments after `--` are passed to gopls (`goozels -- -rpc.trace`); `serve`
(gopls's name for the server mode) is accepted and ignored. Any other argument
is an error.

### Check files from the command line

`goozels check` prints the same gooze diagnostics without an editor, one per line
in the usual `file:line:column: severity: message` form, survivors in line
order. Mutants of one line share one diagnostic that shows the original line
once and up to five mutated variants:

```console
$ goozels check internal/domain/ignore.go gooze.yaml
internal/domain/ignore.go:1:1: warning: gooze: score 67.65% — 69 killed, 32 survived, 1 not covered by any test
internal/domain/ignore.go:23:2: warning: gooze: 2 surviving mutants (comparison ×2) — tests still pass with `if len(r.names) == 0 {` → `if len(r.names) < 0 {`, `if len(r.names) <= 0 {`
internal/domain/ignore.go:108:3: warning: gooze: 6 surviving mutants (comparison ×5, numbers) — tests still pass with `if rule.all || len(rule.names) > 0 {` → `… != 0 {`, `… < 0 {`, `… <= 0 {`, `… == 0 {`, `… >= 0 {` and 1 more
internal/domain/ignore.go:120:3: warning: gooze: surviving comparison mutant — tests still pass with `if group.End() >= file.Package {` → `if group.End() > file.Package {`
internal/domain/ignore.go:199:3: warning: gooze: boolean mutant not covered by any test — `return false` → `return true`
gooze.yaml:3:3: warning: unknown gooze config key "run.paralel"; did you mean "run.parallel"?
```

(Shortened; the real output has a line for every line with survivors.)

- A file whose gooze results are fine gets just its score line.
- A Go file gooze has no results for gets a note on **stderr** —
  `goozels: no gooze results for x.go yet; run: gooze run ./pkg` — so missing
  data never looks like a clean file. When other files of the package have
  results, it says instead that gooze found nothing to mutate in the file, or
  that the file is newer than the last run.
- When the file or its test changed since the last run, the score line says the
  results are out of date and how to rerun (see above).
- Checking a `_test.go` file also prints the score and survivors of the source
  files it tests, as recorded in gooze's reports.
- `-max N` prints at most N diagnostics, then a line naming the files with
  more (`… 59 more; run goozels check internal/domain/workflow.go`) and a hint
  about `//gooze:ignore` for equivalent mutants. The agent hooks use 40.
- goozels finds the workspace root by walking up from each file to the nearest
  directory with `.gooze-reports/` (else the nearest `go.mod`), so it works from
  any directory; `-workdir DIR` overrides that.
- Exit status: **1** when an `error:` diagnostic was printed (or a file could not
  be read), **2** on usage errors, **0** otherwise — warnings such as surviving
  mutants don't fail it.

`goozels hook claude|codex` wraps the same check for the agent hooks below.

## Use with AI coding agents

goozels gives a coding agent the same gooze feedback you get in your editor,
right after it edits a file:

- the file's **mutation score** (*"gooze: score 67.65% — 69 killed, 32 survived, 1 not covered by any test"*),
- every **surviving mutant**, grouped by line, with the change your tests missed
  (*"surviving comparison mutant — tests still pass with `if n < lo {` → `if n <= lo {`"*),
- mutants on lines **no test runs** (*not covered by any test*),
- whether the results are **out of date** (the file or its tests changed since
  the run) and the command that refreshes them,
- a note when gooze has **no results** for the file yet,
- mistyped `//gooze:ignore` mutagens and `gooze.yaml` errors.

The hook's reply starts with `gooze results from the last run:` and stays small
(at most 40 diagnostics and about 4 KB). Within one agent session, a file's full
list is sent the first time it is reported and again whenever its results change
(after a gooze run, or when the file or its tests change); otherwise only its
score line is repeated. That per-session memory is a few bytes per file under
your user cache directory (`goozels/hook`), pruned after a week.

goozels **extends** the agent's Go support instead of replacing it: the agent
keeps its normal Go tooling, and a post-edit hook adds gooze's view on top.

| Agent | Setup | How the agent gets gooze diagnostics |
|-------|-------|--------------------------------------|
| **Claude Code** | [gooze-dev/claude-plugin](https://github.com/gooze-dev/claude-plugin) | The `gooze` plugin: a mutation-testing skill plus a hook after `Write`/`Edit`. Keep the official `gopls-lsp` plugin on. |
| **Codex** | [editors/codex/](editors/codex/README.md) | Hook after `apply_patch` (Codex 0.125 or later). |
| **opencode** | [editors/opencode/](editors/opencode/README.md) | Plugin after `edit`/`write`/patch, for v1 and v2. |

All three need `goozels` on the `PATH` of the shell you start the agent from, and
`gooze` if the agent should run mutation tests itself.

### Claude Code

Install the `gooze` plugin from its marketplace, in a Claude Code session:

```text
/plugin marketplace add gooze-dev/claude-plugin
/plugin install gooze@gooze
```

It adds a `/gooze:mutation-testing` skill that teaches Claude the run, read,
kill-survivors loop, and a post-edit hook that runs `goozels hook claude`. Leave
`gopls-lsp` enabled for Go code intelligence; the plugin adds no language server,
so the two never compete for a file. See the
[plugin's README](https://github.com/gooze-dev/claude-plugin#readme) for details.

### Codex

Copy [`editors/codex/hooks.json`](editors/codex/hooks.json) to `.codex/hooks.json`
in your project, then trust it once with `/hooks` (Codex runs a project hook only
after you review it, and again after it changes). Codex has no language-server
support, so the hook is the whole integration.

### opencode

Copy the plugin for your version (`opencode --version`) to
`.opencode/plugins/goozels.ts`:
[`v1/goozels.ts`](editors/opencode/v1/goozels.ts) for 1.x or
[`v2/goozels.ts`](editors/opencode/v2/goozels.ts) for 2.x. opencode keeps its
built-in gopls; the plugin appends gooze diagnostics to each edit's result.

### The loop

1. Run `gooze run ./pkg/...` (or ask the agent to).
2. The agent edits a file in that package and immediately sees its score and
   surviving mutants. Editing a test file shows the score and survivors of the
   source file it tests, since that's what the new tests should kill.
3. It writes tests aimed at those mutants.
4. It reruns gooze and checks that the score went up.

Example prompts:

- *"Run gooze on `./internal/domain/...`, then kill the surviving mutants in
  `ignore.go` without changing production code."*
- *"Which files in `./pkg/lsp/...` have the lowest gooze score? Start with the
  worst one and add tests until it's above 90%."*
- *"Look at the gooze diagnostics in `parser.go` and tell me which survivors are
  real test gaps and which are equivalent mutants worth a `//gooze:ignore`."*

Scores only change after gooze runs again, so step 4 matters: a new test doesn't
move the score until you rerun gooze.

**Tell the agent about it once.** Add this to `AGENTS.md` (Codex, opencode), or
to `CLAUDE.md` if you use Claude Code without the plugin — the plugin's
`/gooze:mutation-testing` skill already covers it:

```markdown
## Mutation testing (gooze)
- After adding or changing tests, rerun `gooze run <pkg>` for the package you touched.
- Treat gooze diagnostics (mutation score, surviving mutants) as targets: add tests
  that kill those mutants. Don't change production code just to raise the score.
- If you edit Go files with shell commands instead of your edit tool, run
  `goozels check <file>` afterwards; the post-edit hook only sees edit-tool changes.
```

**Skip the permission prompts in Claude Code** by allowing the gooze commands in
`.claude/settings.json`:

```json
{
  "permissions": {
    "allow": ["Bash(gooze run *)", "Bash(gooze report view)", "Bash(goozels check *)"]
  }
}
```

### Troubleshooting (agents)

- **No gooze diagnostics after an edit** — run `goozels check <file>` yourself. It
  prints the score, or says on stderr that gooze has no results for the file yet.
  If it fails, `goozels` isn't on the agent's `PATH` (or is too old: `goozels
  version` must print a line starting with `goozels`).
- **Claude Code** — open `/plugin` and check the **Errors** tab; run
  `claude --debug` to see hook and language-server failures. Plugins don't run in
  cloud sessions or safe mode.
- **Codex** — check that the hook is trusted in `/hooks`; project hooks also need
  the project's `.codex/` folder to be trusted.
- **opencode** — make sure you copied the plugin matching your major version; v1
  plugins don't load in v2.
- **"results are from before … changed"** — the file changed since gooze last
  ran it, so goozels hides markers that could point at the wrong line. Rerun the
  command it gives.

## Troubleshooting

- **No Go features at all** — goozels couldn't start `gopls`. Make sure `gopls` is
  installed and on your `PATH`, or set `GOOZELS_GOPLS` to its full path.
- **The gooze code lenses do nothing / error** — `gooze` isn't installed or on
  your `PATH`. Install it (see Requirements) or set `GOOZELS_GOOZE`.
- **No report scores appear** — run mutation testing first (the **gooze: run
  package** lens or `gooze run ./...`). goozels reads the `.gooze-reports/`
  directory in your workspace root. Scores refresh after every gooze run.
- **Only a "results are from before … changed" line** — you edited the file
  since gooze ran; markers whose line still reads as gooze tested it stay, the
  rest come back after the rerun the line names.
- **Doubled completions in GoLand** — GoLand has its own Go engine, so it shows
  both. See the note in the [JetBrains guide](editors/jetbrains.md).
- **`goozels: command not found`** — your Go bin directory isn't on `PATH`. Add
  `export PATH="$PATH:$(go env GOPATH)/bin"` to your shell profile.

## License

MIT.

---

Contributing or curious how it works? See [CONTRIBUTING.md](CONTRIBUTING.md).
