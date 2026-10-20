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

func TestPackagePattern(t *testing.T) {
	for _, c := range []struct{ workdir, file, want string }{
		{"/w", "/w/internal/domain/ignore.go", "./internal/domain"},
		{"/w", "/w/main.go", "."},
		{"/w", "/elsewhere/x.go", "./..."},
		{"/w", "/w/../x.go", "./..."},
	} {
		if got := PackagePattern(c.workdir, c.file); got != c.want {
			t.Errorf("PackagePattern(%q, %q) = %q, want %q", c.workdir, c.file, got, c.want)
		}
	}
	if got := RunCommandLine("./calc"); got != "gooze run ./calc" {
		t.Errorf("RunCommandLine = %q", got)
	}
}

func TestLensTitlesArePlain(t *testing.T) {
	for _, l := range GoFileLenses(".") {
		if !strings.HasPrefix(l.Command.Title, "gooze: ") {
			t.Errorf("lens title %q should be plain text starting with \"gooze: \"", l.Command.Title)
		}
	}
}

func TestPackagePatternOddNames(t *testing.T) {
	// "-x" and "+x" sort below ".", "~x" above it.
	for _, dir := range []string{"-x", "+x", "~x"} {
		if got := PackagePattern("/w", "/w/"+dir+"/a.go"); got != "./"+dir {
			t.Errorf("PackagePattern for %s = %q, want ./%s", dir, got, dir)
		}
	}
}

func TestGoFileLensesAtTopOfFile(t *testing.T) {
	for _, l := range GoFileLenses(".") {
		if l.Range.Start.Line != 0 || l.Range.Start.Column != 0 || l.Range.End.Line != 0 || l.Range.End.Column != 0 {
			t.Errorf("lens %q range = %+v, want the start of the file", l.Command.Title, l.Range)
		}
	}
}
