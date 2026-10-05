// Package archlint enforces the v2 layer rules from docs/v2-architecture.md
// as a plain go test, so `go test ./...` and CI catch violations without
// extra tooling:
//
//  1. Nothing under internal/ imports cmd/.
//  2. v2 packages below cmd/ never print, exit, or log fatally.
//  3. v2 packages never import config/ (or the v1 cmd tree).
//  4. Only the named seams import internal/platform/.
//
// Closed value sets (localrt.Mode, localrt.State, ...) are covered by the
// `exhaustive` linter, already enabled repo-wide in .golangci.yml.
package archlint

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
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
	"internal/apirequest",
	"internal/deploy",
	"internal/envresolve",
	"internal/localenv",
	"internal/pack",
	"internal/plan",
	"internal/project",
	"internal/runtimecatalog",
	"internal/userstate",
	"internal/vaultenv",
	"pkg/airflowapi",
	"pkg/awsauth",
	"pkg/airflowenv",
	"pkg/checks",
	"pkg/connmodel",
	"pkg/connwarehouse",
	"pkg/emfetch",
	"pkg/envschema",
	"pkg/googleauth",
	"pkg/fsatomic",
	"pkg/imagebuild",
	"pkg/instancelocate",
	"pkg/instances",
	"pkg/localrt",
	"pkg/manifest",
	"pkg/platformversions",
	"pkg/runtimeversions",
	"pkg/scaffold",
	"pkg/secrets",
	"pkg/uv",
	"pkg/astroauth",
	"pkg/container",
	"pkg/telemetry",
	"pkg/proxy",
	"pkg/airflowrt",
}

// v2ConfigReaders lists the v2 packages that read config/ on purpose. Each
// exists so exactly one place in the tree touches it — the login session, the
// Environment Manager provider, the coordinate lookups — and the packages that
// need what they read take them as a seam instead. They are below cmd/, so the
// no-printing rule applies; the no-config rule cannot.
var v2ConfigReaders = []string{
	"internal/astrosession",
	"internal/containercfg",
	"internal/emenv",
	"internal/instancelocate",
}

// v1Internal lists the packages under internal/ that predate v2 and are held
// to none of its rules. It exists so the check below can tell "this package is
// v1" from "somebody added a v2 package and forgot to register it".
var v1Internal = []string{
	"internal/archlint",
	"internal/otto",
	"internal/telemetry",
	// Both control-plane platforms: v1 code that prints and reads config/,
	// moved under internal/ without being rewritten. The subtree form says
	// "every package here is v1" in one line; a v2 package added under
	// internal/platform (local, when it lands) has to name itself.
	"internal/platform/apc/...",
	"internal/platform/astro/...",
}

// v2All lists every v2 package barred from importing config/ or the v1 cmd
// tree.
var v2All = append(v2Cmd, v2BelowCmd...)

// v2Cmd lists the v2 packages in the cmd/ layer. They may import each other,
// and nothing else under cmd/. cmd/cliout is the output contract the whole
// CLI shares; it is v2 code that the v1 tree imports, never the reverse.
var v2Cmd = []string{
	"cmd/cliout",
	"cmd/local",
}

// platformSeams are the packages under internal/ allowed to import
// internal/platform/. Each exists to be the one place in the tree that reaches
// a platform, so the packages needing what it fetches take it as a seam
// instead — the same posture as v2ConfigReaders above, and for the same reason.
// instancelocate's own doc comment says so in as many words.
//
// A package earns a place here by being that door for something, not by
// happening to need a client. internal/plan wanted one and took a constructor
// instead.
var platformSeams = []string{
	"internal/emenv",
	"internal/instancelocate",
}

