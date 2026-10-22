package controller

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

func TestInitializeMergesTriggerCharacters(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	resp := h.awaitResponse(t, "1")

	var result struct {
		Capabilities struct {
			CompletionProvider struct {
				TriggerCharacters []string `json:"triggerCharacters"`
			} `json:"completionProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	got := strings.Join(result.Capabilities.CompletionProvider.TriggerCharacters, "")
	for _, want := range []string{".", " ", ",", ":"} {
		if !strings.Contains(got, want) {
			t.Errorf("trigger characters %q missing %q", got, want)
		}
	}
}

func TestGoCompletionAugmentedWithMutagens(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")

	uri := "file:///tmp/x.go"
	text := "package main\n\nfunc f() {\n\tx := 1 //gooze:ignore ar\n}\n"
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": text},
	})
	// Cursor right after "ar" on line index 3.
	line := "\tx := 1 //gooze:ignore ar"
	h.send(t, "textDocument/completion", "2", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 3, "character": len(line)},
	})
	resp := h.awaitResponse(t, "2")

	var list struct {
		Items []struct {
			Label string `json:"label"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatalf("decode completion: %v", err)
	}
	var hasGopls, hasMutagen bool
	for _, it := range list.Items {
		switch it.Label {
		case "fmt":
			hasGopls = true
		case "arithmetic":
			hasMutagen = true
		}
	}
	if !hasGopls {
		t.Error("gopls completion item 'fmt' was dropped by the proxy")
	}
	if !hasMutagen {
		t.Error("mutagen completion 'arithmetic' was not added")
	}
}

func TestConfigCompletionHandledLocally(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	drainGopls(h) // discard the forwarded initialize

	uri := "file:///proj/gooze.yaml"
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "yaml", "version": 1, "text": "ou"},
	})
	h.send(t, "textDocument/completion", "3", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 0, "character": 2},
	})
	resp := h.awaitResponse(t, "3")

	var list struct {
		Items []struct {
			Label string `json:"label"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatalf("decode config completion: %v", err)
	}
	found := false
	for _, it := range list.Items {
		if it.Label == "output" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'output' config key completion, got %+v", list.Items)
	}

	// The config document must never be forwarded to gopls.
	for {
		select {
		case m := <-h.goplsSeen:
			if u := uriOf(m.Params); u == uri {
				t.Fatalf("config document %s was forwarded to gopls via %s", uri, m.Method)
			}
		default:
			return
		}
	}
}

func TestGoozeYmlForwardedToGopls(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	drainGopls(h)

	uri := "file:///proj/gooze.yml"
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "yaml", "version": 1, "text": "bogus: 1\n"},
	})

	select {
	case m := <-h.goplsSeen:
		if m.Method != "textDocument/didOpen" || uriOf(m.Params) != uri {
			t.Errorf("gopls got %s for %s, want didOpen for %s", m.Method, uriOf(m.Params), uri)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("gooze.yml was not forwarded to gopls")
	}
}

func TestUnknownConfigRequestAnsweredNull(t *testing.T) {
	h := newHarness(t, goplsResponder)
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")

	uri := "file:///proj/gooze.yaml"
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "yaml", "version": 1, "text": "output: x\n"},
	})
	// A request kind we don't implement for config files must be answered locally
	// (null), never forwarded to gopls.
	h.send(t, "textDocument/definition", "7", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 0, "character": 0},
	})
	resp := h.awaitResponse(t, "7")
	if string(resp.Result) != "null" {
		t.Errorf("expected null result for config definition, got %s", resp.Result)
	}
}

func TestNotificationsAboutConfigDocumentsAreSwallowed(t *testing.T) {
	s := newBufServer()
	s.handleClient(notification(t, "textDocument/willSave", map[string]any{"textDocument": map[string]any{"uri": "file:///proj/gooze.yaml"}}))
	s.handleClient(notification(t, "textDocument/willSave", map[string]any{"textDocument": map[string]any{"uri": "file:///proj/main.go"}}))
	s.handleClient(notification(t, "$/setTrace", map[string]any{"value": "off"}))

	var forwarded []string
	for _, m := range sent(s.gopls) {
		forwarded = append(forwarded, m.Method+" "+uriOf(m.Params))
	}
	want := []string{"textDocument/willSave file:///proj/main.go", "$/setTrace "}
	if strings.Join(forwarded, ",") != strings.Join(want, ",") {
		t.Errorf("forwarded to gopls = %q, want %q", forwarded, want)
	}
}

func TestGoplsNotificationsReachTheClient(t *testing.T) {
	// Methods that sort before and after textDocument/publishDiagnostics.
	for _, method := range []string{"$/progress", "window/logMessage"} {
		t.Run(method, func(t *testing.T) {
			s := newBufServer()
			s.handleGopls(notification(t, method, map[string]any{"type": 3, "message": "hi"}))

			msgs := sent(s.client)
			if len(msgs) != 1 || msgs[0].Method != method {
				t.Fatalf("client got %v, want one %s", msgs, method)
			}
		})
	}
}

func TestGoplsDiagnosticsAreMergedWithGoozes(t *testing.T) {
	s := newBufServer()
	uri := "file:///tmp/x.go"
	s.handleClient(notification(t, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": "package x\n\nvar v = 1 //gooze:ignore bogus\n"},
	}))
	s.client.Reset()

	s.handleGopls(notification(t, "textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": []map[string]any{{"range": map[string]any{}, "message": "from gopls", "source": "compiler"}},
	}))

	msgs := sent(s.client)
	if len(msgs) != 1 {
		t.Fatalf("client got %d messages, want one publishDiagnostics", len(msgs))
	}
	var p lsp.PublishDiagnosticsParams
	if err := json.Unmarshal(msgs[0].Params, &p); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	var sources []string
	for _, d := range p.Diagnostics {
		sources = append(sources, d.Source)
	}
	if p.URI != uri || len(p.Diagnostics) != 2 {
		t.Errorf("published %s with sources %v, want gopls's diagnostic and gooze's for %s", p.URI, sources, uri)
	}
}

func TestConfigDiagnosticsPublishedOnOpenAndClearedOnClose(t *testing.T) {
	s := newBufServer()
	uri := "file:///proj/gooze.yaml"
	doc := map[string]any{"uri": uri}

	s.handleClient(notification(t, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": "yaml", "version": 1, "text": "bogus: 1\n"},
	}))
	opened := publishedDiagnostics(t, sent(s.client), uri)
	if len(opened) == 0 {
		t.Fatal("no diagnostics published for an unknown key on open")
	}

	s.client.Reset()
	s.handleClient(notification(t, "textDocument/didClose", map[string]any{"textDocument": doc}))
	if closed := publishedDiagnostics(t, sent(s.client), uri); closed == nil || len(closed) != 0 {
		t.Errorf("diagnostics on close = %v, want an empty list that clears the old ones", closed)
	}
	if fwd := sent(s.gopls); len(fwd) != 0 {
		t.Errorf("gopls got %d messages about the config file, want none", len(fwd))
	}
}

// publishedDiagnostics returns the diagnostics of the last publishDiagnostics
// for uri in msgs, or nil when there is none.
func publishedDiagnostics(t *testing.T, msgs []*jsonrpc.Message, uri string) []lsp.Diagnostic {
	t.Helper()
	var last []lsp.Diagnostic
	for _, m := range msgs {
		if m.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var p lsp.PublishDiagnosticsParams
		if err := json.Unmarshal(m.Params, &p); err != nil {
			t.Fatalf("decode diagnostics: %v", err)
		}
		if p.URI == uri {
			last = p.Diagnostics
			if last == nil {
				last = []lsp.Diagnostic{}
			}
		}
	}
	return last
}

// openDoc initializes a harness and opens one document in it.
func openDoc(t *testing.T, h *harness, uri, languageID, text string) {
	t.Helper()
	h.send(t, "initialize", "1", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "1")
	h.send(t, "textDocument/didOpen", "", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": languageID, "version": 1, "text": text},
	})
}

func TestConfigCompletionOutsideTheDocumentIsEmpty(t *testing.T) {
	uri := "file:///proj/gooze.yaml"
	tests := map[string]map[string]any{
		"line after the last":   {"textDocument": map[string]any{"uri": uri}, "position": map[string]any{"line": 1, "character": 0}},
		"line far past the end": {"textDocument": map[string]any{"uri": uri}, "position": map[string]any{"line": 9, "character": 0}},
		"negative line":         {"textDocument": map[string]any{"uri": uri}, "position": map[string]any{"line": -1, "character": 0}},
		"malformed position":    {"textDocument": map[string]any{"uri": uri}, "position": "start"},
		"unopened document":     {"textDocument": map[string]any{"uri": "file:///other/gooze.yaml"}, "position": map[string]any{"line": 0, "character": 0}},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, goplsResponder)
			openDoc(t, h, uri, "yaml", "ou")
			h.send(t, "textDocument/completion", "2", params)

			var list struct {
				Items []json.RawMessage `json:"items"`
			}
			if err := json.Unmarshal(h.awaitResponse(t, "2").Result, &list); err != nil {
				t.Fatalf("decode completion: %v", err)
			}
			if len(list.Items) != 0 {
				t.Errorf("got %d completion items, want none", len(list.Items))
			}
		})
	}
}

func TestConfigHover(t *testing.T) {
	uri := "file:///proj/gooze.yaml"
	h := newHarness(t, goplsResponder)
	openDoc(t, h, uri, "yaml", "run:\n  parallel: 2\n")

	h.send(t, "textDocument/hover", "2", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 1, "character": 4},
	})
	var hover lsp.Hover
	if err := json.Unmarshal(h.awaitResponse(t, "2").Result, &hover); err != nil {
		t.Fatalf("decode hover: %v", err)
	}
	if !strings.Contains(hover.Contents.Value, "parallel") {
		t.Errorf("hover on run.parallel = %q, want its docs", hover.Contents.Value)
	}

	h.send(t, "textDocument/hover", "3", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 5, "character": 0},
	})
	if got := string(h.awaitResponse(t, "3").Result); got != "null" {
		t.Errorf("hover past the end = %s, want null", got)
	}
}

func TestGoHoverOverDirective(t *testing.T) {
	uri := "file:///tmp/x.go"
	line := "var v = 1 //gooze:ignore comparison"
	h := newHarness(t, goplsResponder)
	openDoc(t, h, uri, "go", "package x\n\n"+line+"\n")

	h.send(t, "textDocument/hover", "2", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 2, "character": strings.Index(line, "comparison") + 2},
	})
	var hover lsp.Hover
	if err := json.Unmarshal(h.awaitResponse(t, "2").Result, &hover); err != nil {
		t.Fatalf("decode hover: %v", err)
	}
	if hover.Contents.Kind != "markdown" || hover.Contents.Value == "" {
		t.Errorf("hover over the mutagen name = %+v, want gooze's docs", hover)
	}

	// Away from the directive, gopls's hover (null here) is kept.
	h.send(t, "textDocument/hover", "3", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 0, "character": 2},
	})
	if got := string(h.awaitResponse(t, "3").Result); got != "null" {
		t.Errorf("hover on package clause = %s, want gopls's null", got)
	}
}

func TestGoCompletionWithoutDirectiveKeepsGoplsResult(t *testing.T) {
	goplsArray := `[{"label":"fmt"}]`
	h := newHarness(t, func(req *jsonrpc.Message) *jsonrpc.Message {
		if req.Method == "textDocument/completion" {
			return &jsonrpc.Message{Result: json.RawMessage(goplsArray)}
		}
		return goplsResponder(req)
	})
	uri := "file:///tmp/x.go"
	openDoc(t, h, uri, "go", "package x\n\nvar v = fm\n")

	h.send(t, "textDocument/completion", "2", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 2, "character": 10},
	})
	if got := string(h.awaitResponse(t, "2").Result); got != goplsArray {
		t.Errorf("completion = %s, want gopls's result unchanged: %s", got, goplsArray)
	}
}
