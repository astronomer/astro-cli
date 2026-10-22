package apc

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// reached is what a run's command would have acted on: the Deployment and
// workspace it read, and the role and description flags it took.
type reached struct {
	deployment, workspace string
	deploymentRole        string
	userRole              string
	workspaceRole         string
	adoptDescription      string
}

// reach runs args on the APC tree built against a platform with every
// version and feature gate open, with the command they name replaced by one
// that records what it would have acted on. Everything up to the run is as
// in production: the flags, the hook that hands 1.x spellings on, the
// argument check and the required flags.
func reach(t *testing.T, args ...string) (reached, error) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	prevVersion, prevConfig := houstonVersion, appConfig
	houstonVersion = "2.1.0"
	appConfig = &houston.AppConfig{Flags: houston.FeatureFlags{AstroRuntimeEnabled: true, TriggererEnabled: true}}
	t.Cleanup(func() {
		houstonVersion, appConfig = prevVersion, prevConfig
		deploymentID, workspaceID = "", ""
	})
	root := &cobra.Command{Use: "astro", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newCmds(io.Discard)...)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	target, _, err := root.Find(args)
	require.NoError(t, err)
	var got reached
	target.RunE = func(*cobra.Command, []string) error {
		got = reached{
			deployment: deploymentID, workspace: workspaceID,
			deploymentRole: deploymentRole, userRole: deploymentUserRole,
			workspaceRole: workspaceTeamRole, adoptDescription: adoptDescription,
		}
		if strings.HasPrefix(target.CommandPath(), "astro workspace user") {
			got.workspaceRole = workspaceUserWsRole
		}
		return nil
	}
	root.SetArgs(args)
	_, err = root.ExecuteC()
	return got, err
}

// The commands that name their Deployment with --deployment alone, as the
// command line that reaches each, and the shorthand 1.x gave --deployment-id
// there, which the hidden alias keeps.
var deploymentFlagCommands = []struct {
	args      []string
	shorthand string
}{
	{[]string{"deployment", "unadopt"}, "d"},
	{[]string{"deployment", "airflow", "upgrade"}, "d"},
	{[]string{"deployment", "runtime", "upgrade"}, "d"},
	{[]string{"deployment", "runtime", "migrate"}, "d"},
	{[]string{"deployment", "service-account", "create", "--label", "l"}, "d"},
	{[]string{"deployment", "service-account", "list"}, "d"},
	{[]string{"deployment", "service-account", "delete", "sa-id"}, "d"},
	{[]string{"deployment", "user", "list"}, ""},
	{[]string{"deployment", "user", "add", "--email", "a@example.com"}, ""},
	{[]string{"deployment", "user", "remove", "a@example.com"}, ""},
	{[]string{"deployment", "user", "update", "a@example.com"}, ""},
	{[]string{"deployment", "team", "list"}, ""},
	{[]string{"deployment", "team", "add", "--team-id", "team-id"}, ""},
	{[]string{"deployment", "team", "remove", "team-id"}, ""},
	{[]string{"deployment", "team", "update", "team-id"}, ""},
}

func with(args []string, more ...string) []string {
	return append(append([]string{}, args...), more...)
}

// --deployment, 1.x's --deployment-id and the shorthand 1.x gave it all name
// the Deployment the command acts on. The shorthand is the alias's, as in
// 1.x; --deployment has none.
func TestDeploymentSpellingsReachTheSameDeployment(t *testing.T) {
	for _, c := range deploymentFlagCommands {
		label := strings.Join(c.args, " ")
		cmd := findAPC(t, c.args...)
		if f := cmd.Flag("deployment"); assert.NotNil(t, f, label) {
			assert.Empty(t, f.Shorthand, label)
			assert.False(t, f.Hidden, label)
		}
		if f := cmd.Flag("deployment-id"); assert.NotNil(t, f, label) {
			assert.Equal(t, c.shorthand, f.Shorthand, label)
			assert.True(t, f.Hidden, label)
		}
		spellings := [][]string{{"--deployment", "dep-id"}, {"--deployment-id", "dep-id"}, {"--deployment=dep-id", "--deployment-id=dep-id"}}
		if c.shorthand != "" {
			spellings = append(spellings, []string{"-" + c.shorthand, "dep-id"})
		}
		for _, s := range spellings {
			got, err := reach(t, with(c.args, s...)...)
			if assert.NoError(t, err, "%s %v", label, s) {
				assert.Equal(t, "dep-id", got.deployment, "%s %v", label, s)
			}
		}
	}
}