// TestOnlyTheSeamsReachIntoAPlatform is the rule the internal/platform layout
// exists for: logic that is not about one platform takes an interface, and
// cmd/ wires the implementation. Anything else is coupling that spreads
// quietly, and unlike "is this package v1 or v2" it is decidable from the
// import path with no list to maintain.
//
// Test files are exempt on purpose. The rule is about the production
// dependency graph — what a platform-agnostic package needs to be built
// against. A test that wires a platform mock to drive a seam is testing the
// seam, and forbidding it would move those assertions somewhere less specific;
// internal/plan/workspace_env_test.go is the case in point, where the mock is
// what makes the Environment Manager messages assertable at all.
func TestOnlyTheSeamsReachIntoAPlatform(t *testing.T) {
	root := repoRoot(t)
	seam := map[string]bool{}
	for _, pkg := range platformSeams {
		seam[pkg] = true
	}
	platformPrefix := modulePrefix + "internal/platform/"

	for _, pkg := range accountablePackages(t, root, "internal") {
		if seam[pkg] || strings.HasPrefix(pkg, "internal/platform/") {
			continue
		}
		goFiles(t, root, pkg, func(rel string) {
			if strings.HasSuffix(rel, "_test.go") {
				return
			}
			for _, imp := range imports(t, filepath.Join(root, rel)) {
				if strings.HasPrefix(imp, platformPrefix) {
					t.Errorf("%s imports %s: only the platformSeams may import internal/platform/ — take an interface and let cmd/ wire it", rel, imp)
				}
			}
		})
	}
}

// TestEveryInternalPackageIsAccountedFor: the lists above are what the rules
// are made of, and a rule nobody is on is not a rule. A new package under
// internal/ has to say which it is — a v2 package, a v2 package that reads
// config/ on purpose, or one of the v1 ones — so that forgetting is a failing
// test rather than a package that quietly obeys nothing.
func TestEveryInternalPackageIsAccountedFor(t *testing.T) {
	root := repoRoot(t)
	known := map[string]bool{}
	var subtrees []string
	for _, pkg := range slices.Concat(v2BelowCmd, v2ConfigReaders, v1Internal) {
		if prefix, ok := strings.CutSuffix(pkg, "/..."); ok {
			subtrees = append(subtrees, prefix+"/")
			continue
		}
		known[pkg] = true
	}
	covered := func(pkg string) bool {
		if known[pkg] {
			return true
		}
		for _, prefix := range subtrees {
			if strings.HasPrefix(pkg, prefix) {
				return true
			}
		}
		return false
	}
	for _, pkg := range accountablePackages(t, root, "internal") {
		if !covered(pkg) {
			t.Errorf("%s is in none of v2BelowCmd, v2ConfigReaders, or v1Internal: add it to the one it belongs to (and to .golangci.yml's forbidigo path list if it is a v2 package)", pkg)
		}
	}
}

// accountablePackages lists the directories under dir that a rule has to name.
//
// A directory holding .go files is one of them, and its own subdirectories ride
// along on its entry — internal/otto has always worked that way. A directory
// holding no .go files is a grouping directory (internal/platform), so the
// packages under it are named individually. Without that, adding one grouping
// directory to a list would exempt everything anyone ever puts inside it, and
// internal/platform holds both v1 and v2 code on purpose.
func accountablePackages(t *testing.T, root, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		t.Fatal(err)
	}
	var pkgs []string
	var subdirs []string
	hasGo := false
	for _, entry := range entries {
		switch {
		case entry.IsDir():
			subdirs = append(subdirs, entry.Name())
		case strings.HasSuffix(entry.Name(), ".go"):
			hasGo = true
		}
	}
	for _, name := range subdirs {
		child := dir + "/" + name
		if hasGo {
			// dir is itself a package; its subdirectories ride on its entry.
			continue
		}
		if sub := accountablePackages(t, root, child); len(sub) > 0 {
			pkgs = append(pkgs, sub...)
			continue
		}
		pkgs = append(pkgs, child)
	}
	if hasGo {
		return nil
	}
	return pkgs
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
				//
				// cmd/astro is not among them. A cmd/astro existed briefly
				// as the v2 composition root and went when that root
				// moved onto cmd/ proper; the name now holds the v1
				// Astro command tree, which is exactly what this forbids.
				if strings.HasPrefix(imp, modulePrefix+"cmd") && !importsV2Cmd(imp) {
					t.Errorf("%s imports %s: v2 packages never import the v1 cmd tree", rel, imp)
				}
			}
		})
	}
}

