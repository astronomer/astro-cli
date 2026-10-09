package cmd

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// stubHoustonAt answers every construction-time Houston call offline, with
// every feature flag on, reporting platform version version — which decides
// the commands, flags and examples cmd/apc's version gates admit.
//
// The flags matter more than they look: cmd/apc mounts `deployment runtime`
// only when AstroRuntimeEnabled and `deployment logs triggerer` only when
// TriggererEnabled, so the APC tree's shape depends on what this returns. A
// mock with the zero AppConfig builds a smaller tree and a tree-wide test over
// it silently covers less than it claims to.
func stubHoustonAt(t *testing.T, version string) *houston_mocks.ClientInterface {
	t.Helper()
	client := new(houston_mocks.ClientInterface)
	client.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment:  true,
			HardDeleteDeployment:   true,
			ManualNamespaceNames:   true,
			TriggererEnabled:       true,
			GitSyncEnabled:         true,
			NamespaceFreeFormEntry: true,
			BYORegistryEnabled:     true,
			AstroRuntimeEnabled:    true,
			DagOnlyDeployment:      true,
		},
	}, nil)
	client.On("GetPlatformVersion", nil).Return(version, nil)
	return client
}

// newestAPCVersion is a platform version at or above every version gate in
// cmd/apc, so the tree built against it holds every command, flag and example
// a gate can add. TestAPCTreesStraddleEveryGate keeps it there.
const newestAPCVersion = "2.1.0"

// oldestAPCVersion is a platform version below every version gate in cmd/apc,
// so the tree built against it holds what each gate's other branch shows: the
// pre-1.0.0 create examples, workspace switch without pagination. Between them
// the two trees build both sides of every gate.
// TestAPCTreesStraddleEveryGate keeps it there too.
const oldestAPCVersion = "0.27.0"

// treeUnderTest is one fully assembled root, built for one configuration.
type treeUnderTest struct {
	// name labels the tree in a failure: "astro hosted", "apc 2.1.0".
	name string
	// platform is cloudPlatform or apcPlatform. Lists that excuse commands
	// (lacksOutputFlag) are keyed by it, and apply to every tree it names: the
	// configurations of one platform differ by a few gated commands and flags,
	// not by which commands exist.
	platform string
	root     *cobra.Command
	// execute is whether a test may run the tree, not only read it; see
	// treeConfigs.
	execute bool
}

// treesToExecute is the trees rootsUnderTest builds that a test may run.
func treesToExecute(t *testing.T) []treeUnderTest {
	t.Helper()
	var trees []treeUnderTest
	for _, tree := range rootsUnderTest(t) {
		if tree.execute {
			trees = append(trees, tree)
		}
	}
	return trees
}

// treeConfig is one configuration a tree-wide test builds a tree for.
type treeConfig struct {
	name       string
	platform   string
	hosted     bool
	apcVersion string
	execute    bool
}

// treeConfigs is every configuration the tree-wide tests build, and what sets
// each one up. Construction decides parts of the tree from the config file and
// from Houston, not from rootOptions alone, so each configuration says what it
// needs of both:
//
//   - whether the organization is hosted: organization.IsOrgHosted reads the
//     current context's product, and cmd/astro registers deployment create
//     and update's hosted-only flags (--cloud-provider, --development-mode,
//     ...) only for one;
//   - whether the context is APC: cmd/apc offers deploy --dags only outside a
//     cloud context, so every APC tree is built under an APC one;
//   - the APC platform version Houston reports, which decides every
//     version-gated command (deployment adopt/unadopt), flag (create --mode)
//     and example. newestAPCVersion and oldestAPCVersion between them build
//     both sides of every gate; 1.0.0 is a version in between.
//
// Most tree-wide tests only read a tree: its commands, flags and help. A test
// that executes one (problemkind's) runs code that reads state outside it —
// cmd/apc's package-level client, app config and platform version, which
// apcCmd.AddCmds sets on every build, and the config file — and after
// rootsUnderTest returns that state is the last-built APC tree's and the
// Astro platform's. Only the trees marked execute match it: APC 1.0.0, built
// last for that reason, and non-hosted Astro. Executing another would run it
// against a different version's or context's state than it was built for.
var treeConfigs = []treeConfig{
	{name: "astro", platform: cloudPlatform, execute: true},
	{name: "astro hosted", platform: cloudPlatform, hosted: true},
	{name: "apc " + newestAPCVersion, platform: apcPlatform, apcVersion: newestAPCVersion},
	{name: "apc " + oldestAPCVersion, platform: apcPlatform, apcVersion: oldestAPCVersion},
	{name: "apc 1.0.0", platform: apcPlatform, apcVersion: "1.0.0", execute: true},
}

