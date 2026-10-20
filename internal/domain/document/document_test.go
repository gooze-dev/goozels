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
