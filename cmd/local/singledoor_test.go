package local

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renderer.Emit is documented as "the single output path for every v2
// command", and until recently it was not: four payloads went around it with
// their own encoder, and a payload that goes around Emit is one nothing can
// observe.
//
// That matters beyond tidiness. The schema pins hold Go types; they cannot
// tell you a command still emits the type they hold. Binding command to type
// means watching what passes through one place, and the watching is only
// worth anything if everything passes through it.
//
// What this guard covers, precisely, because a guard whose reach is vague is
// one people assume more of than it delivers:
//
//   - json.NewEncoder, json.Marshal and json.MarshalIndent
//   - under any local name the encoding/json import is bound to, alias or not
//   - anywhere in the package, including at file scope, not only inside funcs
//   - everywhere except the body of Renderer.Emit itself
//
// It does not know whether a given call publishes anything. A legitimate
// non-output encoder — an HTTP request body, a cache file — says so with the
// directive below rather than being told to route a request body to somebody's
// stdout.
const jsonEscapeDirective = "//astro:non-output-json"

func TestEmitIsTheOnlyJSONEncoder(t *testing.T) {
	banned := map[string]bool{"NewEncoder": true, "Marshal": true, "MarshalIndent": true}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	var outside []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, parser.ParseComments)
		require.NoError(t, perr)

		// Whatever this file calls encoding/json. An alias defeated the first
		// version of this test outright.
		jsonNames := jsonImportNames(t, file)
		if len(jsonNames) == 0 {
			continue
		}
		exempt := emitBodyRange(file)
		excused := linesWithDirective(fset, file)

		// The whole file, not only function bodies: a package-level
		// `var _ = json.NewEncoder(os.Stdout)` was invisible to the first
		// version too.
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !banned[sel.Sel.Name] {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || !jsonNames[pkg.Name] {
				return true
			}
			pos := sel.Pos()
			if exempt.contains(pos) || excused[fset.Position(pos).Line] {
				return true
			}
			outside = append(outside, fset.Position(pos).String()+": "+pkg.Name+"."+sel.Sel.Name)
			return true
		})
	}

	assert.Empty(t, outside,
		"JSON encoded outside Renderer.Emit. Every published payload leaves by\n"+
			"one door, so that a test can see what a command actually emits.\n"+
			"Route this through Emit. If it is not output at all — a request\n"+
			"body, a cache file — put %s on or above the line,\n"+
			"with a reason, because Emit would write it to somebody's stdout.",
		jsonEscapeDirective)
}

// jsonImportNames returns the local names encoding/json is bound to in this
// file: "json" normally, whatever the alias says otherwise, and nothing at
// all if the file does not import it.
func jsonImportNames(t *testing.T, file *ast.File) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)
		if path != "encoding/json" {
			continue
		}
		if imp.Name != nil {
			names[imp.Name.Name] = true
		} else {
			names["json"] = true
		}
	}
	return names
}

// posRange is the source span of Renderer.Emit, the one place allowed to
// hold an encoder.
type posRange struct{ from, to token.Pos }

func (p posRange) contains(pos token.Pos) bool {
	return p.from != token.NoPos && pos >= p.from && pos <= p.to
}

// emitBodyRange finds Renderer.Emit by its receiver, not by its name. A
// second type with its own Emit method was a legal second door in the first
// version of this test.
func emitBodyRange(file *ast.File) posRange {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Emit" || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}
		if id, ok := recv.(*ast.Ident); ok && id.Name == "Renderer" {
			return posRange{from: fn.Pos(), to: fn.End()}
		}
	}
	return posRange{}
}

// linesWithDirective returns the lines the opt-out comment covers: its own,
// for a trailing comment, and the one after it, which is where Go puts a
// directive by convention.
func linesWithDirective(fset *token.FileSet, file *ast.File) map[int]bool {
	lines := map[int]bool{}
	for _, group := range file.Comments {
		for _, c := range group.List {
			if !strings.Contains(c.Text, jsonEscapeDirective) {
				continue
			}
			line := fset.Position(c.Pos()).Line
			lines[line] = true
			lines[line+1] = true
		}
	}
	return lines
}
