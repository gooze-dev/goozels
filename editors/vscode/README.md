# goozels in VS Code

There are two ways to use goozels with VS Code.

## Option A (recommended): tell the Go extension to use goozels as gopls

The official Go extension launches `gopls` for you. Because goozels speaks the
exact same protocol and proxies the real gopls, you can substitute it:

```jsonc
// settings.json
{
  "go.alternateTools": {
    "gopls": "goozels"
  }
}
```

Then ensure goozels can find the real gopls — it looks on `PATH`, or set:

```jsonc
{
  "go.toolsEnvVars": { "GOOZELS_GOPLS": "/absolute/path/to/gopls" }
}
```

That gives you all gopls features plus gooze `//gooze:ignore` support in `.go`
files with no extra extension. (Config-file support for `gooze.yaml` uses the
standalone extension below or the YAML association.)

## Option B: the bundled extension

This directory is a minimal Language Client that launches `goozels` for Go files
and gooze config files. Build it with:

```bash
cd editors/vscode
npm install
npm run compile
```

then press F5 in VS Code to launch an Extension Development Host. Configure the
binary path via the `goozels.path` setting (default `goozels`).

> When using Option B, disable the Go extension's own language server
> (`"go.useLanguageServer": false`) so you don't run two servers for `.go` files.
