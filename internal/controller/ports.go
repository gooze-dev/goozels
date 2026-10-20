package controller

import (
	"context"

	"gooze.dev/pkg/goozels/internal/domain/report"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// Deps are the adapters the controller depends on. The cmd entrypoint builds
// them and injects them through NewServer, so the controller never imports the
// adapter layer.
type Deps struct {
	Gooze     GoozeRunner
	Documents Documents
	Reports   Reports
}

// GoozeRunner executes gooze subcommands. It is the port the controller depends
// on so the external gooze process can be substituted (e.g. mocked) in tests.
// Implemented by adapter/goozecli.Runner.
type GoozeRunner interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
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

// Reports is the port for reading gooze mutation reports from a workspace.
// Implemented by adapter/reportstore.Workspace.
type Reports interface {
	ReportForFile(workdir, absPath string) (report.FileReport, bool)
}
