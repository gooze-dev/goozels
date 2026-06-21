package controller

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"gooze.dev/pkg/goozels/internal/adapter/jsonrpc"
)

// --- shared test harness ---

// harness wires a test "client" and a fake "gopls" to a real Server through
// in-memory pipes, mirroring the production stdio topology. Like a real editor,
// it drains the client stream continuously so the server never blocks writing.
type harness struct {
	clientW   *jsonrpc.Writer       // test -> server (client side)
	inbox     chan *jsonrpc.Message // every server -> client message
	goplsSeen chan *jsonrpc.Message
}

func newHarness(t *testing.T, fakeGopls func(req *jsonrpc.Message) *jsonrpc.Message) *harness {
	return newHarnessWithGooze(t, fakeGopls, "gooze")
}

func newHarnessWithGooze(t *testing.T, fakeGopls func(req *jsonrpc.Message) *jsonrpc.Message, goozePath string) *harness {
	t.Helper()
	c2sR, c2sW := io.Pipe() // client -> server
	s2cR, s2cW := io.Pipe() // server -> client
	g2sR, g2sW := io.Pipe() // gopls -> server
	s2gR, s2gW := io.Pipe() // server -> gopls

	srv := NewServer(s2cW, s2gW, goozePath)
	go func() { _ = srv.Run(c2sR, g2sR) }()

	h := &harness{
		clientW:   jsonrpc.NewWriter(c2sW),
		inbox:     make(chan *jsonrpc.Message, 256),
		goplsSeen: make(chan *jsonrpc.Message, 32),
	}

	// Continuous client reader, mirroring a real editor.
	clientReader := jsonrpc.NewReader(s2cR)
	go func() {
		for {
			msg, err := clientReader.Read()
			if err != nil {
				return
			}
			h.inbox <- msg
		}
	}()

	// Fake gopls: read what the server forwards, optionally reply.
	goplsReader := jsonrpc.NewReader(s2gR)
	goplsWriter := jsonrpc.NewWriter(g2sW)
	go func() {
		for {
			msg, err := goplsReader.Read()
			if err != nil {
				return
			}
			h.goplsSeen <- msg
			if msg.IsRequest() {
				if reply := fakeGopls(msg); reply != nil {
					reply.ID = msg.ID
					_ = goplsWriter.Write(reply)
				}
			}
		}
	}()
	return h
}

func (h *harness) send(t *testing.T, method string, id string, params any) {
	t.Helper()
	msg := &jsonrpc.Message{JSONRPC: "2.0", Method: method}
	if id != "" {
		msg.ID = json.RawMessage(id)
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		msg.Params = raw
	}
	if err := h.clientW.Write(msg); err != nil {
		t.Fatalf("send %s: %v", method, err)
	}
}

// awaitResponse drains client-bound messages until the response with the given
// id arrives.
func (h *harness) awaitResponse(t *testing.T, id string) *jsonrpc.Message {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-h.inbox:
			if msg.IsResponse() && msg.IDKey() == id {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for response id=%s", id)
			return nil
		}
	}
}

// awaitNotification drains client-bound messages until a notification with the
// given method arrives.
func (h *harness) awaitNotification(t *testing.T, method string) *jsonrpc.Message {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-h.inbox:
			if msg.IsNotification() && msg.Method == method {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for notification %s", method)
			return nil
		}
	}
}

// drainGopls clears already-observed gopls messages.
func drainGopls(h *harness) {
	for {
		select {
		case <-h.goplsSeen:
		default:
			return
		}
	}
}

func goplsResponder(req *jsonrpc.Message) *jsonrpc.Message {
	switch req.Method {
	case "initialize":
		result := map[string]any{
			"capabilities": map[string]any{
				"completionProvider": map[string]any{
					"triggerCharacters": []string{"."},
				},
				"hoverProvider": true,
			},
		}
		raw, _ := json.Marshal(result)
		return &jsonrpc.Message{Result: raw}
	case "textDocument/completion":
		// gopls returns one item, as a CompletionList.
		result := map[string]any{
			"isIncomplete": false,
			"items":        []map[string]any{{"label": "fmt", "kind": 9}},
		}
		raw, _ := json.Marshal(result)
		return &jsonrpc.Message{Result: raw}
	case "textDocument/codeLens":
		result := []map[string]any{{
			"range":   map[string]any{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 0, "character": 0}},
			"command": map[string]any{"title": "gopls lens", "command": "gopls.noop"},
		}}
		raw, _ := json.Marshal(result)
		return &jsonrpc.Message{Result: raw}
	default:
		return &jsonrpc.Message{Result: json.RawMessage("null")}
	}
}

// --- server.go tests ---

func TestNewServerDefaultWorkdir(t *testing.T) {
	s := NewServer(io.Discard, io.Discard, "gooze")
	if got := s.getWorkdir(); got != "." {
		t.Errorf("default workdir = %q, want %q", got, ".")
	}
	s.setWorkdir("/proj/root")
	if got := s.getWorkdir(); got != "/proj/root" {
		t.Errorf("workdir after set = %q, want /proj/root", got)
	}
	s.setWorkdir("") // empty is ignored
	if got := s.getWorkdir(); got != "/proj/root" {
		t.Errorf("empty setWorkdir changed value to %q", got)
	}
}
