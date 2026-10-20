package controller

import (
	"cmp"
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gooze.dev/pkg/gooze/pkg/config"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/internal/domain/document"
	"gooze.dev/pkg/goozels/internal/domain/report"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// goozeRunTimeout bounds a single gooze invocation so a hung run can't leak.
const goozeRunTimeout = time.Hour

// maxLogBytes caps the gooze output goozels copies into the editor's log: a
// whole project's `gooze report view` runs to hundreds of kilobytes.
const maxLogBytes = 16 << 10

// handleCodeLens serves code lenses: gooze run, estimate and report lenses,
// scoped to the file's package, are added to gopls's lenses on .go files;
// config files have none.
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
	pattern := command.PackagePattern(s.getWorkdir(), lsp.URIPath(uri))
	ours := lspCodeLenses(document.Lines(text), command.GoFileLenses(pattern))
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
	pattern := firstStringArg(p.Arguments)

	args, ok := command.Args(p.Command, pattern)
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

	s.reportGoozeResult(label, scoreScope(p.Command, pattern), out, err)
	s.respond(req, nil)
}

// scoreScope is the package pattern whose score a finished command reports:
// the pattern a run tested, every package for the report view, and none for
// an estimate, which changes no results.
func scoreScope(id, pattern string) string {
	switch id {
	case command.Run:
		return cmp.Or(pattern, "./...")
	case command.View:
		return "./..."
	default:
		return ""
	}
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

// reportGoozeResult notifies the editor of a run's outcome, with the score of
// the packages scope selects, and on success refreshes report-derived
// diagnostics so a fresh report shows up. The output is logged, capped at
// maxLogBytes.
func (s *Server) reportGoozeResult(label, scope, out string, err error) {
	// Equivalent: no string sorts below "", so > "" is != "" (>= "" is killed by a test).
	if out != "" { //gooze:ignore comparison
		s.notify("window/logMessage", lsp.ShowMessageParams{Type: lsp.MessageLog, Message: capLog(label, out)})
	}

	if err != nil {
		s.notify("window/showMessage", lsp.ShowMessageParams{
			Type:    lsp.MessageError,
			Message: label + " failed: " + err.Error(),
		})

		return
	}

	msg := label + " finished"
	// Equivalent: no string sorts below "", so > "" is != "".
	if score := s.score(scope); score != "" { //gooze:ignore comparison
		msg += " — " + score
	}

	s.notify("window/showMessage", lsp.ShowMessageParams{Type: lsp.MessageInfo, Message: msg})
	s.refreshReports()
}

// score summarizes the reports of the packages a pattern selects, or returns
// "" when there are none.
func (s *Server) score(pattern string) string {
	// Equivalent: no string sorts below "", so < "" never holds and <= "" is == "".
	if pattern == "" { //gooze:ignore comparison
		return ""
	}

	workdir := s.getWorkdir()

	all, err := s.reports.ReadDir(filepath.Join(workdir, config.DefaultReportsDir))
	if err != nil {
		return ""
	}

	matched := report.Matching(all, workdir, pattern)
	// Equivalent: a length is never negative, so <= 0 is == 0 (< 0 and == 1 are killed by a test).
	if len(matched) == 0 { //gooze:ignore comparison
		return ""
	}

	return report.ScoreText(report.Summarize(matched))
}

// capLog shortens command output to maxLogBytes, cut at a line end, and says
// how much was left out.
func capLog(label, out string) string {
	if len(out) <= maxLogBytes {
		return out
	}

	cut := strings.LastIndexByte(out[:maxLogBytes], '\n') + 1
	// Equivalent: cut is never negative, so <= 0 is == 0 (>= 0 is killed by a test).
	if cut == 0 { //gooze:ignore comparison
		cut = maxLogBytes
	}

	rest, unit := strings.Count(strings.TrimSuffix(out[cut:], "\n"), "\n")+1, " more lines"
	// Equivalent: at least one line is left out, so <= 1 is == 1.
	if rest == 1 { //gooze:ignore comparison
		unit = " more line"
	}

	return out[:cut] + "… " + strconv.Itoa(rest) + unit + "; run `" + label + "` in a terminal for all of it"
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
