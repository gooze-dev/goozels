package controller

import (
	"context"
	"encoding/json"
	"errors"
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
	path := lsp.URIPath(uri)
	pattern := command.PackagePattern(s.getWorkdir(), path)

	lenses := command.GoFileLenses(pattern)
	// Function lenses need a gooze with --run, and only non-test files have
	// code to mutate.
	if s.goozeSupports("run") && !strings.HasSuffix(path, "_test.go") {
		lenses = append(lenses, command.FuncLenses(text, pattern)...)
	}

	ours := lspCodeLenses(document.Lines(text), lenses)
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
	args, ok := command.Args(p.Command, stringArgs(p.Arguments)...)
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

	s.reportGoozeResult(label, scoreScope(p.Command, args), out, err)

	if p.Command == command.RunFunction && (err == nil || errors.Is(err, command.ErrBelowThreshold)) {
		s.offerMerge(args)
	}

	s.respond(req, nil)
}

// scoreScope is the package pattern whose score a finished command reports,
// from the gooze arguments it ran: the pattern a run tested (always last),
// every package for the report view, and none for an estimate, which changes
// no results.
func scoreScope(id string, args []string) string {
	switch id {
	case command.Run:
		return args[len(args)-1]
	case command.View:
		return "./..."
	default:
		return ""
	}
}

// stringArgs returns the leading command arguments that are strings: the
// package pattern code lenses pass, then a function lens's --run pattern.
func stringArgs(rawArgs []json.RawMessage) []string {
	var strs []string

	for _, raw := range rawArgs {
		var a string
		if json.Unmarshal(raw, &a) != nil {
			break
		}

		strs = append(strs, a)
	}

	return strs
}

// reportGoozeResult notifies the editor of a run's outcome, with the score of
// the packages scope selects, and on success refreshes report-derived
// diagnostics so a fresh report shows up. A run below a gooze.yaml threshold
// wrote its reports too: it is a warning naming the low scores. The output is
// logged, capped at maxLogBytes.
func (s *Server) reportGoozeResult(label, scope, out string, err error) {
	if out != "" {
		s.notify("window/logMessage", lsp.ShowMessageParams{Type: lsp.MessageLog, Message: capLog(label, out)})
	}

	if errors.Is(err, command.ErrBelowThreshold) {
		s.notify("window/showMessage", lsp.ShowMessageParams{
			Type:    lsp.MessageWarn,
			Message: label + " finished below a threshold: " + breachesOf(out),
		})
		s.refreshReports()

		return
	}

	if err != nil {
		s.notify("window/showMessage", lsp.ShowMessageParams{
			Type:    lsp.MessageError,
			Message: label + " failed: " + err.Error(),
		})

		return
	}

	msg := label + " finished"
	if score := s.score(scope); score != "" {
		msg += " — " + score
	}

	s.notify("window/showMessage", lsp.ShowMessageParams{Type: lsp.MessageInfo, Message: msg})
	s.refreshReports()
}

// breachesOf returns the scores below a threshold that gooze printed after
// "Error: ", one per line, joined with "; ".
func breachesOf(out string) string {
	_, after, ok := strings.Cut(out, "Error: ")
	if !ok {
		return ""
	}

	var lines []string

	for line := range strings.Lines(after) {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "; ")
}

// score summarizes the reports of the packages a pattern selects, or returns
// "" when there are none.
func (s *Server) score(pattern string) string {
	if pattern == "" {
		return ""
	}

	workdir := s.getWorkdir()

	all, err := s.reports.ReadDir(filepath.Join(workdir, config.DefaultReportsDir))
	if err != nil {
		return ""
	}

	matched := report.Matching(all, workdir, pattern)
	if len(matched) == 0 {
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
	// Equivalent: cut is LastIndexByte + 1, never negative, so <= 0 is == 0.
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
