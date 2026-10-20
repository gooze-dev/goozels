# JetBrains setup (GoLand / IntelliJ IDEA)

JetBrains IDEs run external language servers through the **LSP4IJ** plugin
(Red Hat). goozels works as one of these servers, giving you `//gooze:ignore`
support, `gooze.yaml` validation, and the **gooze run / gooze run --estimate /
gooze report** code-lens actions inside the IDE.

## 1. Install LSP4IJ

`Settings → Plugins → Marketplace`, search for **LSP4IJ**, install, restart.

## 2. Register goozels

`Settings → Languages & Frameworks → Language Servers → +` (New Language Server),
then on the **Server** tab:

- **Name:** `goozels`
- **Command:** `goozels`
  (use an absolute path if it isn't on the IDE's `PATH`; add `--gopls
  /path/to/gopls` and `--gooze /path/to/gooze` if those binaries aren't on
  `PATH` either)

On the **Mappings** tab add:

| Kind | Value |
|------|-------|
| Language | `Go` |
| File name pattern | `gooze.yaml` |

Apply, and reopen a `.go` file or a `gooze.yaml`.

## 3. Use it

- In `.go` files: completion, hover and a diagnostic for `//gooze:ignore`
  mutagen names, plus **gooze: run package** / **gooze: estimate** /
  **gooze: report** code lenses at the top of the file. Clicking a lens runs
  gooze in the workspace root (run and estimate on the file's package) and
  reports progress via IDE notifications; the finish message carries the
  mutation score.
- Each file's gooze score and surviving mutants appear as diagnostics. After
  you edit the file (or save its test) they are marked as out of date until
  gooze runs again.
- In `gooze.yaml`: key/value completion, hover docs, and validation.

> **GoLand note:** GoLand already has its own Go engine, so for `.go` files the
> goozels (gopls-backed) completions appear *in addition* to GoLand's. If you find
> that noisy, map goozels to the `gooze.yaml` pattern only and keep using
> GoLand's native Go support — the config features and execute commands still work.
> In IntelliJ IDEA (without the bundled Go engine) you can map `Go` as well to get
> full Go intelligence through goozels.

## Running gooze from anywhere

The code-lens actions are also exposed as LSP commands (`gooze.run`,
`gooze.estimate`, `gooze.view`), so any LSP client that lists server commands
can invoke them — not just via the code lenses. The old `gooze.list` identifier
still works and runs `gooze run --estimate`, which replaced `gooze list`.
