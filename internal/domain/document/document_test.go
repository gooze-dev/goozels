package document

import (
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: []string{""}},
		{name: "single no newline", in: "abc", want: []string{"abc"}},
		{name: "trailing newline", in: "a\nb\n", want: []string{"a", "b", ""}},
		{name: "no trailing", in: "a\nb", want: []string{"a", "b"}},
		{name: "crlf", in: "a\r\nb\r\n", want: []string{"a", "b", ""}},
		{name: "empty line in the middle", in: "a\n\nb", want: []string{"a", "", "b"}},
		{name: "crlf only line", in: "\r\nb", want: []string{"", "b"}},
		{name: "last line ends with cr", in: "a\nbc\r", want: []string{"a", "bc"}},
		{name: "last line is only cr", in: "a\n\r", want: []string{"a", ""}},
		// Tabs sort below '\n' and '\r', so they catch <= and < in the checks.
		{name: "tab inside a line", in: "a\tb\nc", want: []string{"a\tb", "c"}},
		{name: "line ends with tab", in: "a\t\nb", want: []string{"a\t", "b"}},
		{name: "last line ends with tab", in: "a\nb\t", want: []string{"a", "b\t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines(tt.in)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("Lines(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
