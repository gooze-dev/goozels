package controller

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"gooze.dev/goozels/internal/adapter/jsonrpc"
	"gooze.dev/goozels/internal/adapter/lsp"
	"gooze.dev/goozels/internal/adapter/reportstore"
	"gooze.dev/goozels/internal/domain/command"
	"gooze.dev/goozels/internal/domain/report"
	"gooze.dev/goozels/internal/model"
)

// goozeRunTimeout bounds a single gooze invocation so a hung run can't leak.
const goozeRunTimeout = time.Hour

// reportsDirName is gooze's default reports directory, relative to the workspace.
const reportsDirName = ".gooze-reports"

// handleCodeLens serves code lenses: gooze run/list lenses are added to gopls's
// lenses on .go files; config files have none.
func (s *Server) handleCodeLens(msg *jsonrpc.Message) {
	uri := uriOf(msg.Params)
	switch {
	case lsp.IsConfigURI(uri):
		s.respond(msg, []lsp.CodeLens{})
	case lsp.IsGoURI(uri):
		s.registerAugment(msg.IDKey(), func(resp *jsonrpc.Message) { s.augmentCodeLens(msg, resp) })
		s.sendGopls(msg)
	default:
		s.sendGopls(msg)
	}
}

func (s *Server) augmentCodeLens(req, resp *jsonrpc.Message) {
	uri := uriOf(req.Params)
	text, _ := s.store.Text(uri)
	ours := lsp.CodeLenses(model.Lines(text), command.GoFileLenses("./..."))
	resp.Result = mergeCodeLensResult(resp.Result, ours)
	s.sendClient(resp)
}

// mergeCodeLensResult appends our lenses to gopls's code-lens array.
func mergeCodeLensResult(goplsResult json.RawMessage, ours []lsp.CodeLens) json.RawMessage {
	var base []json.RawMessage
	if trimmed := skipSpace(goplsResult); len(trimmed) > 0 && trimmed[0] == '[' {
		_ = json.Unmarshal(trimmed, &base)
	}

	for _, l := range ours {
		if raw, err := json.Marshal(l); err == nil {
			base = append(base, raw)
		}
	}

	if raw, err := json.Marshal(base); err == nil {
		return raw
	}

	return goplsResult
}

// handleExecuteCommand runs gooze commands locally and forwards everything else
// to gopls.
func (s *Server) handleExecuteCommand(msg *jsonrpc.Message) {
	var p lsp.ExecuteCommandParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.sendGopls(msg)
		return
	}

	if command.IsGooze(p.Command) {
		go s.runGoozeCommand(msg, p)
		return
	}

	s.sendGopls(msg)
}

// runGoozeCommand executes a gooze subcommand and reports progress to the editor.
// It runs on its own goroutine so a long run never blocks the message pump.
func (s *Server) runGoozeCommand(req *jsonrpc.Message, p lsp.ExecuteCommandParams) {
	sub, ok := command.Subcommand(p.Command)
	if !ok {
		s.respondNull(req)
		return
	}

	args := goozeArgs(sub, p.Arguments)

	s.notify("window/showMessage", lsp.ShowMessageParams{
		Type:    lsp.MessageInfo,
		Message: "gooze " + strings.Join(args, " ") + " started…",
	})

	ctx, cancel := context.WithTimeout(context.Background(), goozeRunTimeout)
	defer cancel()

	out, err := s.gooze.Run(ctx, s.getWorkdir(), args...)

	s.reportGoozeResult(sub, out, err)
	s.respond(req, nil)
}

// goozeArgs builds the gooze CLI args for a subcommand: run/list take a package
// pattern (default "./...") while view takes none.
func goozeArgs(sub string, rawArgs []json.RawMessage) []string {
	if sub == "view" {
		return []string{sub}
	}

	pathArg := "./..."

	if len(rawArgs) > 0 {
		var a string
		if json.Unmarshal(rawArgs[0], &a) == nil && a != "" {
			pathArg = a
		}
	}

	return []string{sub, pathArg}
}

// reportGoozeResult notifies the editor of a run's outcome and, on success,
// refreshes report-derived diagnostics so a fresh report shows up.
func (s *Server) reportGoozeResult(sub, out string, err error) {
	if out != "" {
		s.notify("window/logMessage", lsp.ShowMessageParams{Type: lsp.MessageLog, Message: out})
	}

	if err != nil {
		s.notify("window/showMessage", lsp.ShowMessageParams{
			Type:    lsp.MessageError,
			Message: "gooze " + sub + " failed: " + err.Error(),
		})

		return
	}

	s.notify("window/showMessage", lsp.ShowMessageParams{
		Type:    lsp.MessageInfo,
		Message: "gooze " + sub + " finished",
	})
	s.refreshReports()
}

// refreshReports re-publishes diagnostics for every open Go file so newly written
// gooze reports are reflected.
func (s *Server) refreshReports() {
	for _, uri := range s.store.URIs() {
		if lsp.IsGoURI(uri) {
			s.republishGo(uri)
		}
	}
}

// reportStore reads gooze reports from the workspace's reports directory.
func (s *Server) reportStore() reportstore.Store {
	return reportstore.New(filepath.Join(s.getWorkdir(), reportsDirName))
}

// reportDiagnostics returns gooze report-derived diagnostics for a .go file: a
// summary plus best-effort survivor markers. It returns nil when no report exists.
func (s *Server) reportDiagnostics(uri string) []model.Diagnostic {
	rep, ok := s.reportStore().ReportForFile(lsp.URIPath(uri))
	if !ok {
		return nil
	}

	diags := []model.Diagnostic{report.SummaryDiagnostic(rep)}

	return append(diags, report.SurvivorDiagnostics(rep)...)
}

// workspaceRoot extracts the workspace root path from initialize params.
func workspaceRoot(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}

	var p struct {
		RootURI          string `json:"rootUri"`
		RootPath         string `json:"rootPath"`
		WorkspaceFolders []struct {
			URI string `json:"uri"`
		} `json:"workspaceFolders"`
	}
	if json.Unmarshal(params, &p) != nil {
		return ""
	}

	if len(p.WorkspaceFolders) > 0 && p.WorkspaceFolders[0].URI != "" {
		return lsp.URIPath(p.WorkspaceFolders[0].URI)
	}

	if p.RootURI != "" {
		return lsp.URIPath(p.RootURI)
	}

	return p.RootPath
}
