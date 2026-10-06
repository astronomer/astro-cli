package local

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renderer.Emit (cmd/cliout), with its stream twin EmitEvent, is documented as
// "the single output path for every core command", and until recently it was not: four payloads went around it with
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
//   - json.NewEncoder, json.Marshal and json.MarshalIndent, and
//     jsoncolor.NewEncoder, which colors the same output on a terminal
//   - under any local name either import is bound to, alias or not
//   - anywhere in this package, in cmd/cliout, where Emit lives, and in the
//     two renderers below cmd/ that the Astro tree hands its Renderer to
//     (pkg/output and internal/platform/astro/env), including at file scope,
//     not only inside funcs
//   - everywhere except the body of Renderer.emit, the one method Emit and
//     EmitEvent both lead through
//
// It does not know whether a given call publishes anything. A legitimate
// non-output encoder — an HTTP request body, a cache file — says so with the
// directive below rather than being told to route a request body to somebody's
// stdout.
const jsonEscapeDirective = "//astro:non-output-json"

func TestEmitIsTheOnlyJSONEncoder(t *testing.T) {
	banned := map[string]bool{"NewEncoder": true, "Marshal": true, "MarshalIndent": true}

	var sources []string
	for _, dir := range []string{
		".",
		filepath.Join("..", "cliout"),
		filepath.Join("..", "..", "pkg", "output"),
		filepath.Join("..", "..", "internal", "platform", "astro", "env"),
	} {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			sources = append(sources, filepath.Join(dir, name))
		}
	}

	fset := token.NewFileSet()
	var outside []string
	sawEmit := false
	for _, name := range sources {
		file, perr := parser.ParseFile(fset, name, nil, parser.ParseComments)
		require.NoError(t, perr)
		if emitBodyRange(file).from != token.NoPos {
			sawEmit = true
		}

		// Whatever this file calls encoding/json or jsoncolor. An alias
		// defeated the first version of this test outright.
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

	// The exemption is found by shape, so a move or rename of Emit would leave
	// the guard exempting nothing and still pass. Finding it is the check that
	// the scan is still looking where the door is.
	assert.True(t, sawEmit, "Renderer.emit was not found in the scanned sources: %v", sources)
	assert.Empty(t, outside,
		"JSON encoded outside Renderer.emit. Every published payload leaves by\n"+
			"one door, so that a test can see what a command actually emits.\n"+
			"Route this through Emit. If it is not output at all — a request\n"+
			"body, a cache file — put %s on or above the line,\n"+
			"with a reason, because Emit would write it to somebody's stdout.",
		jsonEscapeDirective)
}

// jsonImportNames returns the local names encoding/json and jsoncolor are
// bound to in this file: "json" or "jsoncolor" normally, whatever the alias
// says otherwise, and nothing at all if the file imports neither.
func jsonImportNames(t *testing.T, file *ast.File) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		require.NoError(t, err)
		def, ok := jsonEncoderImports[path]
		if !ok {
			continue
		}
		if imp.Name != nil {
			names[imp.Name.Name] = true
		} else {
			names[def] = true
		}
	}
	return names
}

// jsonEncoderImports maps each package that can encode json for output to
// the name it binds by default.
var jsonEncoderImports = map[string]string{
	"encoding/json":                   "json",
	"github.com/neilotoole/jsoncolor": "jsoncolor",
}

// posRange is the source span of Renderer.emit, the one place allowed to
// hold an encoder.
type posRange struct{ from, to token.Pos }

func (p posRange) contains(pos token.Pos) bool {
	return p.from != token.NoPos && pos >= p.from && pos <= p.to
}

// emitBodyRange finds Renderer.emit by its receiver, not by its name alone. A
// second type with its own Emit method was a legal second door in the first
// version of this test.
func emitBodyRange(file *ast.File) posRange {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "emit" || fn.Recv == nil || len(fn.Recv.List) != 1 {
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