// Every one of them needs its Deployment, in either spelling: the team
// commands among them, whose mark was set before the flag existed and so
// never took.
func TestDeploymentFlagIsRequired(t *testing.T) {
	for _, c := range deploymentFlagCommands {
		label := strings.Join(c.args, " ")
		_, err := reach(t, c.args...)
		assert.EqualError(t, err, `required flag(s) "deployment" not set`, label)
	}
}

// The two spellings given together must agree; agreeing, they are one.
func TestDeploymentSpellingsMustAgree(t *testing.T) {
	for _, c := range deploymentFlagCommands {
		label := strings.Join(c.args, " ")
		_, err := reach(t, with(c.args, "--deployment", "dep-a", "--deployment-id", "dep-b")...)
		require.Error(t, err, label)
		assert.True(t, cliout.IsUsage(err), label)
		assert.Equal(t, `--deployment "dep-a" and --deployment-id "dep-b" disagree: pass only --deployment`, err.Error(), label)
	}
}

// --deployment given as "" is a mistake, refused. 1.x's --deployment-id
// given as "", or its shorthand, means what it did in 1.x, where it met the
// required check and named no Deployment.
func TestDeploymentEmptyValues(t *testing.T) {
	for _, c := range deploymentFlagCommands {
		label := strings.Join(c.args, " ")
		_, err := reach(t, with(c.args, "--deployment", "")...)
		require.Error(t, err, label)
		assert.True(t, cliout.IsUsage(err), label)
		assert.Equal(t, "--deployment cannot be empty", err.Error(), label)

		got, err := reach(t, with(c.args, "--deployment-id", "")...)
		assert.NoError(t, err, label)
		assert.Empty(t, got.deployment, label)
		if c.shorthand != "" {
			got, err = reach(t, with(c.args, "-"+c.shorthand, "")...)
			assert.NoError(t, err, label)
			assert.Empty(t, got.deployment, label)
		}
	}
}

// The team commands' --team-id is required, as it was meant to be: marked
// on the command's own flags, where a persistent flag is not, the mark never
// took.
func TestTeamIDIsRequired(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "team", "add", "--deployment", "dep-id"},
		{"workspace", "team", "add"},
	} {
		_, err := reach(t, args...)
		assert.EqualError(t, err, `required flag(s) "team-id" not set`, strings.Join(args, " "))
	}
}

// The commands that act in a workspace, as the command line that reaches
// each, and the shorthand 1.x gave --workspace-id there, which the hidden
// alias keeps.
var workspaceFlagCommands = []struct {
	args      []string
	shorthand string
}{
	{[]string{"deployment", "create", "--label", "l", "--cluster-id", "c"}, ""},
	{[]string{"deployment", "list"}, ""},
	{[]string{"deployment", "adopt", "--cluster-id", "c", "--cr-name", "n", "--namespace", "ns"}, ""},
	{[]string{"workspace", "user", "add", "--email", "a@example.com"}, "w"},
	{[]string{"workspace", "user", "update", "a@example.com"}, "w"},
	{[]string{"workspace", "user", "remove", "a@example.com"}, "w"},
	{[]string{"workspace", "user", "list"}, "w"},
	{[]string{"workspace", "team", "add", "--team-id", "team-id"}, ""},
	{[]string{"workspace", "team", "update", "team-id"}, ""},
	{[]string{"workspace", "team", "remove", "team-id"}, ""},
	{[]string{"workspace", "team", "list"}, ""},
	{[]string{"workspace", "service-account", "create", "--label", "l"}, "w"},
	{[]string{"workspace", "service-account", "list"}, "w"},
	{[]string{"workspace", "service-account", "delete", "sa-id"}, "w"},
	{[]string{"deploy"}, ""},
}

