// Package document holds text-layout rules shared by the domain features and the
// controller, such as how a document splits into LSP-indexed lines.
package document

// Lines splits document text into zero-based lines without their terminators,
// so positions line up with LSP line indices.
func Lines(text string) []string {
	var lines []string

	start := 0

	for i := range len(text) {
		if text[i] == '\n' {
			line := text[start:i]
			if n := len(line); n > 0 && line[n-1] == '\r' {
				line = line[:n-1]
			}

			lines = append(lines, line)
			start = i + 1
		}
	}

	last := text[start:]
	if n := len(last); n > 0 && last[n-1] == '\r' {
		last = last[:n-1]
	}

	lines = append(lines, last)

	return lines
}
