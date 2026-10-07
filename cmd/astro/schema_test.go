package astro

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
)

// The shapes the Astro command tree publishes under `--output json`, each
// pinned by a golden in testdata/schema, the same way cmd/local pins its own
// (see cliouttest). `make update-schemas` rewrites them; read the diff before
// committing it, since a changed golden is a changed contract.
//
// The goldens pin the shape once. A command's own tests then decode what it
// printed and assert what it means — which outcome each input got, which
// variables are listed — rather than restating these bytes.

// schemaDir holds this tree's goldens.
var schemaDir = filepath.Join("testdata", "schema")

// publishedPayloads is every shape a command in this tree writes under
// `--output json`. Named for the surface rather than the Go type, because the
// type is an implementation detail and the command is what somebody depends
// on.
var publishedPayloads = []cliouttest.Case{
	// astro deployment variable list, create, update.
	{Name: "deployment-variable-list", Value: deployment.DeploymentVariables{}},
	{Name: "deployment-variable-modify", Value: deployment.VariableModifyResult{}},
	// VariableInfo's MarshalJSON publishes value as a string, or as null for
	// a secret. Filling can reach only the secret branch, which is what the
	// two goldens above show, so each branch is also pinned as given.
	{Name: "deployment-variable", Value: deployment.VariableInfo{Key: "KEY", Value: "value"}, AsGiven: true},
	{Name: "deployment-variable-secret", Value: deployment.VariableInfo{Key: "KEY", Value: "never published", IsSecret: true}, AsGiven: true},

	// The API token families: a list, the token a create, rotate, update or
	// add-role leaves (with its secret only after a create or a rotate), and
	// what a delete or a remove-role did. `deployment token`, `workspace
	// token` and `organization token` publish the one token and the one list,
	// pinned once under the names the deployment family gave them first; each
	// family's removal names its own object, so each has its own golden.
	{Name: "deployment-token-list", Value: apitoken.List{}},
	{Name: "deployment-token", Value: apitoken.Token{}},
	{Name: "deployment-token-removal", Value: apitoken.DeploymentRemoval{}},
	{Name: "workspace-token-removal", Value: apitoken.WorkspaceRemoval{}},
	{Name: "organization-token-removal", Value: apitoken.OrganizationRemoval{}},
	// astro organization token roles.
	{Name: "organization-token-roles", Value: apitoken.RoleList{}},

	// The lists pkg/output renders, which hand their result to the command's
	// cliout.Renderer. One golden per type: `deployment user list`,
	// `workspace user list` and `organization user list` publish the same
	// UserList, and likewise the team lists.
	{Name: "deployment-list", Value: deployment.DeploymentList{}},
	{Name: "deployment-bundle-list", Value: deployment.BundleList{}},
	{Name: "workspace-list", Value: workspace.WorkspaceList{}},
	{Name: "organization-list", Value: organization.OrganizationList{}},
	{Name: "organization-cluster-list", Value: organization.ClusterList{}},
	{Name: "user-list", Value: user.UserList{}},
	{Name: "team-list", Value: team.TeamList{}},

	// The user and team commands of `workspace` and `organization`. An add or
	// an update publishes the one user or team its list holds, with the role
	// it now has on the object the command is about; a create or an update of
	// a team, the team as it now is. A remove or a delete names what it acted
	// on and what it did, as the token families' removals do.
	{Name: "user", Value: user.UserInfo{}},
	{Name: "team", Value: team.TeamInfo{}},
	{Name: "workspace-user-removal", Value: user.WorkspaceRemoval{}},
	{Name: "workspace-team-removal", Value: team.WorkspaceRemoval{}},
	{Name: "organization-team-removal", Value: team.OrganizationRemoval{}},
	// astro organization user invite.
	{Name: "user-invite", Value: user.Invite{}},
	// astro organization team user add|remove, and list.
	{Name: "team-membership", Value: team.Membership{}},
	{Name: "team-member-list", Value: team.MemberList{}},

	// astro env: internal/platform/astro/env renders the text and hands the
	// result to the command's cliout.Renderer, as pkg/output does. The four
	// per-kind lists carry the same object under different keys, and the key
	// is the contract, so each list is its own golden. `get` on every kind
	// publishes the one object; `variable link list` publishes its own link
	// report, and `connection link list` and `airflow-variable link list`
	// share the other.
	{Name: "env-list", Value: env.InventoryList{}},
	{Name: "env-variable-list", Value: env.VariableList{}},
	{Name: "env-connection-list", Value: env.ConnectionList{}},
	{Name: "env-airflow-variable-list", Value: env.AirflowVariableList{}},
	{Name: "env-metrics-export-list", Value: env.MetricsExportList{}},
	{Name: "env-object", Value: env.ObjectInfo{}},
	{Name: "env-variable-link-list", Value: env.VarLinksReport{}},
	{Name: "env-link-list", Value: env.LinksReport{}},

	// astro deploy --output json, in a project with a manifest: the one
	// object a finished deploy prints.
	{Name: "deploy", Value: deployJSON{}},

	// astro deployment inspect -o json, and -o json --template, which is the
	// same shape with metadata dropped. Its keys predate the CLI's json rules
	// and are snake_case already; the --deployment-file round trip and
	// astronomer/deploy-action read them, so they do not move. The bytes of
	// every inspect output are pinned too, in testdata/deployment_inspect.
	{Name: "deployment-inspect", Value: inspect.FormattedDeployment{}},
}

