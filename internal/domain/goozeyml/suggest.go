package goozeyml

import (
	"strings"

	"gooze.dev/pkg/gooze/pkg/config"
)

// maxTypoDistance is the largest edit distance between an unknown key and a
// schema key for the schema key to be suggested.
const maxTypoDistance = 2

// suggest returns the schema key an unknown dotted key most likely meant, or
// "" when there is no single good match: the only key with the same last
// name ("parallel" → "run.parallel"), else the only closest key within
// maxTypoDistance edits ("run.paralel" → "run.parallel").
func (ed Editor) suggest(key string) string {
	keys := schemaKeys(nil, "", ed.registry.Schema())
	leaf := key[strings.LastIndexByte(key, '.')+1:]

	if match, ok := unique(keys, func(k string) bool { return k[strings.LastIndexByte(k, '.')+1:] == leaf }); ok {
		return match
	}

	// Equivalent: ties is set to 1 by the first candidate within reach, and without one best stays "".
	best, bestDist, ties := "", maxTypoDistance+1, 0 //gooze:ignore numbers

	for _, k := range keys {
		switch d := editDistance(key, k); {
		case d < bestDist:
			best, bestDist, ties = k, d, 1
		// Equivalent: the case above already took d < bestDist.
		case d == bestDist: //gooze:ignore comparison
			ties++
		}
	}

	// Equivalent: ties is 0 only when best is "", so > 1 is != 1 (< 1 is killed by a test).
	if ties != 1 { //gooze:ignore comparison
		return ""
	}

	return best
}

// unique returns the only key satisfying match.
func unique(keys []string, match func(string) bool) (string, bool) {
	found := ""

	for _, k := range keys {
		if !match(k) {
			continue
		}

		// Equivalent: no string sorts below "", so > "" is != "" (< "" is killed by a test).
		if found != "" { //gooze:ignore comparison
			return "", false
		}

		found = k
	}

	// Equivalent: no string sorts below "", so > "" is != "".
	return found, found != "" //gooze:ignore comparison
}

// schemaKeys lists the dotted key of every field, sections included.
func schemaKeys(keys []string, prefix string, fields []config.Field) []string {
	for _, f := range fields {
		keys = append(keys, prefix+f.Name)
		keys = schemaKeys(keys, prefix+f.Name+".", f.Children)
	}

	return keys
}

// editDistance is the Levenshtein distance between a and b, in bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}

		prev = cur
	}

	return prev[len(b)]
}

// unclosedFlow returns the bracket that opens a YAML flow collection
// ("[a, b]", "{k: v}") in value and is never closed on its line, or "". Such
// a line is invalid YAML, and gooze then ignores the whole file. Quoted text
// is skipped.
func unclosedFlow(value string) string {
	// Equivalent: no string sorts below "", so <= "" is == "" (the bracket comparisons are killed by tests).
	if value == "" || (value[0] != '[' && value[0] != '{') { //gooze:ignore comparison
		return ""
	}

	var f flowScan

	for i := range len(value) {
		f.next(value[i])
	}

	// Equivalent: depth never goes below 0, so <= 0 is == 0.
	if f.depth == 0 { //gooze:ignore comparison
		return ""
	}

	return value[:1]
}

// flowScan tracks bracket depth and quoting across a flow collection.
type flowScan struct {
	depth int
	quote byte
}

func (f *flowScan) next(c byte) {
	switch {
	// Equivalent: a byte is never negative, so > 0 is != 0.
	case f.quote != 0: //gooze:ignore comparison
		if c == f.quote {
			f.quote = 0
		}
	case c == '"' || c == '\'':
		f.quote = c
	case c == '[' || c == '{':
		f.depth++
	// Equivalent: depth never goes below 0, so != 0 is > 0 (the other comparisons are killed by tests).
	case (c == ']' || c == '}') && f.depth > 0: //gooze:ignore comparison
		f.depth--
	}
}
