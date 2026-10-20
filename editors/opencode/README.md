# opencode setup

goozels plugs into opencode as a small **plugin**: after the agent edits a Go
file or a gooze config, the plugin runs `goozels check --max 40` on it and
appends the gooze diagnostics (mutation score, surviving mutants, `//gooze:ignore`
and `gooze.yaml` errors), plus notes such as "no gooze results for x.go yet", to
the edit's result. opencode keeps its built-in gopls.

opencode v1 and v2 have different plugin APIs, so there is one file for each.
Check yours with `opencode --version`.

| Version | Plugin | Edit tools it watches |
|---------|--------|-----------------------|
| 1.x | [`v1/goozels.ts`](v1/goozels.ts) | `edit`, `write`, `apply_patch` |
| 2.x | [`v2/goozels.ts`](v2/goozels.ts) | `edit`, `write`, `patch` |

## 1. Install goozels

```bash
go install gooze.dev/pkg/goozels/cmd/goozels@latest
```

`goozels` (and `gooze`, to run mutation tests) must be on the `PATH` of the shell
you start `opencode` from.

## 2. Add the plugin

Copy the file for your version into your project's plugin folder, where opencode
loads it automatically:

```bash
mkdir -p .opencode/plugins
cp /path/to/goozels/editors/opencode/v1/goozels.ts .opencode/plugins/goozels.ts   # or v2/
```

To use it in every project, copy it to `~/.config/opencode/plugins/` instead.

## Optional: gooze.yaml checks while editing (v1 only)

opencode v1 can also run goozels as an extra language server for YAML. Merge
[`opencode.json`](opencode.json) into your project's `opencode.json`:

```json
{
  "lsp": {
    "goozels": { "command": ["goozels"], "extensions": [".yaml", ".yml"] }
  }
}
```

Using a new server name keeps the built-in gopls and YAML servers running next to
it. You don't need this for the agent: v1 only shows the model language-server
*errors*, and the plugin already reports gooze.yaml problems after each edit.
opencode v2 has no language-server support.

> Don't name the server `"gopls"`: that replaces opencode's built-in Go server.
