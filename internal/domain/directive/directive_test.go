package directive

import "testing"

func TestParseLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		want      bool
		names     []string
		ignoreAll bool
	}{
		{name: "no directive", line: "x := 1 + 2", want: false},
		{name: "bare ignore all", line: "//gooze:ignore", want: true, ignoreAll: true},
		{name: "spaced marker", line: "//  gooze:ignore", want: true, ignoreAll: true},
		{name: "single name", line: "//gooze:ignore arithmetic", want: true, names: []string{"arithmetic"}},
		{name: "multiple names", line: "//gooze:ignore arithmetic,comparison", want: true, names: []string{"arithmetic", "comparison"}},
		{name: "spaced names", line: "//gooze:ignore arithmetic, comparison", want: true, names: []string{"arithmetic", "comparison"}},
		{name: "trailing comment", line: "x := 1 + 2 //gooze:ignore numbers", want: true, names: []string{"numbers"}},
		{name: "block comment", line: "/*gooze:ignore branch*/", want: true, names: []string{"branch"}},
		{name: "not a comment", line: "gooze:ignore arithmetic", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := ParseLine(tt.line)
			if ok != tt.want {
				t.Fatalf("ParseLine ok = %v, want %v", ok, tt.want)
			}
			if !ok {
				return
			}
			if d.IgnoreAll != tt.ignoreAll {
				t.Errorf("IgnoreAll = %v, want %v", d.IgnoreAll, tt.ignoreAll)
			}
			var got []string
			for _, n := range d.Names {
				got = append(got, n.Text)
			}
			if len(got) != len(tt.names) {
				t.Fatalf("names = %v, want %v", got, tt.names)
			}
			for i := range got {
				if got[i] != tt.names[i] {
					t.Errorf("name[%d] = %q, want %q", i, got[i], tt.names[i])
				}
			}
		})
	}
}

func TestParseLineSpans(t *testing.T) {
	line := "//gooze:ignore arithmetic,comparison"
	d, ok := ParseLine(line)
	if !ok {
		t.Fatal("expected directive")
	}
	for _, n := range d.Names {
		if line[n.Start:n.End] != n.Text {
			t.Errorf("span %d:%d = %q, want %q", n.Start, n.End, line[n.Start:n.End], n.Text)
		}
	}
}

func TestCursorAt(t *testing.T) {
	line := "//gooze:ignore arith,comparison"
	// position the cursor at the end of "arith"
	col := len("//gooze:ignore arith")
	c, ok := CursorAt(line, col)
	if !ok || !c.InList {
		t.Fatalf("expected in-list cursor, got ok=%v inList=%v", ok, c.InList)
	}
	if c.Prefix != "arith" {
		t.Errorf("prefix = %q, want %q", c.Prefix, "arith")
	}
	if line[c.TokenStart:c.TokenEnd] != "arith" {
		t.Errorf("token = %q, want %q", line[c.TokenStart:c.TokenEnd], "arith")
	}

	// cursor before the keyword is not in the list
	if _, ok := CursorAt(line, 2); ok {
		t.Error("cursor in keyword should not be in list")
	}

	// cursor right after "gooze:ignore " (empty token) should be in the list
	col = len("//gooze:ignore ")
	c, ok = CursorAt(line, col)
	if !ok || !c.InList || c.Prefix != "" {
		t.Errorf("empty-token cursor: ok=%v inList=%v prefix=%q", ok, c.InList, c.Prefix)
	}
}