// importsV2Cmd reports whether imp is one of the v2 cmd packages, or below one.
func importsV2Cmd(imp string) bool {
	for _, pkg := range v2Cmd {
		if imp == modulePrefix+pkg || strings.HasPrefix(imp, modulePrefix+pkg+"/") {
			return true
		}
	}
	return false
}

// authDoors are the packages that carry an expensive auth chain. Each exists so
// a consumer can decline it.
var authDoors = []string{
	modulePrefix + "pkg/awsauth",
	modulePrefix + "pkg/googleauth",
}

// expensiveSDKs are the chains the doors exist to make optional, by import
// prefix — a direct import would bypass the door rule above.
//
// Deliberately not "anything outside this module": pkg/instances
// legitimately reaches a TOML parser through pkg/manifest, so a purity rule
// would be false here on day one. This is a cost guard, and these are the cost:
// against a core that links 5.1MB, Google's chain adds about 1.9MB and AWS's
// about 6.9MB.
var expensiveSDKs = []string{
	"github.com/aws/",
	"golang.org/x/oauth2/google",
	"cloud.google.com/",
	// Google's generated client, which internal/instancelocate deliberately
	// declined; it is the one a future Composer feature is likeliest to reach
	// for, and it brings grpc and protobuf with it.
	"google.golang.org/api/",
}

// TestTheAuthDoorsStayOptional keeps the expensive chains out of everything
// that is not a door.
//
// Go links what is imported, so this property IS the saving. Two things about
// how it is checked, both learned by getting them wrong first.
//
// It asks `go list -deps` rather than reading import blocks, because the
// property is transitive and a one-hop reacquisition is invisible to a direct
// check. internal/instancelocate imported one small package for three Google
// helpers and inherited eighty-eight AWS packages through it, while every
// import block in that tree named no SDK at all.
//
// And the primary rule is "reaches no door", which needs no list to maintain:
// a new door is a new package and is covered by the same sentence. The SDK
// prefixes below are the backstop for a direct import that skips the doors.
func TestTheAuthDoorsStayOptional(t *testing.T) {
	// The core reaches no door and no chain: it is what a consumer imports to
	// read links, and reading links must cost nothing.
	// The whole module, not the one package: instancestest is a non-test
	// package in it and is the obvious home for a future shared helper, so a
	// door import from there has to be caught by this rule too.
	for _, dep := range deps(t, modulePrefix+"pkg/instances/...") {
		for _, door := range authDoors {
			if dep == door {
				t.Errorf("the pkg/instances module reaches %s: every consumer of the core would carry that door's SDK", dep)
			}
		}
		for _, sdk := range expensiveSDKs {
			if strings.HasPrefix(dep, sdk) {
				t.Errorf("the pkg/instances module reaches %s: that chain belongs behind an auth door", dep)
			}
		}
	}

	// Both halves of the coordinate lookup reach the Google door on purpose — a
	// Composer URL lookup speaks to Google's own API — so only AWS is out of
	// bounds here. This is the case that split the doors apart: it wanted three
	// Google helpers and was linking eighty-eight AWS packages to get them.
	//
	// pkg/instancelocate is the half a consumer outside this repo imports, so it
	// carries the same rule: Astro Desktop adopts it for Composer and must not
	// inherit MWAA's SDK by doing so.
	// The whole module in each case, not the one package: a sub-package is
	// where a future shared helper lands (pkg/instances grew instancestest
	// exactly so), and an import from there has to be caught too.
	//
	// google.golang.org/api is banned alongside AWS. cloud.google.com and
	// oauth2/google cannot be — they arrive with the Google door, which is the
	// declared exception — but the generated client is the one a future
	// Composer feature is likeliest to reach for, and composer.go's own
	// comment records declining it. It brings grpc and protobuf with it.
	for _, pkg := range []string{"internal/instancelocate/...", "pkg/instancelocate/..."} {
		for _, dep := range deps(t, modulePrefix+pkg) {
			if dep == modulePrefix+"pkg/awsauth" || strings.HasPrefix(dep, "github.com/aws/") {
				t.Errorf("%s reaches %s: the Composer lookup must not carry MWAA's SDK", pkg, dep)
			}
			if strings.HasPrefix(dep, "google.golang.org/api/") {
				t.Errorf("%s reaches %s: one GET for one field does not earn a generated client, "+
					"which brings grpc and protobuf with it", pkg, dep)
			}
		}
	}
}

