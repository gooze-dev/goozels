package controller

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
