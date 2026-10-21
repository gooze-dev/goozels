package command

import (
	"go/ast"
	"go/parser"
	"go/token"

	"gooze.dev/pkg/goozels/internal/model"
)

// FuncLenses returns a "run function" lens on each function declaration of a
// Go file with a body, which runs gooze on just that function. pathArg is the
// package pattern (see PackagePattern). The lens passes a --run pattern that
// matches the function exactly: ^Name$, or ^Type\.Name$ for a method. A file
// that doesn't parse gets none.
func FuncLenses(text, pathArg string) []model.CodeLens {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", text, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}

	var lenses []model.CodeLens

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}

		pos := fset.Position(fd.Pos())
		at := model.Position{Line: pos.Line - 1, Column: pos.Column - 1}

		lenses = append(lenses, model.CodeLens{
			Range: model.Range{Start: at, End: at},
			Command: model.Command{
				Title:     RunFunctionTitle,
				Command:   RunFunction,
				Arguments: []any{pathArg, funcPattern(fd)},
			},
		})
	}

	return lenses
}

// funcPattern is the --run pattern matching exactly one declared function.
// Go identifiers hold no regular-expression metacharacters, so the names need
// no quoting.
func funcPattern(fd *ast.FuncDecl) string {
	if recv := receiverName(fd); recv != "" {
		return "^" + recv + "\\." + fd.Name.Name + "$"
	}

	return "^" + fd.Name.Name + "$"
}

// receiverName returns a method's receiver type name without pointer or type
// parameters (Parser for *Parser, Gen for Gen[T]), or "" for a function.
func receiverName(fd *ast.FuncDecl) string {
	// Equivalent: a list length is never negative.
	if fd.Recv == nil || len(fd.Recv.List) == 0 { //gooze:ignore comparison
		return ""
	}

	expr := fd.Recv.List[0].Type
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}
