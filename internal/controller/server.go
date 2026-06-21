// Package controller is the orchestration layer: it proxies the LSP message
// stream between the editor (client) and gopls (server), and augments the
// handful of methods that carry gooze features. It depends on the adapter and
// domain layers; nothing depends on it except the cmd entrypoint.
package controller

import (
	"encoding/json"
	"io"
	"sync"

	"gooze.dev/goozels/internal/adapter/goozecli"
	"gooze.dev/goozels/internal/adapter/jsonrpc"
	"gooze.dev/goozels/internal/adapter/lsp"
)

// Server proxies one editor<->gopls session.
type Server struct {
	toClient *jsonrpc.Writer
	toGopls  *jsonrpc.Writer
	store    Documents
	gooze    GoozeRunner

	mu         sync.Mutex
	pending    map[string]func(resp *jsonrpc.Message) // gopls response augmenters, keyed by request id
	goplsDiags map[string][]lsp.Diagnostic            // last gopls diagnostics per .go URI
	workdir    string                                 // workspace root for executing gooze
}

// NewServer builds a server writing to the given client and gopls streams.
// goozePath is the gooze binary used by execute commands (defaults to "gooze").
func NewServer(clientOut, goplsOut io.Writer, goozePath string) *Server {
	return &Server{
		toClient:   jsonrpc.NewWriter(clientOut),
		toGopls:    jsonrpc.NewWriter(goplsOut),
		store:      lsp.NewStore(),
		gooze:      goozecli.New(goozePath),
		pending:    map[string]func(*jsonrpc.Message){},
		goplsDiags: map[string][]lsp.Diagnostic{},
		workdir:    ".",
	}
}

// setWorkdir records the workspace root (used as the cwd for gooze commands).
func (s *Server) setWorkdir(dir string) {
	if dir == "" {
		return
	}

	s.mu.Lock()
	s.workdir = dir
	s.mu.Unlock()
}

// getWorkdir returns the recorded workspace root.
func (s *Server) getWorkdir() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.workdir
}

// Run reads from the client and gopls streams until both close. It returns when
// the client stream ends (editor disconnected) or an unrecoverable read error
// occurs.
func (s *Server) Run(clientIn, goplsIn io.Reader) error {
	clientReader := jsonrpc.NewReader(clientIn)
	goplsReader := jsonrpc.NewReader(goplsIn)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		s.pump(goplsReader, s.handleGopls)
	}()

	// The client pump runs on this goroutine; when the editor disconnects we
	// return and let the caller tear down gopls.
	s.pump(clientReader, s.handleClient)
	wg.Wait()

	return nil
}

// pump reads messages and dispatches them until the stream ends.
func (s *Server) pump(r *jsonrpc.Reader, handle func(*jsonrpc.Message)) {
	for {
		msg, err := r.Read()
		if err != nil {
			return
		}

		handle(msg)
	}
}

// registerAugment records a function to run on gopls's response to id.
func (s *Server) registerAugment(id string, fn func(*jsonrpc.Message)) {
	if id == "" {
		return
	}

	s.mu.Lock()
	s.pending[id] = fn
	s.mu.Unlock()
}

// takeAugment pops a registered augmenter for id.
func (s *Server) takeAugment(id string) (func(*jsonrpc.Message), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fn, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}

	return fn, ok
}

// --- send helpers ---

func (s *Server) sendClient(msg *jsonrpc.Message) { _ = s.toClient.Write(msg) }
func (s *Server) sendGopls(msg *jsonrpc.Message)  { _ = s.toGopls.Write(msg) }

// respond replies to a request with a result value.
func (s *Server) respond(req *jsonrpc.Message, result any) {
	raw, err := json.Marshal(result)
	if err != nil {
		raw = json.RawMessage("null")
	}

	s.sendClient(&jsonrpc.Message{JSONRPC: "2.0", ID: req.ID, Result: raw})
}

// respondNull replies to a request with a null result.
func (s *Server) respondNull(req *jsonrpc.Message) {
	s.sendClient(&jsonrpc.Message{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage("null")})
}

// notify sends a notification to the client.
func (s *Server) notify(method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}

	s.sendClient(&jsonrpc.Message{JSONRPC: "2.0", Method: method, Params: raw})
}
