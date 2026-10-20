// Package docstore tracks the text of documents the editor has open. It is an
// interface adapter implementing the controller's Documents port.
package docstore

import (
	"sync"

	"gooze.dev/pkg/goozels/pkg/lsp"
)

// Document is the tracked state of an open file.
type Document struct {
	Text    string
	Version int
}

// Store is a concurrency-safe map of open documents. The proxy keeps it in sync
// from didOpen/didChange/didClose so the domain can read current text.
type Store struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

// New creates an empty document store.
func New() *Store { return &Store{docs: map[string]*Document{}} }

// Open records a newly opened document.
func (s *Store) Open(uri, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.docs[uri] = &Document{Text: text, Version: version}
}

// Change applies content changes (full or incremental) to a tracked document.
func (s *Store) Change(uri string, version int, changes []lsp.TextDocumentContentChangeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.docs[uri]
	if !ok {
		doc = &Document{}
		s.docs[uri] = doc
	}

	for _, ch := range changes {
		doc.Text = applyChange(doc.Text, ch)
	}

	doc.Version = version
}

// Close drops a document.
func (s *Store) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.docs, uri)
}

// URIs returns the URIs of all currently-open documents.
func (s *Store) URIs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uris := make([]string, 0, len(s.docs))
	for uri := range s.docs {
		uris = append(uris, uri)
	}

	return uris
}

// Text returns the current text of a document and whether it is tracked.
func (s *Store) Text(uri string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.docs[uri]
	if !ok {
		return "", false
	}

	return doc.Text, true
}

func applyChange(text string, ch lsp.TextDocumentContentChangeEvent) string {
	if ch.Range == nil {
		return ch.Text
	}

	start := lsp.PositionToOffset(text, ch.Range.Start)

	end := lsp.PositionToOffset(text, ch.Range.End)
	if start > end {
		start, end = end, start
	}

	return text[:start] + ch.Text + text[end:]
}
