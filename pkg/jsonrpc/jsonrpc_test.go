package jsonrpc

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	out := &Message{ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{"x":1}`)}
	if err := w.Write(out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "Content-Length: ") {
		t.Errorf("missing Content-Length framing: %q", buf.String())
	}

	r := NewReader(bytes.NewReader(buf.Bytes()))
	got, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Method != "initialize" || got.IDKey() != "1" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.JSONRPC != "2.0" {
		t.Errorf("jsonrpc version not defaulted: %q", got.JSONRPC)
	}
}

func TestMessageClassification(t *testing.T) {
	req := &Message{ID: json.RawMessage("1"), Method: "m"}
	note := &Message{Method: "m"}
	resp := &Message{ID: json.RawMessage("1")}

	if !req.IsRequest() || req.IsNotification() || req.IsResponse() {
		t.Error("request misclassified")
	}
	if note.IsRequest() || !note.IsNotification() || note.IsResponse() {
		t.Error("notification misclassified")
	}
	if resp.IsRequest() || resp.IsNotification() || !resp.IsResponse() {
		t.Error("response misclassified")
	}
	if (&Message{}).IDKey() != "" {
		t.Error("empty id should yield empty key")
	}
}

func TestReadMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.Write(&Message{ID: json.RawMessage("1"), Method: "a"})
	_ = w.Write(&Message{ID: json.RawMessage("2"), Method: "b"})

	r := NewReader(bytes.NewReader(buf.Bytes()))
	first, err := r.Read()
	if err != nil || first.Method != "a" {
		t.Fatalf("first message: %+v, %v", first, err)
	}
	second, err := r.Read()
	if err != nil || second.Method != "b" {
		t.Fatalf("second message: %+v, %v", second, err)
	}
}

func TestReadMissingContentLength(t *testing.T) {
	r := NewReader(strings.NewReader("\r\n{}"))
	if _, err := r.Read(); err == nil {
		t.Error("expected error for missing Content-Length")
	}
}
