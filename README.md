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
  survived — score 80%"* — and marks the approximate spots where mutants survived.

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
| **Other LSP editors** | Launch `goozels` over stdio wherever you'd launch `gopls`, and also attach it to `gooze.yaml` files. |

> goozels **replaces** gopls in your editor (it runs gopls for you behind the
> scenes). You don't need to configure gopls separately, and you shouldn't run
> both for the same files.

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

## Troubleshooting

- **No Go features at all** — goozels couldn't start `gopls`. Make sure `gopls` is
  installed and on your `PATH`, or set `GOOZELS_GOPLS` to its full path.
- **The ▶ run / list buttons do nothing / error** — `gooze` isn't installed or on
  your `PATH`. Install it (see Requirements) or set `GOOZELS_GOOZE`.
- **No report scores appear** — run mutation testing first (the **▶ gooze run**
  button or `gooze run ./...`). goozels reads the `.gooze-reports/` directory in
  your workspace root. The per-line "surviving mutant" markers are **approximate**:
  gooze doesn't record exact line positions in its reports, so the file-level
  score is exact but the in-line markers are a best-effort hint.
- **Doubled completions in GoLand** — GoLand has its own Go engine, so it shows
  both. See the note in the [JetBrains guide](editors/jetbrains.md).
- **`goozels: command not found`** — your Go bin directory isn't on `PATH`. Add
  `export PATH="$PATH:$(go env GOPATH)/bin"` to your shell profile.

## License

MIT.

---

Contributing or curious how it works? See [CONTRIBUTING.md](CONTRIBUTING.md).
