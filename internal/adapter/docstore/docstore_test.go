package docstore

import (
	"testing"

	"gooze.dev/pkg/goozels/pkg/lsp"
)

func TestStoreOpenTextClose(t *testing.T) {
	s := New()
	if _, ok := s.Text("file:///x"); ok {
		t.Error("unopened doc should not be present")
	}
	s.Open("file:///x", "hello", 1)
	if text, ok := s.Text("file:///x"); !ok || text != "hello" {
		t.Errorf("Text after Open = %q, %v", text, ok)
	}
	s.Close("file:///x")
	if _, ok := s.Text("file:///x"); ok {
		t.Error("doc should be gone after Close")
	}
}

func TestStoreChangeFull(t *testing.T) {
	s := New()
	s.Open("file:///x", "old", 1)
	s.Change("file:///x", 2, []lsp.TextDocumentContentChangeEvent{{Text: "new"}})
	if text, _ := s.Text("file:///x"); text != "new" {
		t.Errorf("full change result = %q", text)
	}
}

func TestStoreChangeIncremental(t *testing.T) {
	s := New()
	s.Open("file:///x", "hello\nworld", 1)
	// Replace "hello" (line 0, chars 0..5) with "HI".
	s.Change("file:///x", 2, []lsp.TextDocumentContentChangeEvent{{
		Range: &lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 5}},
		Text:  "HI",
	}})
	if text, _ := s.Text("file:///x"); text != "HI\nworld" {
		t.Errorf("incremental change result = %q, want %q", text, "HI\nworld")
	}
}
