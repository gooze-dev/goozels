package lsp

import "testing"

func TestURIHelpers(t *testing.T) {
	if got := URIPath("file:///home/x/main.go"); got != "/home/x/main.go" {
		t.Errorf("URIPath = %q", got)
	}
	if !IsGoURI("file:///x/main.go") {
		t.Error("main.go should be a Go URI")
	}
	if IsGoURI("file:///x/gooze.yaml") {
		t.Error("yaml is not a Go URI")
	}
}

func TestUTF16ByteConversion(t *testing.T) {
	tests := []struct {
		line string
		u16  int
		byte int
	}{
		{"hello", 3, 3},
		{"héllo", 2, 3}, // é is 2 bytes / 1 UTF-16 unit
		{"😀x", 2, 4},    // emoji is 4 bytes / 2 UTF-16 units
	}
	for _, tt := range tests {
		if got := UTF16ToByte(tt.line, tt.u16); got != tt.byte {
			t.Errorf("UTF16ToByte(%q,%d) = %d, want %d", tt.line, tt.u16, got, tt.byte)
		}
		if got := ByteToUTF16(tt.line, tt.byte); got != tt.u16 {
			t.Errorf("ByteToUTF16(%q,%d) = %d, want %d", tt.line, tt.byte, got, tt.u16)
		}
	}
}

func TestPositionToOffset(t *testing.T) {
	text := "ab\ncd"
	if off := PositionToOffset(text, Position{Line: 1, Character: 1}); off != 4 {
		t.Errorf("PositionToOffset = %d, want 4", off)
	}
	// Past end clamps to len.
	if off := PositionToOffset(text, Position{Line: 9, Character: 9}); off != len(text) {
		t.Errorf("out-of-range offset = %d, want %d", off, len(text))
	}
}