// --workspace, 1.x's --workspace-id and the shorthand 1.x gave it name the
// workspace; given together they must agree. --workspace given as "" is
// refused, and --workspace-id or its shorthand given as "" means the
// context's workspace, as in 1.x. --workspace has no shorthand.
func TestWorkspaceSpellings(t *testing.T) {
	for _, c := range workspaceFlagCommands {
		label := strings.Join(c.args, " ")
		cmd := findAPC(t, c.args...)
		if f := cmd.Flag("workspace"); assert.NotNil(t, f, label) {
			assert.Empty(t, f.Shorthand, label)
		}
		if f := cmd.Flag("workspace-id"); assert.NotNil(t, f, label) {
			assert.Equal(t, c.shorthand, f.Shorthand, label)
			assert.True(t, f.Hidden, label)
		}
		spellings := [][]string{{"--workspace", "ws-id"}, {"--workspace-id", "ws-id"}, {"--workspace=ws-id", "--workspace-id=ws-id"}}
		if c.shorthand != "" {
			spellings = append(spellings, []string{"-" + c.shorthand, "ws-id"})
		}
		for _, s := range spellings {
			got, err := reach(t, with(c.args, s...)...)
			if assert.NoError(t, err, "%s %v", label, s) {
				assert.Equal(t, "ws-id", got.workspace, "%s %v", label, s)
			}
		}

		_, err := reach(t, with(c.args, "--workspace", "ws-a", "--workspace-id", "ws-b")...)
		if assert.Error(t, err, label) {
			assert.True(t, cliout.IsUsage(err), label)
			assert.Equal(t, `--workspace "ws-a" and --workspace-id "ws-b" disagree: pass only --workspace`, err.Error(), label)
		}

		_, err = reach(t, with(c.args, "--workspace", "")...)
		if assert.Error(t, err, label) {
			assert.Equal(t, "--workspace cannot be empty", err.Error(), label)
		}

		got, err := reach(t, with(c.args, "--workspace-id", "")...)
		assert.NoError(t, err, label)
		assert.Empty(t, got.workspace, label)
		if c.shorthand != "" {
			got, err = reach(t, with(c.args, "-"+c.shorthand, "")...)
			assert.NoError(t, err, label)
			assert.Empty(t, got.workspace, label)
		}
	}
}

// The commands that take their Deployment as the argument, which still
// works as it did, or as --deployment.
var deploymentArgCommands = [][]string{
	{"deployment", "update"},
	{"deployment", "delete"},
	{"deployment", "hibernate"},
	{"deployment", "wake-up"},
	{"deployment", "logs", "webserver"},
	{"deployment", "logs", "scheduler"},
	{"deployment", "logs", "workers"},
	{"deployment", "logs"},
}

// The argument and --deployment each name the Deployment; given both, they
// must name the same one.
func TestDeploymentArgumentOrFlag(t *testing.T) {
	for _, args := range deploymentArgCommands {
		label := strings.Join(args, " ")
		for _, given := range [][]string{{"dep-id"}, {"--deployment", "dep-id"}, {"dep-id", "--deployment", "dep-id"}} {
			got, err := reach(t, with(args, given...)...)
			if assert.NoError(t, err, "%s %v", label, given) {
				assert.Equal(t, "dep-id", got.deployment, "%s %v", label, given)
			}
		}
		_, err := reach(t, with(args, "dep-a", "--deployment", "dep-b")...)
		if assert.Error(t, err, label) {
			assert.True(t, cliout.IsUsage(err), label)
			assert.Equal(t, `the Deployment argument "dep-a" and --deployment "dep-b" disagree: pass only one`, err.Error(), label)
		}
		_, err = reach(t, with(args, "--deployment", "")...)
		if assert.Error(t, err, label) {
			assert.Equal(t, "--deployment cannot be empty", err.Error(), label)
		}
		_, err = reach(t, with(args, "dep-a", "dep-b")...)
		assert.EqualError(t, err, "accepts at most 1 arg(s), received 2", label)
	}
}

