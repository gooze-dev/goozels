package controller

import (
	"encoding/json"

	"gooze.dev/goozels/internal/adapter/jsonrpc"
	"gooze.dev/goozels/internal/adapter/lsp"
	"gooze.dev/goozels/internal/domain/goozeyml"
	"gooze.dev/goozels/internal/domain/ignore"
	"gooze.dev/goozels/internal/model"
)

// handleClient routes a message from the editor.
func (s *Server) handleClient(msg *jsonrpc.Message) {
	switch {
	case msg.IsRequest():
		s.handleClientRequest(msg)
	case msg.IsNotification():
		s.handleClientNotification(msg)
	case msg.IsResponse():
		// Response to a gopls-originated request (e.g. workspace/configuration).
		s.sendGopls(msg)
	}
}

func (s *Server) handleClientRequest(msg *jsonrpc.Message) {
	switch msg.Method {
	case "initialize":
		// Capture the workspace root for gooze execution, then forward and merge
		// gooze capabilities into gopls's initialize response.
		s.setWorkdir(workspaceRoot(msg.Params))
		s.registerAugment(msg.IDKey(), s.augmentInitialize)
		s.sendGopls(msg)
	case "textDocument/completion":
		s.routePositional(msg, s.configCompletion, func() {
			s.registerAugment(msg.IDKey(), func(resp *jsonrpc.Message) { s.augmentGoCompletion(msg, resp) })
			s.sendGopls(msg)
		})
	case "textDocument/hover":
		s.routePositional(msg, s.configHover, func() {
			s.registerAugment(msg.IDKey(), func(resp *jsonrpc.Message) { s.augmentGoHover(msg, resp) })
			s.sendGopls(msg)
		})
	case "textDocument/codeLens":
		s.handleCodeLens(msg)
	case "workspace/executeCommand":
		s.handleExecuteCommand(msg)
	default:
		// Other requests on a config document can't be served by gopls (it never
		// saw the file); answer null. Everything else is forwarded.
		if lsp.IsConfigURI(uriOf(msg.Params)) {
			s.respondNull(msg)
			return
		}

		s.sendGopls(msg)
	}
}

// routePositional dispatches a textDocument request by URI: config files are
// handled locally via configFn; otherwise goFn runs (typically forward+augment).
func (s *Server) routePositional(msg *jsonrpc.Message, configFn func(*jsonrpc.Message), goFn func()) {
	uri := uriOf(msg.Params)
	switch {
	case lsp.IsConfigURI(uri):
		configFn(msg)
	default:
		goFn()
	}
}

func (s *Server) handleClientNotification(msg *jsonrpc.Message) {
	switch msg.Method {
	case "textDocument/didOpen":
		s.onDidOpen(msg)
	case "textDocument/didChange":
		s.onDidChange(msg)
	case "textDocument/didClose":
		s.onDidClose(msg)
	default:
		// Swallow notifications about config documents (gopls doesn't track
		// them); forward everything else.
		if uri := uriOf(msg.Params); uri != "" && lsp.IsConfigURI(uri) {
			return
		}

		s.sendGopls(msg)
	}
}

// handleGopls routes a message from gopls.
func (s *Server) handleGopls(msg *jsonrpc.Message) {
	switch {
	case msg.IsResponse():
		if fn, ok := s.takeAugment(msg.IDKey()); ok {
			fn(msg)
			return
		}

		s.sendClient(msg)
	case msg.IsNotification():
		if msg.Method == "textDocument/publishDiagnostics" {
			s.onGoplsDiagnostics(msg)
			return
		}

		s.sendClient(msg)
	case msg.IsRequest():
		// gopls -> client request (workspace/configuration, registerCapability...).
		s.sendClient(msg)
	}
}

// --- document lifecycle ---

func (s *Server) onDidOpen(msg *jsonrpc.Message) {
	var p lsp.DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.sendGopls(msg)
		return
	}

	td := p.TextDocument
	s.store.Open(td.URI, td.Text, td.Version)

	if lsp.IsConfigURI(td.URI) {
		s.publishConfig(td.URI)
		return // never forward YAML to gopls
	}

	s.sendGopls(msg)
}

func (s *Server) onDidChange(msg *jsonrpc.Message) {
	var p lsp.DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.sendGopls(msg)
		return
	}

	s.store.Change(p.TextDocument.URI, p.TextDocument.Version, p.ContentChanges)

	if lsp.IsConfigURI(p.TextDocument.URI) {
		s.publishConfig(p.TextDocument.URI)
		return
	}

	s.sendGopls(msg)
	// Refresh gooze diagnostics promptly; gopls's own publish will re-merge.
	s.republishGo(p.TextDocument.URI)
}

