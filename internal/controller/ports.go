package controller

import (
	"context"

	"gooze.dev/pkg/gooze/pkg/config"
	"gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/pkg/lsp"
)

// Deps are the adapters the controller depends on. The cmd entrypoint builds
// them and injects them through NewServer, so the controller never imports the
// adapter layer.
type Deps struct {
	Gooze     GoozeRunner
	Documents Documents
	Reports   report.Reader
	Config    config.Registry
	Files     FileReader

	// Releases, when set, is asked once per session for the latest goozels
	// release; a newer one than Version is announced to the editor.
	Releases ReleaseChecker
	Version  string
}

// FileReader reads files from disk, such as the test files whose changes make
// gooze's results out of date. Implemented by adapter/osfs.FS.
type FileReader interface {
	ReadFile(name string) ([]byte, error)
}

// ReleaseChecker reports the latest published goozels version (e.g. "v0.3.0").
// It is split so nothing waits on the network: Cached answers from local state
// only, and Refresh, which may be slow or fail when offline, runs in the
// background. Implemented by adapter/releasecheck.Checker.
type ReleaseChecker interface {
	// Cached returns the last known tag and whether it is fresh enough that no
	// Refresh is needed.
	Cached() (tag string, fresh bool)
	// Refresh looks up the latest tag and records it for later Cached calls.
	Refresh(ctx context.Context) (string, error)
}

// GoozeRunner executes gooze subcommands. It is the port the controller depends
// on so the external gooze process can be substituted (e.g. mocked) in tests.
// Implemented by adapter/goozecli.Runner.
type GoozeRunner interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
	// RunFlags lists the long flags of the installed gooze's run command,
	// e.g. "run" for --run, so features can depend on what it supports.
	RunFlags(ctx context.Context) (map[string]bool, error)
}

// Documents is the port for the open-document store the controller reads and
// updates. Implemented by adapter/docstore.Store.
type Documents interface {
	Open(uri, text string, version int)
	Change(uri string, version int, changes []lsp.TextDocumentContentChangeEvent)
	Close(uri string)
	Text(uri string) (string, bool)
	URIs() []string
}
