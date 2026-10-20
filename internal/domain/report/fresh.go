package report

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// Freshness says whether reports still describe the files on disk.
type Freshness int

const (
	// Fresh means the source and its tests are as gooze tested them.
	Fresh Freshness = iota
	// TestChanged means a test file changed, appeared or disappeared since the
	// run, so mutants may now be killed (or survive) differently.
	TestChanged
	// SourceChanged means the source file changed since the run, so the
	// reports' line numbers may no longer match it.
	SourceChanged
)

// TestFiles returns the absolute paths of the test files whose contents tell
// whether the reports of the source at absPath are still current: the test
// file each report recorded, or the conventional _test.go next to the source
// for reports made when it had none. Callers read them into Current.Tests.
func TestFiles(reports []rpt.Report, workdir, absPath string) []string {
	var files []string

	for _, r := range ForFile(reports, workdir, absPath) {
		f := conventionalTest(absPath)
		if r.Source.Test != nil {
			f = resolve(r.Source.Test, workdir)
		}

		if !slices.Contains(files, f) {
			files = append(files, f)
		}
	}

	return files
}

// freshness compares the reports of one source with the files as they are
// now. When a test changed, it also returns that test's path.
func freshness(reports []rpt.Report, workdir, absPath string, cur Current) (Freshness, string) {
	hash := hashOf(cur.Text)
	state, changed := Fresh, ""

	for _, r := range reports {
		if !sourceFresh(r, hash) {
			return SourceChanged, ""
		}

		// Equivalent: state is Fresh or TestChanged here, so > Fresh is != Fresh (< Fresh is killed by a test).
		if cur.Tests == nil || state != Fresh { //gooze:ignore comparison
			continue
		}

		if test, ok := testChanged(r, workdir, absPath, cur.Tests); ok {
			state, changed = TestChanged, test
		}
	}

	return state, changed
}

// sourceFresh reports whether r was made from source text with the given hash.
// Reports without a recorded hash are taken as current.
func sourceFresh(r rpt.Report, hash string) bool {
	o := r.Source.Origin

	// Equivalent: no string sorts below "", so <= "" is == "" (<= hash is killed by a test).
	return o == nil || o.Hash == "" || o.Hash == hash //gooze:ignore comparison
}

// testChanged reports whether the test file r recorded differs from (or no
// longer matches) its current contents, or whether a test file appeared next
// to a source that had none. It returns the test file's path.
func testChanged(r rpt.Report, workdir, absPath string, tests map[string]string) (string, bool) {
	if r.Source.Test == nil {
		f := conventionalTest(absPath)
		_, added := tests[f]

		return f, added
	}

	// Equivalent: no string sorts below "", so <= "" is == "".
	if r.Source.Test.Hash == "" { //gooze:ignore comparison
		return "", false
	}

	f := resolve(r.Source.Test, workdir)
	text, ok := tests[f]

	return f, !ok || hashOf(text) != r.Source.Test.Hash
}

// conventionalTest is the test file gooze pairs with a source file.
func conventionalTest(absPath string) string {
	return strings.TrimSuffix(absPath, ".go") + "_test.go"
}

// hashOf is the content hash gooze records for a file: the hex SHA-256 of its
// bytes.
func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))

	return hex.EncodeToString(sum[:])
}
