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
// rendererErrAllowed is what may build a Renderer without Err because there
// is no command to take it from. Each says why.
var rendererErrAllowed = map[string]string{
	"cmd/cliout/errors.go": "EmitError writes the one json error object and no notice, and takes only " +
		"the writer the error goes to; there is no command in hand",
}

// Every cliout.Renderer literal under cmd/ sets Err (an empty one, the zero value
// returned beside an error, writes nothing), except what
// rendererErrAllowed names.
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
			if !ok || !isRendererType(lit.Type, inCliout) || len(lit.Elts) == 0 || hasErrKey(lit) {
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
		t.Errorf("%d cliout.Renderer literals leave Err unset:\n\t%s\n\n"+
			"Set Err to the command's stderr (cmd.ErrOrStderr(), threaded as an errOut io.Writer "+
			"through the functions between the command and the renderer), so the notices it prints "+
			"follow cmd.SetErr. If there is genuinely no command, add the file to rendererErrAllowed "+
			"in internal/archlint/renderer_err_test.go and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for file := range rendererErrAllowed {
		if !used[file] {
			t.Errorf("rendererErrAllowed names %s, which builds no Renderer without Err; remove the entry", file)
		}
	}
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

func hasErrKey(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Err" {
			return true
		}
	}
	return false
}
