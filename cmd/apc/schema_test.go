package apc

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
)

// The shapes the APC (Houston) commands publish under `--output json`, each
// pinned by a golden in testdata/schema, the same way cmd/local, cmd/astro and
// cmd pin their own (see cliouttest). `make update-schemas` rewrites the
// goldens; read the diff before committing it, since a changed golden is a
// changed contract.
//
// The goldens pin the shape once. A command's own tests then decode what it
// printed and assert what it means, rather than restating these bytes.

// schemaDir holds this package's goldens. It does not exist until the first
// case is added: nothing can be orphaned in it before then.
var schemaDir = filepath.Join("testdata", "schema")

// publishedPayloads is every shape a command built here writes under
// `--output json`, named for the surface rather than the Go type. The APC
// commands gain `-o json` a family at a time (cmd/output_flag_test.go's
// lacksOutputFlag lists the rest); each conversion adds its shapes here.
var publishedPayloads = []cliouttest.Case{
	// astro deployment create, update and adopt: the Deployment as it now is.
	{Name: "deployment", Value: deploymentJSON{}},
	// The same Deployment where Houston gave none of the optional values:
	// they are null, never "" (see deploymentJSON). Pinned as given, since
	// populating every field can show only the other branch.
	{Name: "deployment-without-values", Value: deploymentJSON{DeploymentID: "x", Label: "x", ReleaseName: "x", URLs: []deploymentURLJSON{}}, AsGiven: true},
	// astro deployment list.
	{Name: "deployment-list", Value: deploymentListJSON{}},
	// astro deployment delete and unadopt: what they removed.
	{Name: "deployment-removal", Value: deploymentRemovalJSON{}},
	// What unadopt removed when Houston answered with no record: only the id
	// given and the action are known.
	{Name: "deployment-removal-without-values", Value: deploymentRemovalJSON{DeploymentID: "x", Action: "unadopted"}, AsGiven: true},
	// astro deployment airflow upgrade, runtime upgrade and runtime migrate,
	// and their --cancel: what changed.
	{Name: "deployment-version-change", Value: versionChangeJSON{}},
	// astro deployment logs -o json: one of these per line, a stream.
	{Name: "deployment-log-entry", Value: logEntryJSON{}},
	// astro deploy: what it deployed.
	{Name: "deploy", Value: deployJSON{}},
	// astro workspace create, update and switch: the Workspace as it now is.
	{Name: "workspace", Value: workspaceJSON{}},
	// The same Workspace where Houston gave none of the optional values:
	// they are null, never "" (see workspaceJSON). Pinned as given, since
	// populating every field can show only the other branch.
	{Name: "workspace-without-values", Value: workspaceJSON{ID: "x", Label: "x"}, AsGiven: true},
	// astro workspace list.
	{Name: "workspace-list", Value: workspaceListJSON{}},
	// astro workspace delete: what it removed.
	{Name: "workspace-removal", Value: workspaceRemovalJSON{}},
	// astro deployment|workspace service-account create. api_key is the
	// account's credential, the result a script creates it for, and only a
	// create publishes it.
	{Name: "service-account", Value: serviceAccountJSON{}},
	// The same account where Houston gave none of the optional values (a
	// category never given, an account never used): they are null, never "".
	{Name: "service-account-without-values", Value: serviceAccountJSON{ID: "x", Label: sp("x")}, AsGiven: true},
	// astro deployment|workspace service-account list: api_key is always
	// null. Houston returns the key whole for ten minutes after the create,
	// and a list is not where a script should come by it. Pinned as given,
	// since populating every field would show a key.
	{Name: "service-account-list", Value: serviceAccountListJSON{ServiceAccounts: []serviceAccountJSON{
		{ID: "x", Label: sp("x"), Category: sp("x"), Active: true, CreatedAt: sp("x"), LastUsedAt: sp("x")},
	}}, AsGiven: true},
	// astro deployment|workspace service-account delete: what each deleted.
	{Name: "deployment-service-account-removal", Value: deploymentServiceAccountRemovalJSON{}},
	{Name: "workspace-service-account-removal", Value: workspaceServiceAccountRemovalJSON{}},

	// A user and the role they hold on the Workspace or the Deployment the
	// command is about: each item of `deployment user list` and `workspace
	// user list`, and what an add or an update of either publishes.
	{Name: "user", Value: userJSON{}},
	// A Deployment user as an add or an update publishes them: no
	// workspace role, and no full name, which the mutation does not return.
	{Name: "user-without-values", Value: userJSON{ID: sp("x"), Username: sp("x"), DeploymentRole: sp("x")}, AsGiven: true},
	{Name: "user-list", Value: userListJSON{}},
	// astro workspace|deployment user remove.
	{Name: "workspace-user-removal", Value: workspaceUserRemovalJSON{}},
	{Name: "deployment-user-removal", Value: deploymentUserRemovalJSON{}},
	// astro user create. Houston's session token for the new user is not
	// published.
	{Name: "user-created", Value: createdUserJSON{}},

	// A team and the role it holds on the platform, a Workspace or a
	// Deployment: each item of `team list`, `workspace team list` and
	// `deployment team list`, and what an add or an update of any of them
	// publishes.
	{Name: "team", Value: teamJSON{}},
	// A team as `workspace team add` publishes it: Houston's answer names
	// the Workspace, not the team.
	{Name: "team-without-values", Value: teamJSON{ID: "x", WorkspaceRole: sp("x")}, AsGiven: true},
	{Name: "team-list", Value: teamListJSON{}},
	// astro team get. users is null unless --users or --all asked for them.
	{Name: "team-detail", Value: teamDetailJSON{}},
	{Name: "team-detail-without-users", Value: teamDetailJSON{ID: "x", Name: sp("x"), SystemRole: "NONE", WorkspaceRoles: []teamWorkspaceRoleJSON{}, DeploymentRoles: []teamDeploymentRoleJSON{}}, AsGiven: true},
	// astro workspace|deployment team remove.
	{Name: "workspace-team-removal", Value: workspaceTeamRemovalJSON{}},
	{Name: "deployment-team-removal", Value: deploymentTeamRemovalJSON{}},
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

// What the commands actually emit, watched at the door by cliouttest.Watch,
// as cmd, cmd/astro and cmd/local watch theirs: TestMain arms it for the
// package run, and fails a passing run when a named type reached Emit that no
// golden pins, an anonymous struct reached it at all, or the observer was
// replaced during the run. Blind to what the suite does not run.

// minWatchedPayloads is a floor under the tally, not a target: an empty tally
// reads exactly like a clean one, so without it the observer coming unwired
// would be silent. Twenty-four shapes reach Emit in this package's tests today:
// the deployment family's Deployment, list, removal, version change and log
// entry, deploy's result, the workspace family's Workspace, list and
// removal, the user, team and service-account results, and the error
// object.
// Raise it as conversions land; lower it only saying why.
const minWatchedPayloads = 24

// pinnedElsewhere names the shapes that reach Emit here and are pinned by
// another tree's goldens, keyed by type, with where. Pinning one twice would
// give two goldens to keep in step for one contract.
var pinnedElsewhere = map[reflect.Type]string{
	// The failure object cliout.Execute publishes for every command under
	// --output json, whichever tree it is in.
	reflect.TypeOf(cliout.ErrorObject{}): "cmd/local/testdata/schema/error.json",
}

// emitWatch is this package's configuration of the observer TestMain arms.
func emitWatch() cliouttest.Watch {
	return cliouttest.Watch{
		Cases:           publishedPayloads,
		PinnedElsewhere: pinnedElsewhere,
		Floor:           minWatchedPayloads,
		File:            "cmd/apc/schema_test.go",
	}
}

// Every key a golden here publishes is snake_case: a capital in one is a Go
// field name that reached the wire because a tag was forgotten.
func TestPublishedKeysAreSnakeCase(t *testing.T) {
	assert.Empty(t, cliouttest.KeyProblems(t, schemaDir, len(publishedPayloads) > 0))
}
