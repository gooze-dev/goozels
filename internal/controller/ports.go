package controller

import (
	"context"

	"gooze.dev/goozels/internal/adapter/lsp"
)

// GoozeRunner executes gooze subcommands. It is the port the controller depends
// on so the external gooze process can be substituted (e.g. mocked) in tests.
// Implemented by adapter/goozecli.Runner.
type GoozeRunner interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

// Documents is the port for the open-document store the controller reads and
// updates. Implemented by adapter/lsp.Store.
type Documents interface {
	Open(uri, text string, version int)
	Change(uri string, version int, changes []lsp.TextDocumentContentChangeEvent)
	Close(uri string)
	Text(uri string) (string, bool)
	URIs() []string
}
