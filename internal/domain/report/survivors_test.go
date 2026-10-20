package report

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func keepAll(rpt.Report, change) bool { return true }

// realDiff is a unified diff as gooze stores it for a mutant: three lines of
// context, so the hunk starts three lines before the mutated line. gooze writes
// blank context lines as " "; editors strip that trailing space here, and
// diffChanges treats both forms alike.
const realDiff = `--- original
+++ mutated
@@ -205,7 +205,7 @@
 }

 func isLeadingComment(line int, slashOffset int, lineStarts []int, content []byte) bool {
-	if line <= 0 || line > len(lineStarts) {
+	if line == 0 || line > len(lineStarts) {
 		return false
 	}

`

func withName(r rpt.Report, name string) rpt.Report {
	r.Result[0].Name = name
	return r
}

func TestSurvivorDiagnosticsMarksMutatedLine(t *testing.T) {
	reports := []rpt.Report{
		withName(newReport("x.go", realDiff, rpt.Survived), "comparison"),
		withName(newReport("x.go", realDiff, rpt.Killed), "comparison"),
	}

	diags := survivorDiagnostics(reports, keepAll)
	if len(diags) != 1 {
		t.Fatalf("expected 1 survivor diagnostic (killed mutants are not marked), got %d", len(diags))
	}

	d := diags[0]
	if d.Range.Start.Line != 207 { // the "-" line is 1-based 208, not the hunk start 205
		t.Errorf("line = %d, want 207", d.Range.Start.Line)
	}
	if d.Range.Start.Column != 1 || d.Range.End.Column != len("\tif line <= 0 || line > len(lineStarts) {") {
		t.Errorf("columns = %d..%d, want the code after the tab", d.Range.Start.Column, d.Range.End.Column)
	}
	want := "gooze: surviving comparison mutant — tests still pass with `if line <= 0 || line > len(lineStarts) {` → `if line == 0 || line > len(lineStarts) {`"
	if d.Message != want {
		t.Errorf("message = %q\nwant      %q", d.Message, want)
	}
}

func TestSurvivorDiagnosticsNotCoveredAndRemoval(t *testing.T) {
	removal := "--- original\n+++ mutated\n@@ -3,3 +3,2 @@\n a()\n-\tb()\n c()\n"
	reports := []rpt.Report{
		withName(newReport("x.go", removal, rpt.NotCovered), "statement"),
		newReport("x.go", hunk, rpt.Timeout),
	}

	diags := survivorDiagnostics(reports, keepAll)
	if len(diags) != 1 || diags[0].Range.Start.Line != 3 {
		t.Fatalf("diags = %+v, want one marker on 0-based line 3 (timeouts are not marked)", diags)
	}
	if !strings.Contains(diags[0].Message, "not covered by any test") || !strings.Contains(diags[0].Message, "`b()` → (line removed)") {
		t.Errorf("message = %q", diags[0].Message)
	}

	clean := []rpt.Report{newReport("x.go", hunk, rpt.Killed)}
	if got := survivorDiagnostics(clean, keepAll); len(got) != 0 {
		t.Errorf("expected no markers for a clean report, got %v", got)
	}
}

func TestDiffChangesMultipleHunksAndBlankContext(t *testing.T) {
	diff := "--- original\n+++ mutated\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n\n@@ -10,2 +10,2 @@\n-x\n+X\n y\n\\ No newline at end of file\n"
	got := diffChanges(diff)
	if len(got) != 2 || got[0].line != 1 || got[0].mutated != "B" || got[1].line != 9 || got[1].mutated != "X" {
		t.Errorf("changes = %+v", got)
	}
	if diffChanges("") != nil || diffChanges("@@ garbage @@\n-x\n") != nil {
		t.Error("empty or malformed diffs should yield no changes")
	}
}

// lineDiff mutates line 108 of a file from the gooze repository.
func lineDiff(mutated string) string {
	return "--- original\n+++ mutated\n@@ -105,7 +105,7 @@\n a\n b\n c\n-\t\tif rule.all || len(rule.names) > 0 {\n+\t\t" + mutated + "\n d\n"
}

