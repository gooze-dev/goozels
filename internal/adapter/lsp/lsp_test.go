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
	for _, uri := range []string{"file:///p/gooze.yaml", "file:///p/.gooze.yml"} {
		if !IsConfigURI(uri) {
			t.Errorf("%s should be a config URI", uri)
		}
	}
	if IsConfigURI("file:///p/config.yaml") {
		t.Error("config.yaml is not a gooze config URI")
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
		if got := utf16ToByte(tt.line, tt.u16); got != tt.byte {
			t.Errorf("utf16ToByte(%q,%d) = %d, want %d", tt.line, tt.u16, got, tt.byte)
		}
		if got := byteToUTF16(tt.line, tt.byte); got != tt.u16 {
			t.Errorf("byteToUTF16(%q,%d) = %d, want %d", tt.line, tt.byte, got, tt.u16)
		}
	}
	if ByteColumn("héllo", 2) != 3 {
		t.Error("ByteColumn should match utf16ToByte")
	}
}
