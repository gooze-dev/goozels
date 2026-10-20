package controller

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gooze.dev/pkg/goozels/internal/adapter/gopls"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
)

// TestRealGoplsInitialize boots the real gopls binary behind the proxy and
// checks that initialize round-trips and that gooze trigger characters are
// merged into gopls's actual advertised capabilities. It is skipped when gopls
// is not installed.
func TestRealGoplsInitialize(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH; skipping real-gopls integration test")
	}

	proc, err := gopls.Start("gopls")
	if err != nil {
		t.Fatalf("start gopls: %v", err)
	}
	defer func() { _ = proc.Kill(); _ = proc.Wait() }()

	c2sR, c2sW := io.Pipe() // client -> server
	s2cR, s2cW := io.Pipe() // server -> client

	srv := NewServer(s2cW, proc.Stdin, productionDeps("gooze"))
	go func() { _ = srv.Run(c2sR, proc.Stdout) }()

	clientW := jsonrpc.NewWriter(c2sW)
	clientR := jsonrpc.NewReader(s2cR)

	inbox := make(chan *jsonrpc.Message, 64)
	go func() {
		for {
			msg, err := clientR.Read()
			if err != nil {
				return
			}
			// Auto-reply to any server->client request so gopls never blocks.
			if msg.IsRequest() {
				_ = clientW.Write(&jsonrpc.Message{JSONRPC: "2.0", ID: msg.ID, Result: json.RawMessage("null")})
				continue
			}
			inbox <- msg
		}
	}()

	initParams := map[string]any{
		"processId":    nil,
		"rootUri":      nil,
		"capabilities": map[string]any{},
	}
	raw, _ := json.Marshal(initParams)
	if err := clientW.Write(&jsonrpc.Message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: raw}); err != nil {
		t.Fatalf("send initialize: %v", err)
	}

	resp := awaitID(t, inbox, "1", 20*time.Second)
	var result struct {
		Capabilities struct {
			CompletionProvider struct {
				TriggerCharacters []string `json:"triggerCharacters"`
			} `json:"completionProvider"`
			ExecuteCommandProvider struct {
				Commands []string `json:"commands"`
			} `json:"executeCommandProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	got := result.Capabilities.CompletionProvider.TriggerCharacters
	joined := strings.Join(got, "")
	for _, want := range []string{" ", ",", ":"} {
		if !strings.Contains(joined, want) {
			t.Errorf("merged trigger characters %v missing %q", got, want)
		}
	}

	cmds := strings.Join(result.Capabilities.ExecuteCommandProvider.Commands, ",")
	for _, want := range []string{"gooze.run", "gooze.estimate"} {
		if !strings.Contains(cmds, want) {
			t.Errorf("executeCommandProvider commands missing %q (got %v)", want, result.Capabilities.ExecuteCommandProvider.Commands)
		}
	}
	// gopls's own commands must survive the merge.
	if !strings.Contains(cmds, "gopls.") {
		t.Errorf("gopls commands were dropped from executeCommandProvider (got %v)", result.Capabilities.ExecuteCommandProvider.Commands)
	}
}

func awaitID(t *testing.T, inbox chan *jsonrpc.Message, id string, d time.Duration) *jsonrpc.Message {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case msg := <-inbox:
			if msg.IsResponse() && msg.IDKey() == id {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for response id=%s", id)
			return nil
		}
	}
}