// Given neither the argument nor --deployment, the command refuses to run,
// before it asks anything or calls Houston: delete does not ask to confirm
// deleting nothing.
func TestDeploymentArgumentOrFlagIsNeeded(t *testing.T) {
	for _, args := range deploymentArgCommands {
		label := strings.Join(args, " ")
		run := runAPC(t, newAPCClient(), "y\n", with(args, "-o", "json")...)
		require.Error(t, run.err, label)
		assert.True(t, cliout.IsUsage(run.err), label)
		assert.Equal(t, "name the Deployment: astro "+label+" <DEPLOYMENT_ID>, or --deployment <LINK_OR_DEPLOYMENT_ID>", run.err.Error(), label)
		assert.False(t, run.stdinRead, label)
	}
}

// Inside a project, --deployment takes an apc link's name, as astro deploy
// does, and the command acts on the link's Deployment. The argument and
// 1.x's --deployment-id stay what they were, an id, whatever it looks like.
func TestDeploymentFlagTakesALinkName(t *testing.T) {
	linkedProject(t)
	for _, c := range deploymentFlagCommands {
		label := strings.Join(c.args, " ")
		got, err := reach(t, with(c.args, "--deployment", "stage")...)
		if assert.NoError(t, err, label) {
			assert.Equal(t, "dep-stage", got.deployment, label)
		}
		got, err = reach(t, with(c.args, "--deployment-id", "stage")...)
		if assert.NoError(t, err, label) {
			assert.Equal(t, "stage", got.deployment, "%s: --deployment-id is an id, as in 1.x", label)
		}
		if c.shorthand != "" {
			got, err = reach(t, with(c.args, "-"+c.shorthand, "stage")...)
			if assert.NoError(t, err, label) {
				assert.Equal(t, "stage", got.deployment, "%s: -%s is --deployment-id, an id, as in 1.x", label, c.shorthand)
			}
		}
		got, err = reach(t, with(c.args, "--deployment", "dep-other")...)
		if assert.NoError(t, err, label) {
			assert.Equal(t, "dep-other", got.deployment, "%s: a value no link has is an id", label)
		}
	}
	for _, args := range deploymentArgCommands {
		label := strings.Join(args, " ")
		got, err := reach(t, with(args, "--deployment", "prod")...)
		if assert.NoError(t, err, label) {
			assert.Equal(t, "dep-prod", got.deployment, label)
		}
		got, err = reach(t, with(args, "prod")...)
		if assert.NoError(t, err, label) {
			assert.Equal(t, "prod", got.deployment, "%s: the argument is an id, as it was", label)
		}
		// The argument and the link agree when the argument is the link's
		// Deployment, or the link's name, and the command acts on the link's
		// Deployment either way.
		for _, arg := range []string{"dep-prod", "prod"} {
			got, err = reach(t, with(args, arg, "--deployment", "prod")...)
			if assert.NoError(t, err, "%s %s", label, arg) {
				assert.Equal(t, "dep-prod", got.deployment, "%s %s", label, arg)
			}
		}
		_, err = reach(t, with(args, "dep-stage", "--deployment", "prod")...)
		assert.EqualError(t, err, `the Deployment argument "dep-stage" and --deployment "prod" disagree: pass only one`, label)
	}
}

// A link this context cannot act on is refused by name, as astro deploy
// refuses it: an apc link on another domain, with the command that reaches
// it, and a link of another platform.
func TestDeploymentFlagRefusesALinkItCannotReach(t *testing.T) {
	linkedProject(t)
	for _, args := range [][]string{{"deployment", "service-account", "list"}, {"deployment", "logs", "webserver"}} {
		label := strings.Join(args, " ")
		_, err := reach(t, with(args, "--deployment", "elsewhere")...)
		if assert.Error(t, err, label) {
			assert.Contains(t, err.Error(), `link "elsewhere" is on astro.other.example`, label)
			assert.Contains(t, err.Error(), "run astro login astro.other.example, then run the command again", label)
		}
		_, err = reach(t, with(args, "--deployment", "cloud")...)
		if assert.Error(t, err, label) {
			assert.True(t, cliout.IsUsage(err), label)
			assert.Equal(t, `link "cloud" is astro, and astro `+label+` acts on Astro Private Cloud Deployments: name an apc link or a Deployment id`, err.Error(), label)
		}
	}
}

