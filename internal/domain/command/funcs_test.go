package command

import (
	"reflect"
	"testing"

	"gooze.dev/pkg/goozels/internal/model"
)

func TestFuncLenses(t *testing.T) {
	src := `package calc

// InRange reports whether n is in [lo, hi].
func InRange(n, lo, hi int) bool { return n >= lo && n <= hi }

type Parser struct{}

	func (p *Parser) Next() int { return 1 }

func (Parser) Peek() int { return 2 }

type Gen[T any] struct{}

func (g Gen[T]) Do() T { var zero T; return zero }

func asm(x int) int
`
	type lens struct {
		line, col int
		args      []any
	}

	var got []lens

	for _, l := range FuncLenses(src, "./calc") {
		if l.Command.Command != RunFunction || l.Command.Title != RunFunctionTitle {
			t.Errorf("lens command = %+v", l.Command)
		}

		if l.Range.Start != l.Range.End {
			t.Errorf("lens range should be a point: %+v", l.Range)
		}

		got = append(got, lens{l.Range.Start.Line, l.Range.Start.Column, l.Command.Arguments})
	}

	want := []lens{
		{3, 0, []any{"./calc", "^InRange$"}},
		{7, 1, []any{"./calc", "^Parser\\.Next$"}}, // a method's lens sits on its own line and column
		{9, 0, []any{"./calc", "^Parser\\.Peek$"}}, // an unnamed value receiver
		{13, 0, []any{"./calc", "^Gen\\.Do$"}},     // a generic receiver, without type parameters
	} // asm has no body, so nothing to mutate and no lens

	if !reflect.DeepEqual(got, want) {
		t.Errorf("FuncLenses =\n%+v\nwant\n%+v", got, want)
	}
}

func TestFuncLensesNone(t *testing.T) {
	for name, src := range map[string]string{
		"no functions":    "package calc\n\nvar x = 1\n",
		"does not parse":  "package calc\n\nfunc broken( {\n",
		"not even a file": "",
	} {
		if got := FuncLenses(src, "."); len(got) != 0 {
			t.Errorf("%s: FuncLenses = %+v, want none", name, got)
		}
	}
}

func TestFuncLensesUsePointRanges(t *testing.T) {
	lenses := FuncLenses("package a\n\nfunc F() {}\n", ".")
	want := model.Range{Start: model.Position{Line: 2}, End: model.Position{Line: 2}}

	if len(lenses) != 1 || lenses[0].Range != want {
		t.Errorf("FuncLenses = %+v", lenses)
	}
}
