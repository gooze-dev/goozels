package docstore

import (
	"slices"
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

func TestStoreURIs(t *testing.T) {
	s := New()
	if uris := s.URIs(); len(uris) != 0 {
		t.Errorf("URIs of an empty store = %q, want none", uris)
	}

	s.Open("file:///b", "", 1)
	s.Open("file:///a", "", 1)
	uris := s.URIs()
	slices.Sort(uris)
	if !slices.Equal(uris, []string{"file:///a", "file:///b"}) {
		t.Errorf("URIs after two Opens = %q", uris)
	}

	s.Close("file:///b")
	if uris := s.URIs(); !slices.Equal(uris, []string{"file:///a"}) {
		t.Errorf("URIs after Close = %q", uris)
	}
}

func TestStoreChangeRangeEdges(t *testing.T) {
	tests := []struct {
		name       string
		start, end lsp.Position
		text, want string
	}{
		{"reversed range", lsp.Position{Line: 0, Character: 5}, lsp.Position{Line: 0, Character: 0}, "HI", "HI\nworld"},
		{"insert at a point", lsp.Position{Line: 1, Character: 0}, lsp.Position{Line: 1, Character: 0}, "X", "hello\nXworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			s.Open("file:///x", "hello\nworld", 1)
			s.Change("file:///x", 2, []lsp.TextDocumentContentChangeEvent{{
				Range: &lsp.Range{Start: tt.start, End: tt.end},
				Text:  tt.text,
			}})
			if text, _ := s.Text("file:///x"); text != tt.want {
				t.Errorf("change result = %q, want %q", text, tt.want)
			}
		})
	}
}
