package local

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/apirequest"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// publishedPayloads is every shape a v2 command writes under `--output
// json`. Named for the surface rather than the Go type, because the type is
// an implementation detail and the command is what somebody depends on.
//
// Two of them come from other modules: `astro init` publishes
// scaffold.Result and `astro local start|status` publishes localrt.Status,
// so a change over there is a change to this CLI's contract and belongs in
// the same net.
var publishedPayloads = []schemaCase{
	{"init", scaffold.Result{}},
	{"local-status", localrt.Status{}},
	{"local-list-row", listRow{}},
	{"local-reset", localrt.ResetReport{}},
	{"local-upgrade-airflow", airflowUpgrade{}},
	{"check-summary", checkSummary{}},
	{"dev-removed", devRemoved{}},
	{"env-result", envResult{}},
	{"env-value", envValue{}},
	{"env-link-result", envLinkResult{}},
	{"env-declaration", envDeclarationResult{}},
	// `astro local env list --output json` streams one of these per value,
	// then the whole slice in text mode. Found by recording what Emit
	// actually receives during the command tests: it is declared in
	// internal/localenv, so the AST guard over this directory cannot see
	// it, and it was missed by eye too.
	{"env-list-item", localenv.ListItem{}},
	{"health-report", healthReport{}},
	{"health-version", healthVersion{}},
	{"health-import-errors", healthImportErrors{}},
	{"health-dag-warnings", healthDAGWarnings{}},
	{"health-dag-stats", healthDAGStats{}},
	{"health-version-row", versionRow{}},
	{"health-import-error-row", importErrorRow{}},
	{"health-dag-warning-row", dagWarningRow{}},
	{"dag-row", dagRow{}},
	{"dag-source", dagSource{}},
	{"dag-stat-row", dagStatRow{}},
	{"dag-exploration", dagExploration{}},
	{"asset-row", assetRow{}},
	{"asset-event-row", assetEventRow{}},
	{"connection-list-row", connectionListRow{}},
	{"connection-row", connectionRow{}},
	{"variable-list-row", variableListRow{}},
	{"variable-row", variableRow{}},
	{"pool-row", poolRow{}},
	{"provider-row", providerRow{}},
	{"plugin-row", pluginRow{}},
	{"config-option-row", configOptionRow{}},
	{"run-row", runRow{}},
	{"run-triggered", triggeredRun{}},
	{"run-deleted", deletedRun{}},
	{"run-cleared", clearedRun{}},
	{"run-waited", waitedRun{}},
	{"run-diagnosis", runDiagnosis{}},
	{"run-summary", runSummary{}},
	{"task-row", taskRow{}},
	{"task-instance-row", taskInstanceRow{}},
	{"task-log", taskLog{}},
	{"tasks-cleared", clearedTasks{}},
	{"use-result", useResult{}},
	{"use-listing", useListing{}},
	{"use-link-row", useLinkRow{}},
	{"link-result", linkResult{}},
	{"error", jsonError{}},
	{"event", event{}},
	{"open-url", urlResult{}},
	{"check-blocked", checkBlocked{}},
	{"api-exchange", apiExchange{}},

	// Published from other packages. A guard that reads this directory
	// cannot see them, and they are contracts all the same: `astro local
	// check --output json` streams checks.Finding per finding, and
	// MissingPayload's own doc says it is structured for a coding agent to
	// act on — which is the audience least able to cope with it moving.
	{"check-finding", checks.Finding{}},
	{"check-target-report", checks.TargetReport{}},
	{"package-result", pack.Result{}},
	{"start-missing-env", plan.MissingPayload{}},
	// `astro local api ls --output json`, one per endpoint. The row is
	// apirequest's because `astro api airflow ls --json` prints the same one.
	{"api-endpoint-row", apirequest.EndpointRow{}},
}

func TestPublishedJSONPayloadsKeepTheirShape(t *testing.T) {
	for _, c := range publishedPayloads {
		t.Run(c.name, func(t *testing.T) { checkSchema(t, c) })
	}
}