// rootsUnderTest builds the fully assembled root for every configuration in
// treeConfigs, so a tree-wide invariant is checked against all of them. The
// alternative — calling NewRootCmd() — reads whatever context the ambient
// config holds, which is why the tree-wide tests once covered one branch per
// run and depended on test ordering to pick which.
//
// Each configuration's config is set up immediately before its tree is
// built, and a test's config is left as the Astro platform's afterwards, as
// it always was. Run alone or first under -shuffle, a tree test used to panic
// on a nil viper; setting the config here is also what fixed that.
func rootsUnderTest(t *testing.T) []treeUnderTest {
	t.Helper()
	trees := make([]treeUnderTest, 0, len(treeConfigs))
	for _, c := range treeConfigs {
		trees = append(trees, buildTree(t, c))
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	return trees
}

// buildTree sets up c's config and builds its tree. A test that runs a tree
// it builds this way can run it against that tree's own state, whichever
// configuration c is, as long as it builds no other tree in between.
func buildTree(t *testing.T, c treeConfig) treeUnderTest {
	t.Helper()
	// An Astro tree builds no APC commands, so its Houston version is
	// never read; 1.0.0 is what these stubs have always answered.
	houstonClient := stubHoustonAt(t, cmp.Or(c.apcVersion, "1.0.0"))
	if c.platform == apcPlatform {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	} else {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
	}
	if c.hosted {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			t.Fatal(err)
		}
		if err := ctx.SetContextKey("organization_product", "HOSTED"); err != nil {
			t.Fatal(err)
		}
	}
	return treeUnderTest{
		name:     c.name,
		platform: c.platform,
		execute:  c.execute,
		root: newRootCmd(rootOptions{
			platform:      c.platform,
			loggedIn:      true,
			houstonClient: houstonClient,
			out:           new(bytes.Buffer),
		}),
	}
}

// treeNamed is the tree rootsUnderTest built for the configuration called name.
func treeNamed(t *testing.T, trees []treeUnderTest, name string) *cobra.Command {
	t.Helper()
	for _, tree := range trees {
		if tree.name == name {
			return tree.root
		}
	}
	t.Fatalf("no tree named %q in treeConfigs", name)
	return nil
}

// versionGate matches a houston.VersionRestrictions field set in source:
// `GTE: "2.1.0"`, `LT: "1.0.0"`, `EQ: []string{...}`.
// TestAPCTreesStraddleEveryGate parses cmd/apc for the version gates it
// builds its trees by: every houston.VersionRestrictions literal (the
// command-availability table's entries among them), and every call to
// houston.VerifyVersionMatch. Each gate must be a GTE on a string literal that
// newestAPCVersion passes and oldestAPCVersion does not, so that between them
// the two trees build both branches of every gate — what it shows at or above
// its version, and what its else-branch or negation shows below. A gate either
// version lands on the wrong side of, an LT or EQ gate, a version given as
// anything but a literal, and a VerifyVersionMatch whose restriction this test
// cannot read all fail, so a new gate is either straddled or reported.
// Comments, strings and case labels are not gates, which is why this parses
// rather than searches.
//
// What it cannot see is a version compared some other way (semver.Compare on
// houstonVersion directly). cmd/apc gates only through these two, today.
func TestAPCTreesStraddleEveryGate(t *testing.T) {
	gates, problems := apcVersionGates(t)
	for _, problem := range problems {
		t.Error(problem)
	}
	if len(gates) == 0 {
		t.Fatal("found no version gates in cmd/apc; they are no longer written as houston.VersionRestrictions literals")
	}
	for _, g := range gates {
		if problem := checkVersionGate(g); problem != "" {
			t.Errorf("%s: %s", g.at, problem)
		}
	}
}

// versionGate is one field of a houston.VersionRestrictions literal in cmd/apc.
type versionGate struct {
	at    string
	field ast.Expr
}

// checkVersionGate names what keeps the APC trees from straddling g, or
// returns "".
func checkVersionGate(g versionGate) string {
	kv, ok := g.field.(*ast.KeyValueExpr)
	if !ok {
		return "a VersionRestrictions field without a name"
	}
	if key, _ := kv.Key.(*ast.Ident); key == nil || key.Name != "GTE" {
		return fmt.Sprintf("gate %v is not a GTE; the newest and oldest APC trees only straddle a GTE", kv.Key)
	}
	lit, ok := kv.Value.(*ast.BasicLit)
	if !ok || lit.Kind != gotoken.STRING {
		return fmt.Sprintf("gate GTE gives its version as %T, not a string literal this test can read", kv.Value)
	}
	version, err := strconv.Unquote(lit.Value)
	if err != nil {
		return err.Error()
	}
	gate := houston.VersionRestrictions{GTE: version}
	switch {
	case !houston.VerifyVersionMatch(newestAPCVersion, gate):
		return fmt.Sprintf("gate GTE %q is above newestAPCVersion %s; raise it", version, newestAPCVersion)
	case houston.VerifyVersionMatch(oldestAPCVersion, gate):
		return fmt.Sprintf("gate GTE %q is at or below oldestAPCVersion %s; lower it", version, oldestAPCVersion)
	}
	return ""
}

