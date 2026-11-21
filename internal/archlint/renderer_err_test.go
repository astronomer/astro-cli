package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A cliout.Renderer writes its notices (an empty list's sentence, a success
// line, a warning) to its Err, and to the process's os.Stderr when Err is
// unset. A command's notice belongs on the command's own stderr
// (cmd.ErrOrStderr()), so that cmd.SetErr and a test capture it. A Renderer
// built without Err sends them around the command.
//
// The test covers exactly two kinds of literal in non-test code under cmd/:
//   - a cliout.Renderer, which must set Err (an empty one, the zero value
//     returned beside an error, writes nothing and is skipped);
//   - an options struct that hands its ErrOut to a Renderer (cmd/api's
//     ListOptions and DescribeOptions, listed in errOutOptions), which must
//     set ErrOut.
//
// It does not cover os.Stderr written to directly, or a Renderer built some
// other way than by a literal.
//
// rendererErrAllowed is what may build one without it because there is no
// command to take it from. Each says why.
var rendererErrAllowed = map[string]string{
	"cmd/cliout/errors.go": "EmitError writes the one json error object and no notice, and takes only " +
		"the writer the error goes to; there is no command in hand",
}

// errOutOptions are the options structs whose ErrOut becomes a Renderer's Err.
var errOutOptions = map[string]bool{"ListOptions": true, "DescribeOptions": true}

// Every cliout.Renderer literal under cmd/ sets Err, and every errOutOptions
// literal sets ErrOut, except what rendererErrAllowed names.
func TestEveryRendererLiteralSetsErr(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	used := map[string]bool{}
	for _, file := range trackedGoFiles(t, root) {
		if !strings.HasPrefix(file, "cmd/") || !heldToPlainText(file) {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(file)), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		inCliout := f.Name.Name == "cliout"
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch {
			case isRendererType(lit.Type, inCliout):
				if len(lit.Elts) == 0 || hasKey(lit, "Err") {
					return true
				}
			case isErrOutOptions(lit.Type):
				if hasKey(lit, "ErrOut") {
					return true
				}
			default:
				return true
			}
			if _, ok := rendererErrAllowed[file]; ok {
				used[file] = true
				return true
			}
			problems = append(problems, fset.Position(lit.Pos()).String())
			return true
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d literals leave the command's stderr unset (Renderer.Err, or ErrOut on an options struct):\n\t%s\n\n"+
			"Set it to the command's stderr (cmd.ErrOrStderr(), threaded as an errOut io.Writer "+
			"through the functions between the command and the renderer), so the notices it prints "+
			"follow cmd.SetErr. If there is genuinely no command, add the file to rendererErrAllowed "+
			"in internal/archlint/renderer_err_test.go and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for file := range rendererErrAllowed {
		if !used[file] {
			t.Errorf("rendererErrAllowed names %s, which builds no literal without it; remove the entry", file)
		}
	}
}

func isErrOutOptions(typ ast.Expr) bool {
	id, ok := typ.(*ast.Ident)
	return ok && errOutOptions[id.Name]
}

// isRendererType reports whether a composite literal's type is cliout.Renderer
// (or Renderer, inside package cliout itself).
func isRendererType(typ ast.Expr, inCliout bool) bool {
	switch t := typ.(type) {
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		return ok && pkg.Name == "cliout" && t.Sel.Name == "Renderer"
	case *ast.Ident:
		return inCliout && t.Name == "Renderer"
	}
	return false
}

func hasKey(lit *ast.CompositeLit, name string) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if k, ok := kv.Key.(*ast.Ident); ok && k.Name == name {
			return true
		}
	}
	return false
}
