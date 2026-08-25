// Package archlint enforces the v2 layer rules from docs/v2-architecture.md
// as a plain go test, so `go test ./...` and CI catch violations without
// extra tooling:
//
//  1. Nothing under internal/ imports cmd/.
//  2. v2 packages below cmd/ never print, exit, or log fatally.
//  3. v2 packages never import config/ (or the v1 cmd tree).
//
// Closed value sets (localrt.Mode, localrt.State, ...) are covered by the
// `exhaustive` linter, already enabled repo-wide in .golangci.yml.
package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const modulePrefix = "github.com/astronomer/astro-cli/"

// v2BelowCmd lists v2 packages below the cmd/ layer, where printing and
// exiting are review-blocking. Extend as v2 packages land (internal/plan,
// ...).
var v2BelowCmd = []string{
	"internal/checks",
	"internal/deploy",
	"internal/envresolve",
	"internal/instances",
	"internal/localenv",
	"internal/pack",
	"internal/plan",
	"internal/project",
	"internal/scaffold",
	"internal/userstate",
	"internal/vaultenv",
	"pkg/airflowapi",
	"pkg/airflowenv",
	"pkg/connmodel",
	"pkg/envschema",
	"pkg/fsatomic",
	"pkg/imagebuild",
	"pkg/localrt",
	"pkg/manifest",
	"pkg/secrets",
}

// v2ConfigReaders lists the v2 packages that read config/ on purpose. Each
// exists so exactly one place in the tree touches it — the login session, the
// Environment Manager provider, the coordinate lookups — and the packages that
// need what they read take them as a seam instead. They are below cmd/, so the
// no-printing rule applies; the no-config rule cannot.
var v2ConfigReaders = []string{
	"internal/astrosession",
	"internal/emenv",
	"internal/instancelocate",
}

// v1Internal lists the packages under internal/ that predate v2 and are held
// to none of its rules. It exists so the check below can tell "this package is
// v1" from "somebody added a v2 package and forgot to register it".
var v1Internal = []string{
	"internal/archlint",
	"internal/otto",
	"internal/platformversions",
	"internal/telemetry",
}

// v2All lists every v2 package barred from importing config/ or the v1 cmd
// tree.
var v2All = append([]string{
	"cmd/local",
}, v2BelowCmd...)

// TestEveryInternalPackageIsAccountedFor: the lists above are what the rules
// are made of, and a rule nobody is on is not a rule. A new package under
// internal/ has to say which it is — a v2 package, a v2 package that reads
// config/ on purpose, or one of the v1 ones — so that forgetting is a failing
// test rather than a package that quietly obeys nothing.
func TestEveryInternalPackageIsAccountedFor(t *testing.T) {
	root := repoRoot(t)
	known := map[string]bool{}
	for _, pkg := range slices.Concat(v2BelowCmd, v2ConfigReaders, v1Internal) {
		known[pkg] = true
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pkg := "internal/" + entry.Name()
		if !known[pkg] {
			t.Errorf("%s is in none of v2BelowCmd, v2ConfigReaders, or v1Internal: add it to the one it belongs to (and to .golangci.yml's forbidigo path list if it is a v2 package)", pkg)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if dir == filepath.Dir(dir) {
			t.Fatalf("no go.mod above %s", wd)
		}
	}
}

// goFiles walks every .go file under root/rel, calling fn with the path
// relative to root.
func goFiles(t *testing.T, root, rel string, fn func(relPath string)) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join(root, rel), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fn(relPath)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func imports(t *testing.T, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, p)
	}
	return out
}

func TestInternalNeverImportsCmd(t *testing.T) {
	root := repoRoot(t)
	goFiles(t, root, "internal", func(rel string) {
		for _, imp := range imports(t, filepath.Join(root, rel)) {
			if strings.HasPrefix(imp, modulePrefix+"cmd") {
				t.Errorf("%s imports %s: internal/ must never import cmd/", rel, imp)
			}
		}
	})
}

func TestV2PackagesNeverImportConfigOrV1Cmd(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range v2All {
		goFiles(t, root, pkg, func(rel string) {
			for _, imp := range imports(t, filepath.Join(root, rel)) {
				if strings.HasPrefix(imp, modulePrefix+"config") {
					t.Errorf("%s imports %s: v2 packages never import config/", rel, imp)
				}
				// The v1 cmd package: everything directly under cmd/,
				// plus its v1 subpackages. The v2 tree may import its
				// own packages.
				if strings.HasPrefix(imp, modulePrefix+"cmd") &&
					!strings.HasPrefix(imp, modulePrefix+"cmd/local") &&
					!strings.HasPrefix(imp, modulePrefix+"cmd/astro") {
					t.Errorf("%s imports %s: v2 packages never import the v1 cmd tree", rel, imp)
				}
			}
		})
	}
}

// forbiddenCalls maps package selector to forbidden functions below cmd/.
var forbiddenCalls = map[string]map[string]bool{
	"fmt": {"Print": true, "Printf": true, "Println": true},
	"os":  {"Exit": true},
	"log": {"Fatal": true, "Fatalf": true, "Fatalln": true, "Panic": true, "Panicf": true, "Panicln": true},
}

func TestV2PackagesBelowCmdNeverPrintOrExit(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range slices.Concat(v2BelowCmd, v2ConfigReaders) {
		goFiles(t, root, pkg, func(rel string) {
			if strings.HasSuffix(rel, "_test.go") {
				return
			}
			checkNoPrintOrExit(t, root, rel)
		})
	}
}

func checkNoPrintOrExit(t *testing.T, root, rel string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	aliases, dotImported := forbiddenImports(t, rel, f)
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if what := forbiddenCallName(call, aliases, dotImported); what != "" {
			t.Errorf("%s:%d calls %s: below cmd/, report through callbacks and return errors instead",
				rel, fset.Position(call.Pos()).Line, what)
		}
		return true
	})
}

// forbiddenImports resolves each forbidden package's local name in f, so
// aliased and dot imports cannot slip past the check.
func forbiddenImports(t *testing.T, rel string, f *ast.File) (aliases map[string]string, dotImported map[string]bool) {
	t.Helper()
	aliases = map[string]string{} // local name -> import path
	dotImported = map[string]bool{}
	for _, imp := range f.Imports {
		impPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if forbiddenCalls[impPath] == nil {
			continue
		}
		switch {
		case imp.Name == nil:
			aliases[impPath] = impPath
		case imp.Name.Name == ".":
			dotImported[impPath] = true
		case imp.Name.Name == "_":
		default:
			aliases[imp.Name.Name] = impPath
		}
	}
	return aliases, dotImported
}

// forbiddenCallName names the forbidden function call is making, or returns
// "" if call is allowed.
func forbiddenCallName(call *ast.CallExpr, aliases map[string]string, dotImported map[string]bool) string {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := fun.X.(*ast.Ident)
		if !ok {
			return ""
		}
		if forbiddenCalls[aliases[ident.Name]][fun.Sel.Name] {
			return ident.Name + "." + fun.Sel.Name
		}
	case *ast.Ident:
		if fun.Name == "print" || fun.Name == "println" {
			return "builtin " + fun.Name
		}
		for impPath := range dotImported {
			if forbiddenCalls[impPath][fun.Name] {
				return impPath + "." + fun.Name + " (dot import)"
			}
		}
	}
	return ""
}
