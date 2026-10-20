package controller

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/internal/domain/document"
	"gooze.dev/pkg/goozels/internal/model"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// goozeRunTimeout bounds a single gooze invocation so a hung run can't leak.
const goozeRunTimeout = time.Hour

// handleCodeLens serves code lenses: gooze run/list lenses are added to gopls's
// lenses on .go files; config files have none.
func (s *Server) handleCodeLens(msg *jsonrpc.Message) {
	uri := uriOf(msg.Params)
	switch {
	case s.isConfigURI(uri):
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
	ours := lspCodeLenses(document.Lines(text), command.GoFileLenses("./..."))
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
	args, ok := command.Args(p.Command, firstStringArg(p.Arguments))
	if !ok {
		s.respondNull(req)
		return
	}

	label := "gooze " + strings.Join(args, " ")

	s.notify("window/showMessage", lsp.ShowMessageParams{
		Type:    lsp.MessageInfo,
		Message: label + " started…",
	})

	ctx, cancel := context.WithTimeout(context.Background(), goozeRunTimeout)
	defer cancel()

	out, err := s.gooze.Run(ctx, s.getWorkdir(), args...)

	s.reportGoozeResult(label, out, err)
	s.respond(req, nil)
}

// firstStringArg returns the first command argument when it is a non-empty
// string (the package pattern code lenses pass), or "".
func firstStringArg(rawArgs []json.RawMessage) string {
	var a string
	if len(rawArgs) > 0 && json.Unmarshal(rawArgs[0], &a) == nil {
		return a
	}

	return ""
}

// reportGoozeResult notifies the editor of a run's outcome and, on success,
// refreshes report-derived diagnostics so a fresh report shows up.
func (s *Server) reportGoozeResult(label, out string, err error) {
	if out != "" {
		s.notify("window/logMessage", lsp.ShowMessageParams{Type: lsp.MessageLog, Message: out})
	}

	if err != nil {
		s.notify("window/showMessage", lsp.ShowMessageParams{
			Type:    lsp.MessageError,
			Message: label + " failed: " + err.Error(),
		})

		return
	}

	s.notify("window/showMessage", lsp.ShowMessageParams{
		Type:    lsp.MessageInfo,
		Message: label + " finished",
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

// reportDiagnostics returns gooze report-derived diagnostics for an open .go
// file. It returns nil when no report exists.
func (s *Server) reportDiagnostics(uri string) []model.Diagnostic {
	return reportDiagnostics(s.reports, s.getWorkdir(), lsp.URIPath(uri))
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