// A pyproject.toml that does not load is reported when --deployment would
// read a link from it. 1.x's spellings read no link, so they never load it.
func TestDeploymentFlagReportsAManifestThatDoesNotLoad(t *testing.T) {
	dir := inWorkingDir(t, false)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.astro]\nruntime = [\n"), 0o600))
	for _, args := range [][]string{{"deployment", "service-account", "list"}, {"deployment", "logs", "webserver"}} {
		label := strings.Join(args, " ")
		_, err := reach(t, with(args, "--deployment", "prod")...)
		assert.Error(t, err, label)
	}
	got, err := reach(t, "deployment", "service-account", "list", "-d", "prod")
	require.NoError(t, err)
	assert.Equal(t, "prod", got.deployment)
	got, err = reach(t, "deployment", "logs", "webserver", "prod")
	require.NoError(t, err)
	assert.Equal(t, "prod", got.deployment)
}

// A delete that a link named says which Deployment that is before it asks.
func TestDeploymentDeleteByLinkNamesTheDeployment(t *testing.T) {
	linkedProject(t)
	run := runAPC(t, newAPCClient(), "n\n", "deployment", "delete", "--deployment", "prod")
	require.NoError(t, run.err)
	assert.True(t, run.stdinRead)
	assert.Contains(t, run.stdout+run.stderr, "Link prod names Deployment dep-prod.")

	run = runAPC(t, newAPCClient(), "n\n", "deployment", "delete", "dep-prod")
	require.NoError(t, run.err)
	assert.NotContains(t, run.stdout+run.stderr, "names Deployment")
}

// findAPC is the command args name in the APC tree reach builds.
func findAPC(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	prevVersion, prevConfig := houstonVersion, appConfig
	houstonVersion = "2.1.0"
	appConfig = &houston.AppConfig{Flags: houston.FeatureFlags{AstroRuntimeEnabled: true, TriggererEnabled: true}}
	t.Cleanup(func() { houstonVersion, appConfig = prevVersion, prevConfig })
	root := &cobra.Command{Use: "astro"}
	root.AddCommand(newCmds(io.Discard)...)
	cmd, _, err := root.Find(args)
	require.NoError(t, err)
	return cmd
}

// -r is --role on the commands that take a role and had no shorthand for it,
// and -d is adopt's --description.
func TestRoleAndDescriptionShorthands(t *testing.T) {
	got, err := reach(t, "deployment", "user", "add", "--deployment", "d", "--email", "a@example.com", "-r", "DEPLOYMENT_EDITOR")
	require.NoError(t, err)
	assert.Equal(t, "DEPLOYMENT_EDITOR", got.userRole)
	got, err = reach(t, "deployment", "user", "update", "a@example.com", "--deployment", "d", "-r", "DEPLOYMENT_ADMIN")
	require.NoError(t, err)
	assert.Equal(t, "DEPLOYMENT_ADMIN", got.userRole)
	got, err = reach(t, "deployment", "team", "add", "--deployment", "d", "--team-id", "t", "-r", "DEPLOYMENT_EDITOR")
	require.NoError(t, err)
	assert.Equal(t, "DEPLOYMENT_EDITOR", got.deploymentRole)
	got, err = reach(t, "deployment", "team", "update", "t", "--deployment", "d", "-r", "DEPLOYMENT_ADMIN")
	require.NoError(t, err)
	assert.Equal(t, "DEPLOYMENT_ADMIN", got.deploymentRole)
	got, err = reach(t, "workspace", "team", "add", "--team-id", "t", "-r", "WORKSPACE_EDITOR")
	require.NoError(t, err)
	assert.Equal(t, "WORKSPACE_EDITOR", got.workspaceRole)
	got, err = reach(t, "workspace", "team", "update", "t", "-r", "WORKSPACE_ADMIN")
	require.NoError(t, err)
	assert.Equal(t, "WORKSPACE_ADMIN", got.workspaceRole)
	got, err = reach(t, "workspace", "user", "update", "a@example.com", "-r", "WORKSPACE_ADMIN")
	require.NoError(t, err)
	assert.Equal(t, "WORKSPACE_ADMIN", got.workspaceRole)
	got, err = reach(t, "deployment", "adopt", "--cluster-id", "c", "--cr-name", "n", "--namespace", "ns", "-d", "adopted")
	require.NoError(t, err)
	assert.Equal(t, "adopted", got.adoptDescription)
}
