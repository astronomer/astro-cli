package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// A test run must not touch the machine it runs on: not the developer's
// ~/.astro (an agent's test once replaced its access token), not the OS
// keychain (macOS prompts for it), not their uv Pythons. pkg/testing/testenv
// makes a package's tests hermetic, from its TestMain. Nothing in a test
// package makes it call it, so these tests do: a package added without one
// fails here.
//
// The root module's packages call testenv.Isolate. The pkg/* modules and e2e
// are modules of their own and cannot import the root's testenv, so each test
// package in them calls its own copy of the home isolation, isolateHome (or, in
// e2e, guardRealConfig, which only watches the real config), from a hermetic_test.go.
// Modules that cannot reach a home are exempt below, and what keeps that true
// is checked.

// hermeticExempt are the root-module test packages that may go without
// testenv.Isolate, each with why. A package needs a reason that is true of
// its tests, not of its code: "its tests set HOME themselves" is not one,
// because a test that forgets to is the case this guards.
//
// Empty today: every test package in the root module reaches config/, which
// reaches HOME, or is one a later test would.
var hermeticExempt = map[string]string{}

// homeFree are the modules whose code and tests cannot reach a home or the
// OS keychain, so a test package in them needs no isolation. It is not a
// promise kept by hand: TestHomeFreeModulesReachNoHome fails when one of them
// reads HOME, the XDG variables, ASTRO_HOME or a keyring, or requires a module
// that is not itself in this list (the way manifest, which reads HOME, reaches
// awsauth). A module that grows such a read moves out of this list into the
// ones with an isolateHome copy.
var homeFree = []string{
	"pkg/airflowapi",
	"pkg/airflowenv",
	"pkg/airflowrt",
	"pkg/astroauth",
	"pkg/connmodel",
	"pkg/container",
	"pkg/emfetch",
	"pkg/envschema",
	"pkg/fsatomic",
	"pkg/platformversions",
}

// homeReads is what makes code able to reach a home or the keychain.
var homeReads = regexp.MustCompile(`UserHomeDir|UserCacheDir|UserConfigDir|"HOME"|USERPROFILE|APPDATA|ASTRO_HOME|XDG_|go-keyring|go-homedir`)

// isolators are the calls a TestMain makes to isolate: testenv.Main or
// testenv.Isolate in the root module, and the copies in the modules of their own.
var isolators = []string{"Isolate", "isolateHome", "guardRealConfig"}

// testPackage is a directory with test files, and whether a TestMain in them
// calls an isolator.
type testPackage struct {
	dir      string // relative to the repository root, slash-separated
	isolated bool
	hasMain  bool
}

// testPackages walks root for directories with _test.go files, not descending
// into other modules unless module is that module's own root.
func testPackages(t *testing.T, root string) []testPackage {
	t.Helper()
	var out []testPackage
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
			return filepath.SkipDir
		}
		if path != root {
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir // another module: walked from its own root
			}
		}
		pkg := testPackage{}
		files, err := filepath.Glob(filepath.Join(path, "*_test.go"))
		if err != nil || len(files) == 0 {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkg.dir = filepath.ToSlash(rel)
		for _, file := range files {
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			main, calls := testMainCalls(f)
			pkg.hasMain = pkg.hasMain || main
			pkg.isolated = pkg.isolated || calls
		}
		out = append(out, pkg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// testMainCalls reports whether f declares TestMain, and whether its body calls
// one of the isolators: testenv.Isolate, testenv.Main or a bare name (a copy in this package,
// or Isolate inside testenv itself).
func testMainCalls(f *ast.File) (hasMain, isolates bool) {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
			continue
		}
		hasMain = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && x.Name == "testenv" && (fun.Sel.Name == "Isolate" || fun.Sel.Name == "Main") {
					isolates = true
				}
			case *ast.Ident:
				if slices.Contains(isolators, fun.Name) {
					isolates = true
				}
			}
			return true
		})
	}
	return hasMain, isolates
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

// Every test package in the root module isolates its run, with testenv.Isolate
// from TestMain.
func TestEveryRootTestPackageIsolatesItsRun(t *testing.T) {
	root := repoRoot(t)
	used := map[string]bool{}
	for _, pkg := range testPackages(t, root) {
		dir := pkg.dir
		if why, ok := hermeticExempt[dir]; ok {
			used[dir] = true
			if why == "" {
				t.Errorf("hermeticExempt[%q] has no reason", dir)
			}
			continue
		}
		switch {
		case !pkg.hasMain:
			t.Errorf("%s has tests and no TestMain. Add one that calls testenv.Main(m) "+
				"(pkg/testing/testenv): a test run must not touch the developer's HOME or the OS keychain. "+
				"See docs/architecture.md, \"Tests never touch the machine\".", dir)
		case !pkg.isolated:
			t.Errorf("%s has a TestMain that does not call testenv.Isolate, so its tests run against the "+
				"real HOME and keychain. Make it `testenv.Main(m)`, or call `finish := testenv.Isolate()` first and exit with `finish(m.Run())`.", dir)
		}
	}
	for dir := range hermeticExempt {
		if !used[dir] {
			t.Errorf("hermeticExempt names %q, which is not a root-module test package: remove it", dir)
		}
	}
}

