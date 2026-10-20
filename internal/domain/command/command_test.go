package command

import (
	"strings"
	"testing"
)

func TestArgs(t *testing.T) {
	cases := []struct {
		id, path string
		want     string
	}{
		{Run, "./pkg/...", "run ./pkg/..."},
		{Run, "", "run ./..."},
		{Estimate, "./pkg/...", "run --estimate ./pkg/..."},
		{"gooze.list", "", "run --estimate ./..."}, // legacy identifier for the removed `gooze list`
		{View, "./ignored", "report view"},
	}
	for _, c := range cases {
		args, ok := Args(c.id, c.path)
		if got := strings.Join(args, " "); !ok || got != c.want {
			t.Errorf("Args(%q, %q) = %q, %v; want %q", c.id, c.path, got, ok, c.want)
		}
	}
	if _, ok := Args("gooze.bogus", ""); ok {
		t.Error("unknown command resolved")
	}
}

func TestIsGoozeAndNames(t *testing.T) {
	if !IsGooze(Run) || !IsGooze(Estimate) || !IsGooze(View) || !IsGooze("gooze.list") {
		t.Error("Run/Estimate/View and the legacy gooze.list should be gooze commands")
	}
	if IsGooze("gopls.test") {
		t.Error("gopls command misclassified as gooze")
	}
	if got := Names(); len(got) != 3 {
		t.Errorf("Names len = %d, want 3", len(got))
	}
}

func TestGoFileLenses(t *testing.T) {
	lenses := GoFileLenses("./...")
	if len(lenses) != 3 {
		t.Fatalf("expected 3 lenses, got %d", len(lenses))
	}
	var hasView bool
	for _, l := range lenses {
		if l.Command.Command == View {
			hasView = true
		}
	}
	if !hasView {
		t.Error("expected a gooze.view (report) lens")
	}
	var runCmd bool
	for _, l := range lenses {
		if l.Command.Command == Run {
			runCmd = true
			if len(l.Command.Arguments) != 1 || l.Command.Arguments[0] != "./..." {
				t.Errorf("run lens args = %v", l.Command.Arguments)
			}
		}
		if l.Command.Title == "" {
			t.Error("lens has empty title")
		}
	}
	if !runCmd {
		t.Error("no gooze.run lens produced")
	}
}