// lintSubmodules reads the modules `make lint-submodules` runs the linter over.
// The Makefile is the single source: CI, the two tests below and a developer
// typing the target all have to agree on the list, and parsing it is how they do.
func lintSubmodules(t *testing.T, root string) []string {
	t.Helper()
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	var line string
	for _, l := range strings.Split(string(makefile), "\n") {
		if strings.HasPrefix(l, "LINT_SUBMODULES=") {
			line = strings.TrimPrefix(l, "LINT_SUBMODULES=")
			break
		}
	}
	if line == "" {
		t.Fatal("no LINT_SUBMODULES in the Makefile: this test reads it to know which modules are held to v2 lint standards")
	}
	mods := strings.Fields(line)
	if len(mods) == 0 {
		t.Fatal("LINT_SUBMODULES is empty")
	}
	return mods
}

// A pkg/* module is one golangci-lint cannot reach on its own: a root run does
// not descend into a nested module, so being named in LINT_SUBMODULES is the
// only thing that lints it. Ten were missing at once, long enough that the
// Makefile grew a comment explaining the backlog — which is what a hand-kept
// list does. An eleventh fails here instead, on the commit that adds it.
func TestEveryPkgSubmoduleIsLinted(t *testing.T) {
	root := repoRoot(t)
	listed := make(map[string]bool)
	for _, m := range lintSubmodules(t, root) {
		listed[m] = true
	}
	entries, err := os.ReadDir(filepath.Join(root, "pkg"))
	if err != nil {
		t.Fatalf("read pkg/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mod := "pkg/" + e.Name()
		if _, err := os.Stat(filepath.Join(root, mod, "go.mod")); err != nil {
			continue // not a module of its own; the root run covers it
		}
		if !listed[mod] {
			t.Errorf("%s is a module of its own and is not in LINT_SUBMODULES, so nothing lints it: "+
				"a root golangci-lint run does not descend into a nested module. Add it to LINT_SUBMODULES "+
				"in the Makefile, and to v2BelowCmd above", mod)
		}
	}
}

// Every module we lint to v2 standards is also held to the no-print rule.
//
// The gap this closes: accountablePackages walks `internal` only, so while this
// package lived at internal/checks, deleting its v2BelowCmd entry failed
// TestEveryInternalPackageIsAccountedFor. Promoted to pkg/checks, deleting the
// entry silently disabled TestV2PackagesBelowCmdNeverPrintOrExit for it and no
// test objected — and forbidigo cannot backstop it, since .golangci.yml anchors
// that rule at ^internal/.
//
// LINT_SUBMODULES is the right set to tie this to: it is exactly the list of
// sub-modules we have decided to hold to v2 lint standards, so a module joining
// it and not the no-print rule is an oversight rather than a decision.
//
// Deliberately NOT "every pkg/ directory": many name no rule at all, most of
// them v1 helpers that print by design (ansi, printutil, spinner). Classifying
// those is worth doing and is its own change. Every v2 sub-module is already
// here, because TestEveryPkgSubmoduleIsLinted puts it in LINT_SUBMODULES and
// this test then requires it here.
func TestEveryLintedSubmoduleIsHeldToTheNoPrintRule(t *testing.T) {
	root := repoRoot(t)
	registered := make(map[string]bool, len(v2BelowCmd))
	for _, p := range v2BelowCmd {
		registered[p] = true
	}
	for _, m := range lintSubmodules(t, root) {
		if !registered[m] {
			t.Errorf("%s is in LINT_SUBMODULES but not in v2BelowCmd, so nothing enforces that it does not print "+
				"or exit: a root golangci-lint run does not descend into a sub-module, and forbidigo is anchored "+
				"at ^internal/. Add it to v2BelowCmd", m)
		}
	}
}

// deps lists everything a package links, transitively.
//
// Stderr is captured rather than discarded: every way this can fail exits 1,
// and the ExitError on its own renders all of them as "exit status 1".
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = repoRoot(t)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v: %s", pkg, err, strings.TrimSpace(stderr.String()))
	}
	return strings.Fields(string(out))
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
