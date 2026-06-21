// Package mutagen is the canonical registry of gooze mutation categories
// ("mutagens"). It is the innermost domain layer: it has zero dependencies on
// LSP, gopls, or any transport concern.
//
// The names and descriptions mirror gooze's own model.MutationType values
// (gooze.dev/pkg/gooze/internal/model). A gooze:ignore directive references a
// mutagen by its lowercase Name; gooze matches it case-insensitively.
package mutagen

// Mutagen is a single mutation category recognised by gooze.
type Mutagen struct {
	// Name is the canonical, lowercase identifier used in //gooze:ignore
	// directives (e.g. "arithmetic").
	Name string
	// Detail is a short one-line summary suitable for a completion item's
	// detail field.
	Detail string
	// Doc is a longer human-readable description suitable for hover.
	Doc string
}

// All is the ordered list of every mutagen gooze supports, taken verbatim from
// gooze's model.MutationType definitions.
var All = []Mutagen{
	{
		Name:   "arithmetic",
		Detail: "Arithmetic operator mutations",
		Doc:    "Mutates arithmetic operators (+, -, *, /, %).",
	},
	{
		Name:   "boolean",
		Detail: "Boolean literal mutations",
		Doc:    "Mutates boolean literals (true <-> false).",
	},
	{
		Name:   "numbers",
		Detail: "Numeric literal mutations",
		Doc:    "Mutates numeric literals (e.g. 5 -> 0, 5 -> 1).",
	},
	{
		Name:   "comparison",
		Detail: "Comparison operator mutations",
		Doc:    "Mutates comparison operators (<, >, <=, >=, ==, !=).",
	},
	{
		Name:   "logical",
		Detail: "Logical operator mutations",
		Doc:    "Mutates logical operators (&&, ||).",
	},
	{
		Name:   "unary",
		Detail: "Unary operator mutations",
		Doc:    "Mutates unary operators (-, +, !, ^).",
	},
	{
		Name:   "branch",
		Detail: "Branch/conditional mutations",
		Doc:    "Mutates branch and conditional expressions (if, for, switch conditions).",
	},
	{
		Name:   "statement",
		Detail: "Statement deletion mutations",
		Doc:    "Deletes statements (assignments, expressions, defer, go, send).",
	},
	{
		Name:   "loop",
		Detail: "Loop mutations",
		Doc:    "Mutates loops (boundary conditions, loop body removal, break/continue removal).",
	},
}

// byName indexes All by canonical lowercase name.
var byName = func() map[string]Mutagen {
	m := make(map[string]Mutagen, len(All))
	for _, mg := range All {
		m[mg.Name] = mg
	}
	return m
}()

// Lookup returns the mutagen with the given name and whether it exists. The
// lookup is case-insensitive to match gooze's directive handling.
func Lookup(name string) (Mutagen, bool) {
	mg, ok := byName[normalize(name)]
	return mg, ok
}

// IsValid reports whether name is a recognised mutagen (case-insensitive).
func IsValid(name string) bool {
	_, ok := byName[normalize(name)]
	return ok
}

// Names returns the canonical names of every mutagen, in registry order.
func Names() []string {
	names := make([]string, len(All))
	for i, mg := range All {
		names[i] = mg.Name
	}

	return names
}

func normalize(name string) string {
	// Mirror gooze: names are compared lowercased and trimmed.
	out := make([]byte, 0, len(name))
	for i := range len(name) {
		c := name[i]
		if c == ' ' || c == '\t' {
			continue
		}

		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}

		out = append(out, c)
	}

	return string(out)
}