func (s *Server) onDidClose(msg *jsonrpc.Message) {
	var p lsp.DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.sendGopls(msg)
		return
	}

	uri := p.TextDocument.URI
	s.store.Close(uri)

	if lsp.IsConfigURI(uri) {
		s.notify("textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{URI: uri, Diagnostics: []lsp.Diagnostic{}})
		return
	}

	s.mu.Lock()
	delete(s.goplsDiags, uri)
	s.mu.Unlock()
	s.sendGopls(msg)
}

// --- positional features ---

func (s *Server) configCompletion(msg *jsonrpc.Message) {
	uri, lineNum, line, col, ok := s.posContext(msg)
	if !ok {
		s.respond(msg, lsp.CompletionList{Items: []lsp.CompletionItem{}})
		return
	}

	text, _ := s.store.Text(uri)
	items := goozeyml.Complete(text, lineNum, col)
	s.respond(msg, lsp.CompletionResult(lineNum, line, lsp.CompletionKindProperty, items))
}

func (s *Server) configHover(msg *jsonrpc.Message) {
	uri, lineNum, _, col, ok := s.posContext(msg)
	if !ok {
		s.respondNull(msg)
		return
	}

	text, _ := s.store.Text(uri)

	h, ok := goozeyml.Hover(text, lineNum, col)
	if !ok {
		s.respondNull(msg)
		return
	}

	s.respond(msg, lsp.HoverResult(model.Lines(text), h))
}

func (s *Server) augmentGoCompletion(req, resp *jsonrpc.Message) {
	uri, lineNum, line, col, ok := s.posContext(req)
	if !ok {
		s.sendClient(resp)
		return
	}

	_ = uri

	items := ignore.Complete(line, col)
	if len(items) == 0 {
		s.sendClient(resp)
		return
	}

	ours := lsp.CompletionResult(lineNum, line, lsp.CompletionKindEnumMember, items)
	resp.Result = mergeCompletionResult(resp.Result, ours.Items)
	s.sendClient(resp)
}

func (s *Server) augmentGoHover(req, resp *jsonrpc.Message) {
	_, lineNum, line, col, ok := s.posContext(req)
	if !ok {
		s.sendClient(resp)
		return
	}

	h, ok := ignore.Hover(line, lineNum, col)
	if !ok {
		s.sendClient(resp) // not over a directive: keep gopls's hover
		return
	}

	text, _ := s.store.Text(uriOf(req.Params))
	if raw, err := json.Marshal(lsp.HoverResult(model.Lines(text), h)); err == nil {
		resp.Result = raw
	}

	s.sendClient(resp)
}

// --- diagnostics ---

func (s *Server) onGoplsDiagnostics(msg *jsonrpc.Message) {
	var p lsp.PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.sendClient(msg)
		return
	}

	s.mu.Lock()
	s.goplsDiags[p.URI] = p.Diagnostics
	s.mu.Unlock()
	s.publishMergedGo(p.URI, p.Version)
}

// republishGo re-emits merged diagnostics using the last cached gopls set.
func (s *Server) republishGo(uri string) {
	s.publishMergedGo(uri, nil)
}

func (s *Server) publishMergedGo(uri string, version *int) {
	s.mu.Lock()
	goplsDiags := s.goplsDiags[uri]
	s.mu.Unlock()

	merged := append([]lsp.Diagnostic{}, goplsDiags...)
	if text, ok := s.store.Text(uri); ok {
		merged = append(merged, lsp.Diagnostics(text, ignore.Diagnose(text))...)
		merged = append(merged, lsp.Diagnostics(text, s.reportDiagnostics(uri))...)
	}

	s.notify("textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
		URI:         uri,
		Version:     version,
		Diagnostics: merged,
	})
}

func (s *Server) publishConfig(uri string) {
	text, ok := s.store.Text(uri)
	if !ok {
		return
	}

	diags := lsp.Diagnostics(text, goozeyml.Diagnose(text))
	s.notify("textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{URI: uri, Diagnostics: diags})
}

// --- shared helpers ---

// posContext resolves the URI, line number, line text and byte column for a
// positional request. ok is false when the document or position is unavailable.
func (s *Server) posContext(msg *jsonrpc.Message) (uri string, lineNum int, line string, col int, ok bool) {
	var p lsp.TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		return "", 0, "", 0, false
	}

	text, found := s.store.Text(p.TextDocument.URI)
	if !found {
		return "", 0, "", 0, false
	}

	lines := model.Lines(text)
	if p.Position.Line < 0 || p.Position.Line >= len(lines) {
		return "", 0, "", 0, false
	}

	line = lines[p.Position.Line]

	return p.TextDocument.URI, p.Position.Line, line, lsp.ByteColumn(line, p.Position.Character), true
}

// uriOf extracts textDocument.uri from raw params, or "" when absent.
func uriOf(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}

	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ""
	}

	return p.TextDocument.URI
}