func TestPublishedJSONPayloadsKeepTheirShape(t *testing.T) {
	for _, c := range publishedPayloads {
		t.Run(c.Name, func(t *testing.T) { cliouttest.Check(t, schemaDir, c) })
	}
}

func TestEveryGoldenHasACase(t *testing.T) {
	assert.Empty(t, cliouttest.Orphans(t, schemaDir, publishedPayloads),
		"these goldens have no case in publishedPayloads, so nothing regenerates\n"+
			"them and nothing would notice the shape they pin going stale. If the\n"+
			"payload is gone, delete the file; if a case was renamed, delete the\n"+
			"file the old name left behind.")
}

// What the commands actually emit, watched at the door.
//
// The list above is hand-written, and the commands in lacksOutputFlag gain
// `-o json` a family at a time, so it is the list most likely to fall behind:
// a conversion that publishes a new type and forgets to pin it passes every
// test it wrote. TestMain arms cliout.EmitObserver for the package run, and
// fails it when a named type reached Emit that no golden pins, or an
// anonymous struct reached it at all, since that shape cannot be pinned.
//
// Blind to what the suite does not run. pkg/output's lists and the `astro
// env` writers pass through Emit too, since they take the command's Renderer.
// cmd/local's emitrecord_test.go does the same for its tree, with the
// reasoning at length.

// minWatchedPayloads is a floor under the tally, not a target: an empty tally
// reads exactly like a clean one, so without it the observer coming unwired
// would be silent. Thirty shapes reach Emit in this package's tests today:
// the deployment variable and token results, the workspace and organization
// token, user and team results, the pkg/output lists and the `astro env`
// payloads their tests reach, the manifest deploy's result, deployment
// inspect's deployment, and the error object. Raise it as conversions land;
// lower it only saying why.
const minWatchedPayloads = 30

// pinnedElsewhere names the shapes that reach Emit here and are pinned by
// another tree's goldens, keyed by type, with where. Pinning one twice would
// give two goldens to keep in step for one contract.
var pinnedElsewhere = map[reflect.Type]string{
	// The failure object cliout.Execute publishes for every command, whichever
	// tree it is in.
	reflect.TypeOf(cliout.ErrorObject{}): "cmd/local/testdata/schema/error.json",
}

var emitted = struct {
	sync.Mutex
	named     map[reflect.Type]bool
	anonymous map[string]bool
}{
	named:     map[reflect.Type]bool{},
	anonymous: map[string]bool{},
}

func recordEmitted(v any) {
	if v == nil {
		return
	}
	t := cliouttest.PayloadType(reflect.TypeOf(v))
	if t == nil {
		return
	}
	emitted.Lock()
	defer emitted.Unlock()
	if t.Name() == "" {
		emitted.anonymous[t.String()] = true
		return
	}
	emitted.named[t] = true
}

// watchEmit arms the observer and returns what reports on it after the run.
func watchEmit() (problems func() []string) {
	cliout.EmitObserver = recordEmitted
	return func() []string {
		emitted.Lock()
		defer emitted.Unlock()
		return emitProblems(emitted.named, emitted.anonymous, minWatchedPayloads)
	}
}

// emitProblems is the report over a tally handed in, so it can be tested
// directly: in a clean tree every branch is silent, which is the state that
// lets a mutation to one of them survive.
func emitProblems(named map[reflect.Type]bool, anonymous map[string]bool, floor int) []string {
	pinned := map[reflect.Type]bool{}
	for _, c := range publishedPayloads {
		if t := cliouttest.PayloadType(reflect.TypeOf(c.Value)); t != nil {
			pinned[t] = true
		}
	}

	var out []string
	if len(named) < floor {
		out = append(out, fmt.Sprintf(
			"only %d payload shapes were seen reaching cliout.Renderer.Emit, below the floor of %d.\n"+
				"    Either the observer is no longer wired up, in which case the check\n"+
				"    below is passing on an empty tally, or command tests stopped running.\n"+
				"    If the drop is real and intended, lower minWatchedPayloads and say why.",
			len(named), floor))
	}
	for t := range named {
		if !pinned[t] && pinnedElsewhere[t] == "" {
			out = append(out, fmt.Sprintf(
				"a command passed %s through cliout.Renderer.Emit and no golden pins it.\n"+
					"    Add it to publishedPayloads in cmd/astro/schema_test.go and run\n"+
					"    `make update-schemas`; a shape that reaches stdout is a contract.", t))
		}
	}
	for name := range anonymous {
		out = append(out, fmt.Sprintf(
			"a command passed the anonymous struct %s through cliout.Renderer.Emit.\n"+
				"    An anonymous shape cannot be pinned. Give it a name and add it to\n"+
				"    publishedPayloads.", name))
	}
	sort.Strings(out)
	return out
}

func TestEmitProblemsReportsWhatIsUnpinned(t *testing.T) {
	type notPinned struct {
		A string `json:"a"`
	}
	got := emitProblems(
		map[reflect.Type]bool{reflect.TypeOf(notPinned{}): true},
		map[string]bool{"struct { A int }": true},
		99,
	)
	assert.Len(t, got, 3, "the floor, the named type and the anonymous one")

	pinned := map[reflect.Type]bool{reflect.TypeOf(apitoken.Token{}): true}
	assert.Empty(t, emitProblems(pinned, nil, 1), "a pinned type, above the floor")
}
