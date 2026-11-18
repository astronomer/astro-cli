package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A test run must not touch the machine it runs on: not the developer's
// ~/.astro (an agent's test once replaced its access token), not the OS
// keychain (macOS prompts for it), not their uv Pythons. pkg/testenv makes a
// test binary hermetic, from its TestMain. Nothing in a test package makes it
// call it, so these tests do: a package added without one fails here.
//
// The root module's packages call the root's front for it, pkg/testing/testenv,
// which adds the pkg/secrets keyring hooks. The pkg/* modules and e2e call
// pkg/testenv itself, and a package whose test binary links pkg/secrets passes
// those hooks in testenv.Options.

const (
	testenvPath    = modulePrefix + "pkg/testenv"
	testenvRootFor = modulePrefix + "pkg/testing/testenv"
	secretsPath    = modulePrefix + "pkg/secrets"
)

// testenvCalls are the calls a TestMain makes to isolate.
var testenvCalls = []string{"Main", "Isolate"}

// testPackage is a directory with test files, and what its TestMain does.
type testPackage struct {
	dir      string // relative to the walked root, slash-separated
	hasMain  bool
	isolates bool   // a TestMain calls testenv.Main or testenv.Isolate
	from     string // the import path of the testenv it calls
	hooks    bool   // the call passes Options with MockKeyring and KeyringRefusals
}

// testPackages walks root for directories with _test.go files, not descending
// into other modules.
func testPackages(t *testing.T, root string) []testPackage {
	t.Helper()
	var out []testPackage
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if p != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
			return filepath.SkipDir
		}
		if p != root {
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir // another module: walked from its own root
			}
		}
		files, err := filepath.Glob(filepath.Join(p, "*_test.go"))
		if err != nil || len(files) == 0 {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		pkg := testPackage{dir: filepath.ToSlash(rel)}
		pkg.hasMain, pkg.isolates, pkg.from, pkg.hooks = testMainOf(t, files)
		// A bare Main or Isolate is testenv's own only inside testenv; anywhere
		// else it is some local function that happens to be named like it.
		if pkg.isolates && pkg.from == "" && !isTestenvDir(root, pkg.dir) {
			pkg.isolates = false
		}
		out = append(out, pkg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// isTestenvDir reports whether dir, in the tree walked from root, holds testenv
// itself: pkg/testing/testenv in the root module, or the root of pkg/testenv.
func isTestenvDir(root, dir string) bool {
	if dir == "pkg/testing/testenv" {
		return true
	}
	return dir == "." && filepath.Base(root) == "testenv" && filepath.Base(filepath.Dir(root)) == "pkg"
}

// testMainOf reads the test files of one package for its TestMain and what it
// calls to isolate.
func testMainOf(t *testing.T, files []string) (hasMain, isolates bool, from string, hooks bool) {
	t.Helper()
	for _, file := range files {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		main, call := testMainCall(f)
		hasMain = hasMain || main
		if call != nil {
			isolates, from, hooks = true, call.from, call.hooks
		}
	}
	return hasMain, isolates, from, hooks
}

// isolation is a TestMain's call to testenv.Main or testenv.Isolate.
type isolation struct {
	from  string // import path of testenv; "" for a bare call inside testenv itself
	hooks bool
}

// testMainCall reports whether f declares TestMain, and the call it makes to
// isolate, if any.
func testMainCall(f *ast.File) (hasMain bool, call *isolation) {
	imports := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = p
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
			continue
		}
		hasMain = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := c.Fun.(type) {
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && containsString(testenvCalls, fun.Sel.Name) && imports[x.Name] != "" &&
					(imports[x.Name] == testenvPath || imports[x.Name] == testenvRootFor) {
					call = &isolation{from: imports[x.Name], hooks: passesKeyringHooks(c)}
				}
			case *ast.Ident:
				if containsString(testenvCalls, fun.Name) {
					call = &isolation{hooks: passesKeyringHooks(c)}
				}
			}
			return true
		})
	}
	return hasMain, call
}

