# Claude Code setup

This directory is a ready-to-load Claude Code plugin. It runs goozels as Claude's
language server for `.go`, `.yaml` and `.yml` files, so Claude sees gooze diagnostics
(mutation scores, survivor markers, `//gooze:ignore` and `gooze.yaml` errors)
after every edit, on top of everything gopls provides.

## 1. Install goozels

```bash
go install gooze.dev/pkg/goozels/cmd/goozels@latest
```

`goozels` (and `gopls`, and `gooze` if you want to run mutation tests) must be on
the `PATH` of the shell you launch `claude` from.

## 2. Load the plugin

```bash
claude --plugin-dir /absolute/path/to/goozels/editors/claude-code
```

This loads the plugin for that session only (it shows up as `@inline` in
`/plugin`). To install it permanently, publish this directory to a plugin
marketplace and install it with `/plugin install`.

## 3. Turn off the official gopls plugin

If `gopls-lsp` is enabled, disable it in `/plugin`, or in your settings:

```json
{
  "enabledPlugins": {
    "gopls-lsp@claude-plugins-official": false
  }
}
```

Then run `/reload-plugins`. Only one server is used per file extension; if both
are enabled, the `/plugin` **Errors** tab shows
`LSP server "<name>" is not used for .go files`.

## What's in it

| File | Purpose |
|------|---------|
| [`.claude-plugin/plugin.json`](.claude-plugin/plugin.json) | Plugin name and description |
| [`.lsp.json`](.lsp.json) | Starts `goozels` for `.go`, `.yaml` and `.yml` files |

> Claude Code picks a language server by file extension only, so `.yaml`/`.yml` send
> **every** YAML file to goozels, not just `gooze.yaml`. goozels only checks gooze
> config files, but if you'd rather not route other YAML through it, delete the
> `".yaml"` and `".yml"` lines from `.lsp.json`.

To use specific `gopls` / `gooze` binaries, add an `env` block to the server in
`.lsp.json`, e.g. `"env": { "GOOZELS_GOPLS": "/usr/local/bin/gopls" }`.

For the full workflow (example prompts, a `CLAUDE.md` snippet, troubleshooting),
see [Use with Claude Code](../../README.md#use-with-claude-code) in the main
README.