// Every test package in a module of its own isolates its run too, with its copy
// of the isolation, unless the whole module is home-free.
func TestEveryModuleTestPackageIsolatesItsRun(t *testing.T) {
	root := repoRoot(t)
	for _, mod := range modules(t, root) {
		if slices.Contains(homeFree, mod) {
			continue
		}
		for _, pkg := range testPackages(t, filepath.Join(root, filepath.FromSlash(mod))) {
			dir := mod
			if pkg.dir != "." {
				dir = mod + "/" + pkg.dir
			}
			switch {
			case !pkg.hasMain:
				t.Errorf("%s has tests and no TestMain. Copy hermetic_test.go from another package of this module "+
					"and call isolateHome from a TestMain (this module cannot import testenv). "+
					"If the module cannot reach a home at all, add it to homeFree.", dir)
			case !pkg.isolated:
				t.Errorf("%s has a TestMain that does not call isolateHome (or guardRealConfig), so its tests "+
					"run against the real HOME", dir)
			}
		}
	}
}

// Every module test package's isolateHome is the template, unchanged but for the
// package clause, so a fix to the isolation reaches all of them. e2e has its own
// (it does not move HOME, see guardRealConfig) and is not one of them.
func TestModuleIsolationCopiesMatchTheTemplate(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "internal", "archlint", "testdata", "hermetic_template.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, templateBody, _ := strings.Cut(string(raw), "\n")
	for _, mod := range modules(t, root) {
		if mod == "e2e" || slices.Contains(homeFree, mod) {
			continue
		}
		for _, pkg := range testPackages(t, filepath.Join(root, filepath.FromSlash(mod))) {
			file := filepath.Join(root, filepath.FromSlash(mod), filepath.FromSlash(pkg.dir), "hermetic_test.go")
			got, err := os.ReadFile(file)
			if err != nil {
				t.Errorf("%s/%s has no hermetic_test.go: copy internal/archlint/testdata/hermetic_template.go.txt there, changing only the package clause (%v)", mod, pkg.dir, err)
				continue
			}
			first, body, _ := strings.Cut(string(got), "\n")
			if !strings.HasPrefix(first, "package ") || body != templateBody {
				t.Errorf("%s differs from internal/archlint/testdata/hermetic_template.go.txt: change the template and copy it over every module test package, not one copy", filepath.ToSlash(file))
			}
		}
	}
}

// The template unsets every variable testenv does (those it does not unset by
// prefix), so a variable added to testenv.behaviorEnv reaches the modules too.
func TestTemplateClearsWhatTestenvClears(t *testing.T) {
	root := repoRoot(t)
	template, err := os.ReadFile(filepath.Join(root, "internal", "archlint", "testdata", "hermetic_template.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg", "testing", "testenv", "testenv.go"), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "behaviorEnv" || len(spec.Values) != 1 {
			return true
		}
		if lit, ok := spec.Values[0].(*ast.CompositeLit); ok {
			for _, e := range lit.Elts {
				if s, ok := e.(*ast.BasicLit); ok {
					names = append(names, strings.Trim(s.Value, `"`))
				}
			}
		}
		return false
	})
	if len(names) < 10 {
		t.Fatalf("read %d names from testenv.behaviorEnv; this test no longer finds it", len(names))
	}
	for _, name := range names {
		if strings.HasPrefix(name, "ASTRO_") || strings.HasPrefix(name, "ASTRONOMER_") {
			continue // the template unsets these by prefix
		}
		if !strings.Contains(string(template), `"`+name+`"`) {
			t.Errorf("testenv unsets %s and internal/archlint/testdata/hermetic_template.go.txt does not: add it to behaviorVars, and copy the template over every module test package", name)
		}
	}
}

// The modules exempt from isolation have to stay unable to reach a home.
func TestHomeFreeModulesReachNoHome(t *testing.T) {
	root := repoRoot(t)
	all := modules(t, root)
	for _, mod := range homeFree {
		if !slices.Contains(all, mod) {
			t.Errorf("homeFree names %q, which is not a module: remove it", mod)
			continue
		}
		dir := filepath.Join(root, filepath.FromSlash(mod))
		goFiles(t, root, mod, func(rel string) {
			src, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			if m := homeReads.Find(src); m != nil {
				t.Errorf("%s reads %q, so %s can reach a home: take it out of homeFree and give its test packages isolateHome", rel, m, mod)
			}
		})
		gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err != nil {
			t.Fatal(err)
		}
		for _, other := range regexp.MustCompile(`astro-cli/(pkg/[a-z]+)`).FindAllStringSubmatch(string(gomod), -1) {
			if other[1] != mod && !slices.Contains(homeFree, other[1]) {
				t.Errorf("%s requires %s, which is not home-free, so %s can reach a home through it", mod, other[1], mod)
			}
		}
	}
}