// A payload nobody pinned is a contract nobody is holding.
//
// The list above is hand-written, so it rots the moment a command grows a
// new shape — and the rot is invisible, because the tests that exist keep
// passing. This reads the package back and fails when a struct carrying
// json tags is not covered, which is the same reason the engines each got
// their own wiring test: a rule's own tests pass whether or not anything
// uses it.
// notPublished are types that carry json tags and never reach stdout, with
// the reason each one is exempt. The guard below works off json tags, which
// is a proxy for "is published" and not the thing itself: a struct tagged
// for an inbound payload, a config file, or a request body is not a
// contract this CLI owes anybody.
//
// A named exemption rather than a cleverer rule. Whoever adds one has to
// write down why, and whoever reads it later can check the claim — which is
// the part a heuristic cannot do.
var notPublished = map[string]string{
	"fileStore": "an adapter around localenv.Store; embeds it for the methods, never marshaled",
	"query":     "a command receiver carrying *cli and its target; never marshaled",
	"reachJSON": "never emitted alone; nested in env-value and env-link-result, whose goldens pin it",
	"reachPath": "never emitted alone; nested in reachJSON, pinned by the same goldens",
}

func TestEveryJSONPayloadTypeIsPinned(t *testing.T) {
	// Only the entries declared in this package count towards covering a
	// name found here. scaffold.Result, localrt.Status and checks.Finding
	// register the bare names Result, Status and Finding, and a local type
	// that later took one of those names would be waved through unpinned
	// with no diagnostic.
	const localPkg = "github.com/astronomer/astro-cli/cmd/local"
	covered := map[string]bool{}
	for _, c := range publishedPayloads {
		if rt := reflect.TypeOf(c.value); rt.PkgPath() == localPkg {
			covered[rt.Name()] = true
		}
	}
	for name := range notPublished {
		covered[name] = true
	}

	var missing []string
	walkStructs(t, func(name string, st *ast.StructType) {
		if hasJSONTag(st) && !covered[name] {
			missing = append(missing, name)
		}
	})

	assert.Empty(t, missing,
		"these types carry json tags and are not pinned in publishedPayloads.\n"+
			"If a command emits one, add it — a new `--output json` shape is a\n"+
			"new public contract. If it is internal and never reaches stdout,\n"+
			"add it to notPublished with a note saying why.")
}

// A golden nobody generates is a snapshot of a shape that may no longer
// exist — and e2e/schema_test.go looks these up by filename, so an orphan
// left behind by a renamed case would keep that test green against a dead
// file. Renaming a case writes the new golden and leaves the old one; this
// is what says so.
func TestEveryGoldenHasACase(t *testing.T) {
	expected := map[string]bool{}
	for _, c := range publishedPayloads {
		expected[c.name+".json"] = true
	}

	entries, err := os.ReadDir(filepath.Join("testdata", "schema"))
	require.NoError(t, err)

	var orphans []string
	for _, e := range entries {
		if !e.IsDir() && !expected[e.Name()] {
			orphans = append(orphans, e.Name())
		}
	}
	assert.Empty(t, orphans,
		"these goldens have no case in publishedPayloads, so nothing regenerates\n"+
			"them and nothing would notice the shape they pin going stale. If the\n"+
			"payload is gone, delete the file; if a case was renamed, delete the\n"+
			"file the old name left behind.")
}

// An exemption for a type that no longer exists is a note nobody will ever
// read again, and a name somebody might reuse for something that IS
// published — at which point the guard waves it through.
func TestNotPublishedExemptionsStillExist(t *testing.T) {
	declared := map[string]bool{}
	for _, name := range structTypeNames(t) {
		declared[name] = true
	}
	for name, reason := range notPublished {
		assert.True(t, declared[name],
			"notPublished exempts %q (%s), which no longer exists in this package; drop the entry",
			name, reason)
	}
}

// walkStructs visits every struct type declared in this package's non-test
// sources.
func walkStructs(t *testing.T, fn func(name string, st *ast.StructType)) {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, perr)

		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				fn(ts.Name.Name, st)
			}
			return true
		})
	}
}

func structTypeNames(t *testing.T) []string {
	t.Helper()
	var names []string
	walkStructs(t, func(name string, _ *ast.StructType) { names = append(names, name) })
	return names
}

// hasJSONTag reports whether a struct declares json tags, or embeds
// something — `type summaryRow struct { runRow }` republishes runRow's whole
// shape under a new command while declaring no tag of its own, and a guard
// that only looked at direct tags would let that ship unpinned. The three
// embedding types already in this package are caught today only because
// each happens to add a tagged field as well.
//
// Over-inclusive on purpose: a struct that embeds something for reasons
// having nothing to do with output can say so in notPublished.
func hasJSONTag(st *ast.StructType) bool {
	for _, f := range st.Fields.List {
		if f.Tag != nil && strings.Contains(f.Tag.Value, `json:"`) {
			return true
		}
		if len(f.Names) == 0 { // embedded
			return true
		}
	}
	return false
}
