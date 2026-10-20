# Claude Code setup

This directory is a ready-to-load Claude Code plugin. It **extends** Claude's Go
support instead of replacing it: keep the official `gopls-lsp` plugin for Go
code intelligence, and this plugin adds gooze on top.

- **After every Go edit**, a hook runs `goozels hook claude` and adds the file's
  gooze diagnostics to Claude's context: mutation score, each surviving mutant
  with the change your tests missed, and `//gooze:ignore` mistakes.
- **For `gooze.yaml`**, goozels runs as the language server for YAML files, so
  config typos are flagged as Claude edits them.

## 1. Install goozels

```bash
go install gooze.dev/pkg/goozels/cmd/goozels@latest
```

`goozels` (and `gooze`, to run mutation tests) must be on the `PATH` of the shell
you launch `claude` from.

## 2. Load the plugin

```bash
claude --plugin-dir /absolute/path/to/goozels/editors/claude-code
```

This loads the plugin for that session only (it shows up as `@inline` in
`/plugin`). To install it permanently, publish this directory to a plugin
marketplace and install it with `/plugin install`.

Keep `gopls-lsp` enabled for Go definitions, references and type errors. The two
plugins don't overlap: `gopls-lsp` handles `.go`, this one handles `.yaml`/`.yml`
plus the post-edit hook.

## What's in it

| File | Purpose |
|------|---------|
| [`.claude-plugin/plugin.json`](.claude-plugin/plugin.json) | Plugin name and description |
| [`hooks/hooks.json`](hooks/hooks.json) | After `Write`/`Edit`, runs `goozels hook claude` and adds gooze diagnostics to Claude's context |
| [`.lsp.json`](.lsp.json) | Starts `goozels` for `.yaml` and `.yml` files |

The hook never blocks an edit. It prints nothing for files gooze has nothing to
say about, and caps its output at 40 lines (run `goozels check <file>` for the
rest).

> Claude Code picks a language server by file extension only, so `.yaml`/`.yml`
> send **every** YAML file to goozels, not just `gooze.yaml`. goozels only checks
> gooze config files and stays quiet on the rest. If you'd rather not route other
> YAML through it, delete `.lsp.json`: the hook still checks `gooze.yaml` after
> each edit.

## Using goozels instead of gopls-lsp

goozels also proxies gopls, so it can be Claude's only Go language server. To do
that, add `".go": "go"` to `.lsp.json` and disable `gopls-lsp` (Claude Code uses
one server per extension). You gain little over the hook, so the default setup
above is recommended.

For the full workflow (example prompts, a `CLAUDE.md` snippet, troubleshooting),
see [Use with AI coding agents](../../README.md#use-with-ai-coding-agents) in the
main README.