func TestSurvivorDiagnosticsGroupsLineInOrder(t *testing.T) {
	reports := []rpt.Report{
		withName(newReport("x.go", lineDiff("if rule.all || len(rule.names) < 0 {"), rpt.Survived), "comparison"),
		withName(newReport("x.go", hunk, rpt.Survived), "arithmetic"),
		withName(newReport("x.go", lineDiff("if rule.all && len(rule.names) > 0 {"), rpt.Survived), "logical"),
		withName(newReport("x.go", lineDiff("if rule.all || len(rule.names) == 0 {"), rpt.Survived), "comparison"),
		withName(newReport("x.go", lineDiff("if rule.all || len(rule.names) < 0 {"), rpt.Survived), "comparison"), // duplicate
	}

	diags := survivorDiagnostics(reports, keepAll)
	if len(diags) != 2 || diags[0].Range.Start.Line != 9 || diags[1].Range.Start.Line != 107 {
		t.Fatalf("diags = %+v, want line 9 then the grouped line 107", diags)
	}
	want := "gooze: 3 surviving mutants (comparison ×2, logical) — tests still pass with " +
		"`if rule.all || len(rule.names) > 0 {` → `… < 0 {`, `… == 0 {`, `… && len(rule.names) > 0 {`"
	if diags[1].Message != want {
		t.Errorf("grouped message = %q\nwant             %q", diags[1].Message, want)
	}
}

func TestGroupHeadNotCovered(t *testing.T) {
	ms := []mutant{{mutagen: "boolean", status: rpt.NotCovered}, {mutagen: "boolean", status: rpt.NotCovered}}
	if head, verb := groupHead(ms); head != "2 mutants not covered by any test (boolean ×2)" || verb != "" {
		t.Errorf("groupHead = %q, %q", head, verb)
	}
	ms = append(ms, mutant{mutagen: "", status: rpt.Survived})
	if head, _ := groupHead(ms); head != "3 surviving mutants (boolean ×2, unknown), 2 of them not covered by any test" {
		t.Errorf("mixed groupHead = %q", head)
	}
}

func TestVariant(t *testing.T) {
	for _, c := range []struct {
		orig, mutated string
		elide         bool
		want          string
	}{
		{"return n > 0", "return n >= 0", true, "`return n >= 0`"}, // shared prefix too short to elide
		{"if alpha.beta.gamma > 0 {", "if alpha.beta.gamma >= 0 {", true, "`… >= 0 {`"},
		{"if alpha.beta.gamma > 0 {", "if alpha.beta.gamma >= 0 {", false, "`if alpha.beta.gamma >= 0 {`"},
		{"if a > 0 && alpha.beta.gamma {", "if!a", true, "`if!a`"}, // nothing shared up to a space
		{"x()", "", true, "(line removed)"},
		{"if rule.all || len(rule.names) > 0 {", "if rule.all || len(rule.names) > 1 {", true, "`… > 1 {`"}, // a changed literal keeps its operator
	} {
		if got := variant(c.orig, c.mutated, c.elide); got != c.want {
			t.Errorf("variant(%q, %q, %v) = %q, want %q", c.orig, c.mutated, c.elide, got, c.want)
		}
	}
	if !slices.Contains([]string{IgnoreHint}, IgnoreHint) || !strings.Contains(IgnoreHint, "//gooze:ignore <mutagen>") {
		t.Error("IgnoreHint should name the directive")
	}
}

// gooze rewrites the end of a file when it mutates its last lines, so the
// diff also removes and re-adds lines that did not change.
const tailDiff = "--- original\n+++ mutated\n@@ -233,6 +233,6 @@\n \n var topOfFile = model.Range{\n \tStart: model.Position{Line: 0, Column: 0},\n-\tEnd:   model.Position{Line: 0, Column: 0},\n-}\n-\n+\tEnd:   model.Position{Line: 0, Column: 1},\n+}\n+\n"

func TestDiffChangesSkipsUnchangedLines(t *testing.T) {
	got := diffChanges(tailDiff)
	if len(got) != 1 || got[0].line != 235 || got[0].mutated != "End:   model.Position{Line: 0, Column: 1}," {
		t.Errorf("changes = %+v, want only the End line", got)
	}
}

