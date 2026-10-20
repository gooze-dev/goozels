# Contributing to goozels

Thanks for hacking on goozels! This document covers how it's built and how to
work on it. For installing and using the server, see the [README](README.md).

## How it works

goozels is a **proxy in front of `gopls`**. Your editor connects to `goozels`
exactly as it would to `gopls`; goozels forwards every LSP message to a real gopls
child process (so you keep full Go intelligence) and augments the handful of
messages that carry gooze features.

```
Editor ──LSP──> goozels ──┬─ .go         → gopls (+ gooze:ignore features)
                          └─ gooze.yaml  → handled by goozels (gopls never sees it)
```

Routing is by document URI: `.go` files are forwarded to gopls and the responses
(completion, hover, diagnostics, code lenses) are merged with gooze additions;
`gooze.yaml` files are handled entirely inside goozels. Because LSP allows only one
authoritative set of diagnostics per file, goozels caches gopls's diagnostics and
publishes the **union** of gopls + gooze findings.

## Layout

The codebase follows the same clean-architecture layering as gooze. Dependencies
point inward (`cmd` → `controller` → `domain` → `model`); adapters implement the
controller's ports (`internal/controller/ports.go`) and are wired in `cmd`:

| Layer | Package | Responsibility |
|-------|---------|----------------|
| model | `internal/model` | LSP-agnostic result types (positions, completions, diagnostics, commands) |
| domain | `internal/domain/{command,report,document}` | command/code-lens definitions (package-scoped patterns), report diagnostics (score, survivors grouped by line, staleness from the content hashes gooze records), line splitting; the mutagen registry, `gooze:ignore` parser, config schema and report files come from gooze's `gooze.dev/pkg/gooze/pkg/{mutagen,directive,config,report}` |
| domain | `internal/domain/{ignore,goozeyml}` | feature logic: completion / hover / diagnostics |
| adapter | `internal/adapter/{docstore,gopls,goozecli,reportcache,osfs,hookstate}` | open-document store, gopls subprocess, gooze CLI runner, cache in front of gooze's report reader, disk reads (test files for staleness), per-session hook memory |
| controller | `internal/controller` | the proxy engine: routing, augmentation, diagnostics merge, model↔LSP mapping; `Checker` serves the same diagnostics to `goozels check` and agent hooks |
| pkg | `pkg/{jsonrpc,lsp}` | protocol libraries with no project imports: JSON-RPC framing, LSP wire types and UTF-16 helpers |
| cmd | `cmd/goozels` | composition root; usage and `version`, the `check` and `hook claude\|codex` subcommands and their shared rendering (caps, per-session summaries) |

The domain and model layers know nothing about LSP or gopls and are unit-tested in
isolation. The controller is covered by an in-process proxy test (with a fake
gopls) plus a real-gopls integration test that is skipped when gopls is absent.

## Development

```bash
go build ./...      # build
go test ./...       # run all tests
go test -race ./... # with the race detector
```

Or via the Makefile (mirrors gooze's):

```bash
make build          # build the binary into ./.bin
make test           # tests with coverage report
make install-tools  # golangci-lint, mockery, pre-commit, cobra-cli, semgrep
make lint           # golangci-lint (config in .golangci.yml)
make semgrep        # architecture, correctness and security rules (.semgrep/)
make fmt            # gofmt + golangci-lint formatters
make mocks          # regenerate mocks (config in .mockery.yaml)
make install-precommit
```

### Conventions

- `golangci-lint run` must be clean (the config matches gooze's strict set).
- Every source file has a matching `*_test.go`.
- Keep the domain/model layers free of LSP/gopls imports; translation lives in the
  controller.
- `make semgrep` must be clean. `.semgrep/architecture.yaml` enforces the layering:
  `model` imports nothing from the project and has no free functions, `domain`
  never imports `adapter`/`controller`/`cmd`, and `controller` never imports
  `adapter`/`cmd` (add a port instead).

## Editor integrations

The thin editor glue lives under [`editors/`](editors/): a minimal VS Code
extension, a Neovim `lspconfig` snippet, and a JetBrains (LSP4IJ) guide.
