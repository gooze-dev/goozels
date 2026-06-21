package command

import "testing"

func TestSubcommand(t *testing.T) {
	if sub, ok := Subcommand(Run); !ok || sub != "run" {
		t.Errorf("Subcommand(Run) = %q, %v", sub, ok)
	}
	if sub, ok := Subcommand(List); !ok || sub != "list" {
		t.Errorf("Subcommand(List) = %q, %v", sub, ok)
	}
	if sub, ok := Subcommand(View); !ok || sub != "view" {
		t.Errorf("Subcommand(View) = %q, %v", sub, ok)
	}
	if _, ok := Subcommand("gooze.bogus"); ok {
		t.Error("unknown command resolved")
	}
}

func TestIsGoozeAndNames(t *testing.T) {
	if !IsGooze(Run) || !IsGooze(List) || !IsGooze(View) {
		t.Error("Run/List/View should be gooze commands")
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
