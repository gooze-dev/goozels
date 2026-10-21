package report

import (
	"path/filepath"
	"reflect"
	"testing"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// TestDiffChangesCharacterization pins the exact changes read from diffs, so
// swapping the parser behind diffChanges cannot move a marker.
func TestDiffChangesCharacterization(t *testing.T) {
	cases := map[string]struct {
		diff string
		want []change
	}{
		"two hunks": {
			"--- original\n+++ mutated\n@@ -2,4 +2,4 @@\n a\n-\tif x < y {\n+\tif x <= y {\n b\n@@ -20,3 +20,3 @@\n c\n-  return a + b\n+  return a - b\n d\n",
			[]change{{line: 2, indent: 1, end: 11, text: "\tif x < y {", mutated: "if x <= y {"}, {line: 20, indent: 2, end: 14, text: "  return a + b", mutated: "return a - b"}},
		},
		"crlf": {
			"--- original\n+++ mutated\n@@ -1,3 +1,3 @@\n a\r\n-x := 1\r\n+x := 0\r\n b\r\n",
			[]change{{line: 1, indent: 0, end: 7, text: "x := 1\r", mutated: "x := 0"}},
		},
		"no newline at end": {
			"--- original\n+++ mutated\n@@ -1,2 +1,2 @@\n a\n-return true\n\\ No newline at end of file\n+return false\n\\ No newline at end of file\n",
			[]change{{line: 1, indent: 0, end: 11, text: "return true", mutated: "return false"}},
		},
		"removed line": {
			"--- original\n+++ mutated\n@@ -3,3 +3,2 @@\n a\n-\tdo()\n b\n",
			[]change{{line: 3, indent: 1, end: 5, text: "\tdo()", mutated: ""}},
		},
		"unchanged tail dropped": {
			"--- original\n+++ mutated\n@@ -5,4 +5,4 @@\n a\n-x < y\n-}\n+x <= y\n+}\n",
			[]change{{line: 5, indent: 0, end: 5, text: "x < y", mutated: "x <= y"}},
		},
		"malformed header": {"--- original\n+++ mutated\n@@ bogus @@\n-x\n+y\n", nil},
		"first line": {
			"--- original\n+++ mutated\n@@ -1,2 +1,2 @@\n-package a\n+package b\n x\n",
			[]change{{line: 0, indent: 0, end: 9, text: "package a", mutated: "package b"}},
		},
	}

	for name, tc := range cases {
		if got := diffChanges(tc.diff); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: diffChanges = %#v, want %#v", name, got, tc.want)
		}
	}
}

// TestDiffChangesCharacterizationOfTestdata pins the changes of every report
// in testdata/reports.
func TestDiffChangesCharacterizationOfTestdata(t *testing.T) {
	want := map[string][]change{
		"1ec5fe8dcb0fe411.yaml": {{line: 15, indent: 1, end: 13, text: "\treturn n * 2", mutated: "return n - 2"}},
		"24cbb4468032c589.yaml": {{line: 15, indent: 1, end: 13, text: "\treturn n * 2", mutated: "return n * 1"}},
		"406fb538029703a9.yaml": {{line: 10, indent: 1, end: 13, text: "\treturn n > 0", mutated: "return n > 1"}},
		"41af641bd6f5c637.yaml": {{line: 15, indent: 1, end: 13, text: "\treturn n * 2", mutated: "return n % 2"}},
		"5372a2f319200cb8.yaml": {{line: 15, indent: 1, end: 13, text: "\treturn n * 2", mutated: "return n / 2"}},
		"55e7a51270ad6598.yaml": {{line: 15, indent: 1, end: 13, text: "\treturn n * 2", mutated: "return n * 0"}},
		"a2246b76432dbe89.yaml": {{line: 10, indent: 1, end: 13, text: "\treturn n > 0", mutated: "return n >= 0"}},
		"b78bd7fdf88acd92.yaml": {{line: 10, indent: 1, end: 13, text: "\treturn n > 0", mutated: "return n != 0"}},
		"c15558913c8ec793.yaml": {{line: 15, indent: 1, end: 13, text: "\treturn n * 2", mutated: "return n + 2"}},
	}

	files, err := filepath.Glob(filepath.Join("..", "..", "..", "testdata", "reports", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	seen := 0

	for _, file := range files {
		r, err := rpt.ReadFile(file)
		if err != nil || r.Diff == nil {
			continue
		}

		seen++

		name := filepath.Base(file)
		if got := diffChanges(string(*r.Diff)); !reflect.DeepEqual(got, want[name]) {
			t.Errorf("%s: diffChanges = %#v, want %#v", name, got, want[name])
		}
	}

	if seen != len(want) {
		t.Errorf("checked %d reports with diffs, want %d", seen, len(want))
	}
}
