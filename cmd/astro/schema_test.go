package astro

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/workerqueue"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	"github.com/astronomer/astro-cli/internal/platform/astro/ide"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	roleClient "github.com/astronomer/astro-cli/internal/platform/astro/role"
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
	// astro deployment user|team remove: the same removal, naming the
	// Deployment. Its add and update publish the user and team above, with
	// their deployment_role set.
	{Name: "deployment-user-removal", Value: user.DeploymentRemoval{}},
	{Name: "deployment-team-removal", Value: team.DeploymentRemoval{}},
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
	// The writes publish those same shapes: a `set` the object as it now
	// is and a `delete` the object as it was (env-object), a `link set` or
	// `link delete` the links as it left them (the two link reports), and
	// `variable export` the variable list. `set --from-file` is the one of
	// its own: what it did with each key of the file.
	{Name: "env-set-from-file", Value: env.SetFromFileResult{}},
	// A variable's value, and a link's override of it, is null when hidden
	// (secret, and --include-secrets not given, or any write) and the value
	// itself otherwise, "" included. Filling shows only the shown branch, so
	// both are also pinned as given; the objects go through NewObjectInfo, the
	// conversion every env json path uses.
	{Name: "env-object-secret", Value: env.NewObjectInfo(&astrov1.EnvironmentObject{
		ObjectKey: "KEY", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: "WS", SetFields: []string{"value"},
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{IsSecret: true, Value: "never published"},
		Links: &[]astrov1.EnvironmentObjectLink{{
			Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: "DEP", SetFields: []string{"value"},
			EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: "never published"},
		}},
	}, false), AsGiven: true},
	{Name: "env-object-empty-value", Value: env.NewObjectInfo(&astrov1.EnvironmentObject{
		ObjectKey: "KEY", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: "WS", SetFields: []string{},
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: ""},
		Links: &[]astrov1.EnvironmentObjectLink{{
			Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: "DEP", SetFields: []string{},
			EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: ""},
		}},
	}, false), AsGiven: true},
	// The variable link report says the same: workspace_value null when
	// hidden, and override_value absent for a link with no override, null for
	// one whose value is hidden, and the value otherwise.
	{Name: "env-variable-link-list-secret", Value: env.VarLinksReport{
		ObjectKey: "KEY", ObjectID: "ID", IsSecret: true, ExcludeLinks: []string{},
		Links: []env.VarLink{{DeploymentID: "DEP", OverrideValue: schemaPtr[*string](nil)}, {DeploymentID: "DEP2"}},
	}, AsGiven: true},
	{Name: "env-variable-link-list-empty-value", Value: env.VarLinksReport{
		ObjectKey: "KEY", ObjectID: "ID", WorkspaceValue: schemaPtr(""), ExcludeLinks: []string{},
		Links: []env.VarLink{{DeploymentID: "DEP", OverrideValue: schemaPtr(schemaPtr(""))}},
	}, AsGiven: true},

	// astro deploy --output json, in a project with a manifest: the one
	// object a finished deploy prints.
	{Name: "deploy", Value: deployJSON{}},
	// astro dbt deploy: what it deployed where. delete: the bundle it
	// removed. cleanup: the artifacts it removed and kept.
	{Name: "dbt-deploy", Value: dbtDeployJSON{}},
	{Name: "dbt-delete", Value: dbtDeleteJSON{}},
	{Name: "dbt-cleanup", Value: dbtCleanupJSON{}},
	// astro remote deploy: the client image it pushed.
	{Name: "remote-deploy", Value: remoteDeployJSON{}},

	// astro deployment inspect -o json. Its keys predate the CLI's json rules
	// and are snake_case already; astronomer/deploy-action reads them, so
	// they do not move. The bytes of every inspect output are pinned too, in
	// testdata/deployment_inspect. `deployment create` and `update` -o json
	// publish the Deployment they leave in this shape too.
	{Name: "deployment-inspect", Value: inspect.FormattedDeployment{}},

	// astro deployment delete: what it deleted.
	{Name: "deployment-removal", Value: deployment.Removal{}},
	// astro deployment bundle create and update: the bundle as the change
	// left it, as bundle list gives each one. delete: what it deleted.
	{Name: "deployment-bundle", Value: deployment.BundleInfo{}},
	{Name: "deployment-bundle-removal", Value: deployment.BundleRemoval{}},
	// astro deployment hibernate and wake-up: the override the Deployment now
	// has, which is null after --remove-override.
	{Name: "deployment-hibernation", Value: deployment.HibernationResult{}},
	// astro deployment worker-queue create, update and delete: the queue
	// as the change left it, or as it was before a delete.
	{Name: "deployment-worker-queue", Value: workerqueue.Result{}},
	// astro deployment logs -o json: one of these per line, a stream.
	{Name: "deployment-log-entry", Value: deployment.LogEntry{}},

	// astro workspace create, update and switch: the one Workspace, as
	// `workspace list` shows it. delete: what it deleted.
	{Name: "workspace", Value: workspace.WorkspaceInfo{}},
	{Name: "workspace-removal", Value: workspace.Removal{}},
	// astro organization switch: the Organization and the Workspace the run
	// left current, each as its list shows one; the Workspace is null when
	// none of the Organization's is current.
	{Name: "organization-switch", Value: organization.SwitchResult{}},
	// astro organization role list.
	{Name: "organization-role-list", Value: roleClient.RoleList{}},
	// astro organization audit-logs export: the file it wrote.
	{Name: "organization-audit-logs-export", Value: organization.AuditLogExport{}},
	// astro ide project list, import and export: the projects, and what an
	// import or an export moved.
	{Name: "ide-project-list", Value: ide.ProjectList{}},
	{Name: "ide-project-import", Value: ide.Import{}},
	{Name: "ide-project-export", Value: ide.Export{}},
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
// would be silent. Fifty-two shapes reach Emit in this package's tests
// today: the deployment variable results (2) and the six API token results;
// the deployment, workspace and organization user and team results, with the
// organization's invite and team membership (10); the pkg/output lists their
// tests reach (deployment, bundle, workspace, organization, cluster: 5); the
// `astro env` payloads, reads and writes (8); the manifest deploy's result,
// deployment inspect's deployment (which create and update publish too),
// delete's removal, hibernate's override, the worker-queue result and the
// log entry (6); the bundle a bundle create or update leaves and what a
// bundle delete did (2); the dbt deploy, delete and cleanup results and the remote
// deploy's pushed image (4); the Workspace a create, update or switch
// publishes and what a Workspace delete did, what an Organization switch left
// current, the role list and the file an audit-log export wrote (5); the
// Astro IDE project list and what an import or an export moved (3); and the
// error object.
// Raise it as conversions land; lower it only saying why.
const minWatchedPayloads = 52

// pinnedElsewhere names the shapes that reach Emit here and are pinned by
// another tree's goldens, keyed by type, with where. Pinning one twice would
// give two goldens to keep in step for one contract.
var pinnedElsewhere = map[reflect.Type]string{
	// The failure object cliout.Execute publishes for every command, whichever
	// tree it is in.
	reflect.TypeOf(cliout.ErrorObject{}): "cmd/local/testdata/schema/error.json",
}

// emitWatch is this package's configuration of the observer TestMain arms.
func emitWatch() cliouttest.Watch {
	return cliouttest.Watch{
		Cases:           publishedPayloads,
		PinnedElsewhere: pinnedElsewhere,
		Floor:           minWatchedPayloads,
		File:            "cmd/astro/schema_test.go",
	}
}

// Every key a golden here publishes is snake_case: a capital in one is a Go
// field name that reached the wire because a tag was forgotten.
func TestPublishedKeysAreSnakeCase(t *testing.T) {
	assert.Empty(t, cliouttest.KeyProblems(t, schemaDir, len(publishedPayloads) > 0))
}

// schemaPtr is a pointer to v, for the cases pinned as given.
func schemaPtr[T any](v T) *T { return &v }
