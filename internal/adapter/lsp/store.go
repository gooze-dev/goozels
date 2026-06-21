package lsp

import "sync"

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

// NewStore creates an empty document store.
func NewStore() *Store { return &Store{docs: map[string]*Document{}} }

// Open records a newly opened document.
func (s *Store) Open(uri, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.docs[uri] = &Document{Text: text, Version: version}
}

// Change applies content changes (full or incremental) to a tracked document.
func (s *Store) Change(uri string, version int, changes []TextDocumentContentChangeEvent) {
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

func applyChange(text string, ch TextDocumentContentChangeEvent) string {
	if ch.Range == nil {
		return ch.Text
	}

	start := positionToOffset(text, ch.Range.Start)

	end := positionToOffset(text, ch.Range.End)
	if start > end {
		start, end = end, start
	}

	return text[:start] + ch.Text + text[end:]
}

// positionToOffset converts an LSP position to a byte offset within text.
func positionToOffset(text string, pos Position) int {
	line, i := 0, 0
	for line < pos.Line && i < len(text) {
		if text[i] == '\n' {
			line++
		}

		i++
	}

	end := i
	for end < len(text) && text[end] != '\n' {
		end++
	}

	off := i + utf16ToByte(text[i:end], pos.Character)
	if off > len(text) {
		off = len(text)
	}

	return off
}
