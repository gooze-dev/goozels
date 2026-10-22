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

// The values are fixed by the LSP 3.17 specification and go over the wire, so
// a changed constant would silently break editors.
func TestProtocolConstantsMatchSpec(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"MessageType.Error", MessageError, 1},
		{"MessageType.Warning", MessageWarn, 2},
		{"MessageType.Info", MessageInfo, 3},
		{"MessageType.Log", MessageLog, 4},
		{"CompletionItemKind.Property", CompletionKindProperty, 10},
		{"CompletionItemKind.Value", CompletionKindValue, 12},
		{"CompletionItemKind.EnumMember", CompletionKindEnumMember, 20},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestURIPathSchemes(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		// Percent-encoding is decoded only for parsed file URIs.
		{"file:///home/x/my%20file.go", "/home/x/my file.go"},
		// Other schemes, sorting before and after "file", are returned as is.
		{"data:text/plain,x", "data:text/plain,x"},
		{"untitled:Untitled-1", "untitled:Untitled-1"},
		// An unparsable URI falls back to trimming the file:// prefix.
		{"file://%zz/x.go", "%zz/x.go"},
	}
	for _, tt := range tests {
		if got := URIPath(tt.uri); got != tt.want {
			t.Errorf("URIPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestUTF16ByteConversionEdges(t *testing.T) {
	tests := []struct {
		name string
		line string
		u16  int
		byte int
	}{
		{"start of line", "hello", 0, 0},
		{"end of line", "hello", 5, 5},
		{"empty line", "", 0, 0},
		// U+FFFF is the largest one-unit character in UTF-16.
		{"after U+FFFF", "￿x", 1, 3},
		{"end after U+FFFF", "￿x", 2, 4},
		{"two surrogate pairs", "😀😀x", 4, 8},
		{"end after surrogate pairs", "😀😀x", 5, 9},
	}
	for _, tt := range tests {
		if got := UTF16ToByte(tt.line, tt.u16); got != tt.byte {
			t.Errorf("%s: UTF16ToByte(%q,%d) = %d, want %d", tt.name, tt.line, tt.u16, got, tt.byte)
		}
		if got := ByteToUTF16(tt.line, tt.byte); got != tt.u16 {
			t.Errorf("%s: ByteToUTF16(%q,%d) = %d, want %d", tt.name, tt.line, tt.byte, got, tt.u16)
		}
	}
}

func TestUTF16ToByteInsideSurrogatePair(t *testing.T) {
	// Offset 1 points inside the emoji's surrogate pair; it resolves to the
	// next character.
	if got := UTF16ToByte("😀x", 1); got != 4 {
		t.Errorf("UTF16ToByte inside a surrogate pair = %d, want 4", got)
	}
	if got := UTF16ToByte("hello", -3); got != 0 {
		t.Errorf("UTF16ToByte with a negative offset = %d, want 0", got)
	}
}

func TestByteToUTF16InsideRuneAndPastEnd(t *testing.T) {
	// Byte 2 is inside é (bytes 1-2): only the runes starting before it count.
	if got := ByteToUTF16("héllo", 2); got != 2 {
		t.Errorf("ByteToUTF16 inside a rune = %d, want 2", got)
	}
	if got := ByteToUTF16("héllo", 99); got != 5 {
		t.Errorf("ByteToUTF16 past the end = %d, want 5", got)
	}
}

func TestPositionToOffsetEdges(t *testing.T) {
	tests := []struct {
		name string
		text string
		pos  Position
		want int
	}{
		{"start of text", "ab\ncd", Position{Line: 0, Character: 0}, 0},
		{"end of first line", "ab\ncd", Position{Line: 0, Character: 2}, 2},
		{"character past line end stops at the newline", "ab\ncd", Position{Line: 0, Character: 9}, 2},
		{"empty line", "ab\n\ncd", Position{Line: 1, Character: 3}, 3},
		{"tab is not a newline", "\tx\ny", Position{Line: 1, Character: 0}, 3},
		{"tab inside the line", "a\tb", Position{Line: 0, Character: 3}, 3},
		{"multi-byte character", "ab\nhéllo", Position{Line: 1, Character: 2}, 6},
		{"negative line is the first line", "ab\ncd", Position{Line: -1, Character: 1}, 1},
	}
	for _, tt := range tests {
		if got := PositionToOffset(tt.text, tt.pos); got != tt.want {
			t.Errorf("%s: PositionToOffset(%q, %+v) = %d, want %d", tt.name, tt.text, tt.pos, got, tt.want)
		}
	}
}
