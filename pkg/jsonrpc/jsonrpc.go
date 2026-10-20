// Package jsonrpc implements the LSP base protocol: JSON-RPC 2.0 messages framed
// with Content-Length headers. It deliberately keeps params/result/error as raw
// JSON so a proxy can forward messages it does not need to understand without
// loss of fidelity.
package jsonrpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Version is the JSON-RPC protocol version every message carries.
const Version = "2.0"

// Message is a JSON-RPC 2.0 message (request, response, or notification). Unused
// payload fields stay as raw JSON for transparent forwarding.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// IsRequest reports whether the message is a request (has both id and method).
func (m *Message) IsRequest() bool { return len(m.ID) > 0 && m.Method != "" }

// IsNotification reports whether the message is a notification (method, no id).
func (m *Message) IsNotification() bool { return len(m.ID) == 0 && m.Method != "" }

// IsResponse reports whether the message is a response (id, no method).
func (m *Message) IsResponse() bool { return len(m.ID) > 0 && m.Method == "" }

// IDKey returns a stable string key for the message id, suitable for map use.
// It returns "" when there is no id.
func (m *Message) IDKey() string {
	if len(m.ID) == 0 {
		return ""
	}

	return string(m.ID)
}

// Reader reads framed messages from a stream.
type Reader struct {
	r *bufio.Reader
}

// NewReader wraps r in a framed-message reader.
func NewReader(r io.Reader) *Reader { return &Reader{r: bufio.NewReader(r)} }

// Read returns the next message, or io.EOF when the stream ends cleanly.
func (rd *Reader) Read() (*Message, error) {
	length := -1

	for {
		line, err := rd.r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break // end of headers
		}

		name, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", value, err)
			}

			length = n
		}
	}

	if length < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(rd.r, body); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}

	return &msg, nil
}

// Writer writes framed messages to a stream. It is safe for concurrent use.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter wraps w in a framed-message writer.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write frames and writes a message.
func (wr *Writer) Write(msg *Message) error {
	if msg.JSONRPC == "" {
		msg.JSONRPC = Version
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return wr.WriteRaw(body)
}

// WriteRaw frames and writes an already-encoded JSON body.
func (wr *Writer) WriteRaw(body []byte) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	if _, err := fmt.Fprintf(wr.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}

	_, err := wr.w.Write(body)

	return err
}