func TestLineMessageListsAFewVariants(t *testing.T) {
	var ms []mutant
	for i := range 9 {
		c := change{text: "total := compute(a, b) + 10", mutated: "total := compute(a, b) + " + string(rune('0'+i))}
		ms = append(ms, mutant{change: c, mutagen: "numbers", status: rpt.Survived})
	}
	got := lineMessage(ms)
	want := "gooze: 9 surviving mutants (numbers ×9) — tests still pass with `total := compute(a, b) + 10` → " +
		"`… + 0`, `… + 1`, `… + 2`, `… + 3`, `… + 4` and 4 more"
	if got != want {
		t.Errorf("lineMessage =\n%q\nwant\n%q", got, want)
	}
}

func TestVariantElisionBoundaries(t *testing.T) {
	for _, c := range []struct{ orig, mutated, want string }{
		// The shared prefix is a short "ab ": too short to elide.
		{"ab < cccccccccccccccccccccccc", "ab > cccccccccccccccccccccccc", "`ab > cccccccccccccccccccccccc`"},
		// A shared prefix of exactly minElided bytes is elided.
		{"abcdefg < ccccccccccccccccccc", "abcdefg > ccccccccccccccccccc", "`… > ccccccccccccccccccc`"},
		// A line of exactly shortLine bytes is shown whole.
		{"if alpha.beta.gamm > 0 {", "if alpha.beta.gamm < 0 {", "`if alpha.beta.gamm < 0 {`"},
		// The mutated line extends the original.
		{"if alpha.beta.gamma > 0 {", "if alpha.beta.gamma > 0 { x", "`… { x`"},
	} {
		if got := variant(c.orig, c.mutated, true); got != c.want {
			t.Errorf("variant(%q, %q) = %q, want %q", c.orig, c.mutated, got, c.want)
		}
	}
}

func TestIsWordByte(t *testing.T) {
	for _, c := range "_09azAZ" {
		if !isWordByte(byte(c)) {
			t.Errorf("%q should be a word byte", c)
		}
	}
	for _, c := range "/:`{@[^ ~<>" {
		if isWordByte(byte(c)) {
			t.Errorf("%q should not be a word byte", c)
		}
	}
}

func TestDiffChangesEdges(t *testing.T) {
	if got := diffChanges("@@ -1,1 +1,2 @@\n+added\n a\n"); len(got) != 0 {
		t.Errorf("a pure addition removes no line, got %+v", got)
	}
	got := diffChanges("@@ -1,1 +1,1 @@\n-a\n+b\n")
	if len(got) != 1 || got[0].line != 0 {
		t.Errorf("a change on the first line = %+v, want line 0", got)
	}
	if c := removedLine(0, "\t\tx"); c.indent != 2 {
		t.Errorf("indent = %d, want 2", c.indent)
	}
	for _, h := range []string{"@@ -0,0 +1,1 @@", "@@ -abc +1 @@", "@@ garbage @@"} {
		if got := hunkOldStart(h); got != -1 {
			t.Errorf("hunkOldStart(%q) = %d, want -1", h, got)
		}
	}
}

func TestLineMessageCountsTheRest(t *testing.T) {
	variants := func(n int) []mutant {
		var ms []mutant
		for i := range n {
			c := change{text: "total := compute(a, b) + 99", mutated: "total := compute(a, b) + " + strconv.Itoa(i)}
			ms = append(ms, mutant{change: c, mutagen: "numbers", status: rpt.Survived})
		}
		return ms
	}
	for n, suffix := range map[int]string{5: "`… + 4`", 6: "`… + 4` and 1 more", 12: "`… + 4` and 7 more"} {
		if got := lineMessage(variants(n)); !strings.HasSuffix(got, suffix) {
			t.Errorf("%d variants: %q, want it to end with %q", n, got, suffix)
		}
	}
}

func TestMutantName(t *testing.T) {
	if got := mutantName(""); got != "mutant" {
		t.Errorf("mutantName(\"\") = %q", got)
	}
	if got := mutantName("logical"); got != "logical mutant" {
		t.Errorf("mutantName(logical) = %q", got)
	}
}

func TestVariantFirstByteDiffers(t *testing.T) {
	orig, mutated := "a := alpha.beta.gamma + 1000", "b := alpha.beta.gamma + 1000"
	if got := variant(orig, mutated, true); got != "`"+mutated+"`" {
		t.Errorf("variant = %q, want the whole line when the first byte changed", got)
	}
}