// apcVersionGates parses cmd/apc's non-test files and returns the fields of
// every houston.VersionRestrictions literal in them, and a problem for each
// version check it cannot read.
func apcVersionGates(t *testing.T) (gates []versionGate, problems []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("apc", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := gotoken.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if problem := unreadableVersionCheck(n); problem != "" {
					problems = append(problems, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), problem))
				}
			case *ast.CompositeLit:
				for _, restriction := range versionRestrictions(n) {
					for _, field := range restriction.Elts {
						gates = append(gates, versionGate{at: fset.Position(field.Pos()).String(), field: field})
					}
				}
			}
			return true
		})
	}
	return gates, problems
}

// isHoustonName reports whether e is houston.<name>.
func isHoustonName(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "houston"
}

// versionRestrictions is lit if it is a houston.VersionRestrictions literal,
// or the entries of lit if it is a map of them — a table of gates, whose
// entries elide their type.
func versionRestrictions(lit *ast.CompositeLit) []*ast.CompositeLit {
	if isHoustonName(lit.Type, "VersionRestrictions") {
		return []*ast.CompositeLit{lit}
	}
	table, ok := lit.Type.(*ast.MapType)
	if !ok || !isHoustonName(table.Value, "VersionRestrictions") {
		return nil
	}
	var entries []*ast.CompositeLit
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if entry, ok := kv.Value.(*ast.CompositeLit); ok {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

// unreadableVersionCheck names what is wrong with a VerifyVersionMatch call
// whose restriction this test cannot read, or returns "" for any other call.
// A literal is checked where it is written; a variable is how a table of
// gates is read (the command-availability map), whose entries are checked
// where it is written.
func unreadableVersionCheck(call *ast.CallExpr) string {
	if !isHoustonName(call.Fun, "VerifyVersionMatch") || len(call.Args) != 2 {
		return ""
	}
	switch arg := call.Args[1].(type) {
	case *ast.CompositeLit:
		if !isHoustonName(arg.Type, "VersionRestrictions") {
			return fmt.Sprintf("VerifyVersionMatch's restriction is a %T literal, not a houston.VersionRestrictions", arg.Type)
		}
	case *ast.Ident:
	default:
		return fmt.Sprintf("VerifyVersionMatch's restriction is a %T, which this test cannot read; write it as a houston.VersionRestrictions literal or read it from a table of them", arg)
	}
	return ""
}

// TestEveryTreeBuildsWhatItIsFor pins one thing each configuration in
// treeConfigs exists for, so a change to how construction decides (what makes
// an organization hosted, what makes a context APC, which version a gate
// reads) cannot quietly turn one tree into a copy of another.
func TestEveryTreeBuildsWhatItIsFor(t *testing.T) {
	roots := rootsUnderTest(t)
	hosted := treeNamed(t, roots, "astro hosted")
	newestAPC := treeNamed(t, roots, "apc "+newestAPCVersion)
	oldestAPC := treeNamed(t, roots, "apc "+oldestAPCVersion)
	if c, _, err := treeNamed(t, roots, "astro").Find([]string{"deployment", "create"}); err != nil || c.Flags().Lookup("development-mode") != nil {
		t.Error("the non-hosted Astro tree's deployment create has --development-mode; it is being built for a hosted organization")
	}
	hasFlag := func(root *cobra.Command, path []string, flag string) bool {
		c, _, err := root.Find(path)
		return err == nil && c != root && c.Flags().Lookup(flag) != nil
	}
	for _, sub := range []string{"create", "update"} {
		if !hasFlag(hosted, []string{"deployment", sub}, "development-mode") {
			t.Errorf("the hosted tree's deployment %s has no --development-mode; it is not being built for a hosted organization", sub)
		}
	}
	if !hasFlag(newestAPC, []string{"deployment", "create"}, "mode") {
		t.Errorf("the newest APC tree's deployment create has no --mode; it is not being built at %s", newestAPCVersion)
	}
	if !hasFlag(newestAPC, []string{"deploy"}, "dags") {
		t.Error("the newest APC tree's deploy has no --dags; it is not being built under an APC context")
	}
	if c, _, err := newestAPC.Find([]string{"deployment", "adopt"}); err != nil || c.Hidden || c.Name() != "adopt" {
		t.Errorf("the newest APC tree has no visible deployment adopt; it is not being built at %s", newestAPCVersion)
	}
	// Below 1.0.0 deployment create shows the examples without --cluster-id,
	// which only the oldest tree builds.
	if c, _, err := oldestAPC.Find([]string{"deployment", "create"}); err != nil || c.Name() != "create" ||
		strings.Contains(c.Example, "--cluster-id") || !strings.Contains(c.Example, "--nfs-location") {
		t.Errorf("the oldest APC tree's deployment create does not show the pre-1.0.0 examples; it is not being built at %s", oldestAPCVersion)
	}
}