func containsString(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// passesKeyringHooks reports whether the call has an Options literal setting both
// MockKeyring and KeyringRefusals to something other than nil. It reads the
// source: a hook behind a build tag or an `if false` is a limit it does not see.
func passesKeyringHooks(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.CompositeLit)
		if !ok {
			continue
		}
		keys := map[string]bool{}
		for _, e := range lit.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			// A hook set to nil is no hook.
			if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "nil" {
				continue
			}
			if id, ok := kv.Key.(*ast.Ident); ok {
				keys[id.Name] = true
			}
		}
		if keys["MockKeyring"] && keys["KeyringRefusals"] {
			return true
		}
	}
	return false
}

// modules lists the separate modules under the repository: the pkg/* ones and
// e2e, found by their go.mod.
func modules(t *testing.T, root string) []string {
	t.Helper()
	var mods []string
	for _, pattern := range []string{"pkg/*/go.mod", "e2e/go.mod"} {
		found, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		for _, gomod := range found {
			rel, err := filepath.Rel(root, filepath.Dir(gomod))
			if err != nil {
				t.Fatal(err)
			}
			mods = append(mods, filepath.ToSlash(rel))
		}
	}
	sort.Strings(mods)
	return mods
}

// Every test package in the root module isolates its run through the root's
// front, which passes the keyring hooks.
func TestEveryRootTestPackageIsolatesItsRun(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range testPackages(t, root) {
		switch {
		case !pkg.hasMain:
			t.Errorf("%s has tests and no TestMain. Add `func TestMain(m *testing.M) { testenv.Main(m) }` "+
				"(pkg/testing/testenv): a test run must not touch the developer's HOME or the OS keychain. "+
				"See docs/architecture.md, \"Tests never touch the machine\".", pkg.dir)
		case !pkg.isolates:
			t.Errorf("%s has a TestMain that does not call testenv.Main or testenv.Isolate, so its tests run against the "+
				"real HOME and keychain.", pkg.dir)
		case pkg.from != testenvRootFor && pkg.dir != "pkg/testing/testenv":
			t.Errorf("%s calls %q, not the root's pkg/testing/testenv, which passes the keyring hooks every root test binary needs", pkg.dir, pkg.from)
		}
	}
}

// Every test package in a module of its own isolates its run with pkg/testenv,
// and one whose test binary links pkg/secrets passes the keyring hooks, or its
// tests could reach the OS keychain, or fall back on its refusal unseen.
func TestEveryModuleTestPackageIsolatesItsRun(t *testing.T) {
	root := repoRoot(t)
	for _, mod := range modules(t, root) {
		modDir := filepath.Join(root, filepath.FromSlash(mod))
		links := linkingSecrets(t, modDir)
		for _, pkg := range testPackages(t, modDir) {
			dir := mod
			if pkg.dir != "." {
				dir = mod + "/" + pkg.dir
			}
			abs := filepath.Join(modDir, filepath.FromSlash(pkg.dir))
			switch {
			case !pkg.hasMain:
				t.Errorf("%s has tests and no TestMain. Add `func TestMain(m *testing.M) { testenv.Main(m) }` (pkg/testenv).", dir)
			case !pkg.isolates:
				t.Errorf("%s has a TestMain that does not call testenv.Main or testenv.Isolate, so its tests run against the real HOME", dir)
			case pkg.from != "" && pkg.from != testenvPath:
				t.Errorf("%s calls %q: a pkg module uses pkg/testenv", dir, pkg.from)
			case (links[abs] || mod == "pkg/secrets") && !pkg.hooks && mod != "pkg/testenv":
				t.Errorf("%s links pkg/secrets, so its TestMain passes the keyring hooks: "+
					"testenv.Main(m, testenv.Options{MockKeyring: secrets.MockKeyringForTests, KeyringRefusals: secrets.OSKeyringRefusals})", dir)
			}
		}
	}
}

// linkingSecrets lists the package directories of the module at dir whose test
// binary links pkg/secrets. Only a module whose go.mod names pkg/secrets can
// have one, so only those are asked.
func linkingSecrets(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gomod), secretsPath) {
		return out
	}
	cmd := exec.CommandContext(t.Context(), "go", "list", "-test", "-f", "{{.Dir}}|{{join .Deps \" \"}}", "./...")
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list in %s: %v", dir, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		pkgDir, deps, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		for _, dep := range strings.Fields(deps) {
			if dep == secretsPath {
				out[filepath.FromSlash(filepath.ToSlash(pkgDir))] = true
			}
		}
	}
	return out
}
