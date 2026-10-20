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
- **▶ gooze run**, **gooze list**, and **📊 gooze report** buttons (code lenses)
  at the top of every Go file — run mutation testing or open the report without
  leaving the editor.
- **Report results, in your editor.** After a run, goozels reads gooze's reports
  and shows each file's mutation score at the top — e.g. *"gooze: 2/10 mutants
  survived — score 80.00%"* — and marks each line where a mutant survived, with
  the change your tests missed (*"surviving comparison mutant — tests still pass
  with: `if x <= 0`"*). Mutants on lines no test runs are marked as not covered.

**In your `gooze.yaml`**

- Autocomplete config keys (`output`, `run.parallel`, `log.level`, …) and their
  allowed values.
- Hover a key to see its type, meaning, and default.
- Live validation: unknown keys and bad values (e.g. `log.level: loud`) are
  flagged as you type.

The mutagens you can reference in `//gooze:ignore` are: `arithmetic`, `boolean`,
`numbers`, `comparison`, `logical`, `unary`, `branch`, `statement`, `loop`.
An empty `//gooze:ignore` skips them all.

---

## Requirements

- **Go** (to install goozels) — <https://go.dev/dl/>
- **gopls** — the Go language server: `go install golang.org/x/tools/gopls@latest`
- **gooze** — only needed for the *run / list* buttons:
  `go install gooze.dev/pkg/gooze@latest`

goozels finds `gopls` and `gooze` on your `PATH` automatically.

## Install

```bash
go install gooze.dev/pkg/goozels/cmd/goozels@latest
```

This puts a `goozels` binary in your Go bin directory (usually `~/go/bin` — make
sure it's on your `PATH`). Check it:

```bash
goozels --help
```

## Set up your editor

Pick your editor — each guide is a couple of lines of config:

| Editor | Guide |
|--------|-------|
| **VS Code** | [editors/vscode/README.md](editors/vscode/README.md) — easiest: add `"go.alternateTools": { "gopls": "goozels" }` to settings |
| **Neovim** | [editors/neovim.md](editors/neovim.md) |
| **GoLand / IntelliJ** | [editors/jetbrains.md](editors/jetbrains.md) |
| **Claude Code** | [editors/claude-code/](editors/claude-code/README.md) — a ready-to-load plugin that works alongside `gopls-lsp`; see [Use with AI coding agents](#use-with-ai-coding-agents) |
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
| `--gooze PATH` | `GOOZELS_GOOZE` | `gooze` on `PATH` | The gooze binary used by the run / list buttons |

### Check files from the command line

`goozels check` prints the same gooze diagnostics without an editor, one per line
in the usual `file:line:column: severity: message` form, and nothing for clean
files:

```bash
$ goozels check internal/domain/ignore.go gooze.yaml
internal/domain/ignore.go:1:1: warning: gooze: 32/102 mutants survived, 1 not covered — score 67.65%
internal/domain/ignore.go:108:3: warning: gooze: surviving comparison mutant — tests still pass with: if rule.all || len(rule.names) < 0 {
gooze.yaml:13:1: error: missing a space after ":"; gooze will ignore this whole file and use its defaults
```

Run it from your project root, or pass `-workdir DIR` to say where
`.gooze-reports/` lives. `goozels hook claude|codex` wraps the same check for the
agent hooks below.

## Use with AI coding agents

goozels gives a coding agent the same gooze feedback you get in your editor,
right after it edits a file:

- the file's **mutation score** (*"gooze: 32/102 mutants survived, 1 not covered — score 67.65%"*),
- every **surviving mutant**, on its line, with the change your tests missed
  (*"surviving comparison mutant — tests still pass with: `if line == 0 …`"*),
- mutants on lines **no test runs**,
- mistyped `//gooze:ignore` mutagens and `gooze.yaml` errors.

goozels **extends** the agent's Go support instead of replacing it: the agent
keeps its normal Go tooling, and a post-edit hook adds gooze's view on top.

| Agent | Setup | How the agent gets gooze diagnostics |
|-------|-------|--------------------------------------|
| **Claude Code** | [editors/claude-code/](editors/claude-code/README.md) | Plugin hook after `Write`/`Edit`, plus goozels as the language server for `gooze.yaml`. Keep the official `gopls-lsp` plugin on. |
| **Codex** | [editors/codex/](editors/codex/README.md) | Hook after `apply_patch` (Codex 0.125 or later). |
| **opencode** | [editors/opencode/](editors/opencode/README.md) | Plugin after `edit`/`write`/patch, for v1 and v2. |

All three need `goozels` on the `PATH` of the shell you start the agent from, and
`gooze` if the agent should run mutation tests itself.

### Claude Code

```bash
claude --plugin-dir /absolute/path/to/goozels/editors/claude-code
```

`--plugin-dir` loads the plugin for that session only (it shows up as `@inline`
in `/plugin`); to install it permanently, publish the directory to a plugin
marketplace and use `/plugin install`. Leave `gopls-lsp` enabled: it handles
`.go`, while this plugin handles `.yaml`/`.yml` and the post-edit hook, so they
never compete for a file.

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
   surviving mutants.
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

**Tell the agent about it once.** Add this to `CLAUDE.md` (Claude Code) or
`AGENTS.md` (Codex, opencode):

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
    "allow": ["Bash(gooze run *)", "Bash(gooze list *)", "Bash(goozels check *)"]
  }
}
```

### Troubleshooting (agents)

- **No gooze diagnostics after an edit** — run `goozels check <file>` yourself. If
  it prints nothing, gooze has no findings for that file (or you haven't run gooze
  yet). If it fails, `goozels` isn't on the agent's `PATH`.
- **Claude Code** — open `/plugin` and check the **Errors** tab; run
  `claude --debug` to see hook and language-server failures. Plugins don't run in
  cloud sessions or safe mode.
- **Codex** — check that the hook is trusted in `/hooks`; project hooks also need
  the project's `.codex/` folder to be trusted.
- **opencode** — make sure you copied the plugin matching your major version; v1
  plugins don't load in v2.
- **Markers point at the wrong line** — the file changed since gooze last ran it.
  Rerun gooze.

## Troubleshooting

- **No Go features at all** — goozels couldn't start `gopls`. Make sure `gopls` is
  installed and on your `PATH`, or set `GOOZELS_GOPLS` to its full path.
- **The ▶ run / list buttons do nothing / error** — `gooze` isn't installed or on
  your `PATH`. Install it (see Requirements) or set `GOOZELS_GOOZE`.
- **No report scores appear** — run mutation testing first (the **▶ gooze run**
  button or `gooze run ./...`). goozels reads the `.gooze-reports/` directory in
  your workspace root. Scores refresh after every gooze run. The per-line markers
  come from each mutant's diff, so they point at the line as gooze tested it and
  can drift if you edit the file before rerunning gooze.
- **Doubled completions in GoLand** — GoLand has its own Go engine, so it shows
  both. See the note in the [JetBrains guide](editors/jetbrains.md).
- **`goozels: command not found`** — your Go bin directory isn't on `PATH`. Add
  `export PATH="$PATH:$(go env GOPATH)/bin"` to your shell profile.

## License

MIT.

---

Contributing or curious how it works? See [CONTRIBUTING.md](CONTRIBUTING.md).
