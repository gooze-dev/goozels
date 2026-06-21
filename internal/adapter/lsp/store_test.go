package lsp

import "testing"

func TestStoreOpenTextClose(t *testing.T) {
	s := NewStore()
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
	s := NewStore()
	s.Open("file:///x", "old", 1)
	s.Change("file:///x", 2, []TextDocumentContentChangeEvent{{Text: "new"}})
	if text, _ := s.Text("file:///x"); text != "new" {
		t.Errorf("full change result = %q", text)
	}
}

func TestStoreChangeIncremental(t *testing.T) {
	s := NewStore()
	s.Open("file:///x", "hello\nworld", 1)
	// Replace "hello" (line 0, chars 0..5) with "HI".
	s.Change("file:///x", 2, []TextDocumentContentChangeEvent{{
		Range: &Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}},
		Text:  "HI",
	}})
	if text, _ := s.Text("file:///x"); text != "HI\nworld" {
		t.Errorf("incremental change result = %q, want %q", text, "HI\nworld")
	}
}

func TestPositionToOffset(t *testing.T) {
	text := "ab\ncd"
	if off := positionToOffset(text, Position{Line: 1, Character: 1}); off != 4 {
		t.Errorf("positionToOffset = %d, want 4", off)
	}
	// Past end clamps to len.
	if off := positionToOffset(text, Position{Line: 9, Character: 9}); off != len(text) {
		t.Errorf("out-of-range offset = %d, want %d", off, len(text))
	}
}
