package astro

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const unlinkedDep = "clunlinkeddeployment00001"

// parsePreferred finds the command args names under root, parses its flags
// and runs the hook every Astro command runs once they are parsed. It puts
// back what the hook and the flags set.
func parsePreferred(t *testing.T, root *cobra.Command, args ...string) (cmd *cobra.Command, positional []string) {
	t.Helper()
	t.Cleanup(func() {
		deploymentID, deploymentName, workspaceID, deploymentArg = "", "", "", ""
		resetEnvFlags()
	})
	cmd, rest, err := root.Find(args)
	require.NoError(t, err)
	require.NoError(t, cmd.ParseFlags(rest))
	require.NoError(t, applyPreferredFlags(cmd, cmd.Flags().Args()))
	return cmd, cmd.Flags().Args()
}

// --deployment goes where the older spellings would have taken the same
// value: an id to --deployment-id, anything else to --deployment-name, and
// everything to the one spelling a command has, except an id on a command
// that takes its Deployment's id as an argument.
func TestDeploymentFlagGoesWhereTheOldSpellingsWould(t *testing.T) {
	tests := []struct {
		name               string
		root               func() *cobra.Command
		args               []string
		wantID, wantName   string
		wantArg, wantEnvID string
	}{
		{name: "a name on delete is a name", args: []string{"delete", "--deployment", "my-dep"}, wantName: "my-dep"},
		{name: "an id on delete stands for the argument", args: []string{"delete", "--deployment", unlinkedDep}, wantID: unlinkedDep, wantArg: unlinkedDep},
		{name: "an id on inspect stands for the argument", args: []string{"inspect", "--deployment", unlinkedDep}, wantID: unlinkedDep, wantArg: unlinkedDep},
		{name: "an id where both spellings exist is an id", args: []string{"variable", "list", "--deployment", unlinkedDep}, wantID: unlinkedDep},
		{name: "a name where both spellings exist is a name", args: []string{"variable", "list", "--deployment", "my-dep"}, wantName: "my-dep"},
		{name: "a name on worker-queue is a name", args: []string{"worker-queue", "delete", "--deployment", "my-dep"}, wantName: "my-dep"},
		{name: "an id-only command takes anything as the id", args: []string{"team", "list", "--deployment", "my-dep"}, wantID: "my-dep"},
		{name: "env takes it as its deployment id", root: func() *cobra.Command { return newEnvRootCmd(new(bytes.Buffer)) }, args: []string{"variable", "list", "--deployment", "test"}, wantEnvID: "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newDeploymentRootCmd(new(bytes.Buffer))
			if tt.root != nil {
				root = tt.root()
			}
			parsePreferred(t, root, tt.args...)
			assert.Equal(t, tt.wantID, deploymentID, "deploymentID")
			assert.Equal(t, tt.wantName, deploymentName, "deploymentName")
			assert.Equal(t, tt.wantArg, deploymentArg, "deploymentArg")
			assert.Equal(t, tt.wantEnvID, envDeploymentID, "envDeploymentID")
		})
	}
}

func TestWorkspaceFlagIsTheWorkspaceID(t *testing.T) {
	parsePreferred(t, newDeploymentRootCmd(new(bytes.Buffer)), "list", "--workspace", projectWS)
	assert.Equal(t, projectWS, workspaceID)

	cmd, _ := parsePreferred(t, newDeploymentRootCmd(new(bytes.Buffer)), "create", "-w", projectWS)
	assert.Equal(t, projectWS, workspaceID)
	assert.True(t, cmd.Flags().Changed("workspace"), "followProject reads --workspace")
}

// Inside a project --deployment resolves in the order the older spellings
// do: a link name first, then a Deployment id, then a Deployment name.
func TestDeploymentFlagResolvesALinkFirst(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	tests := []struct {
		name     string
		args     []string
		wantID   string
		wantName string
		wantLink string
	}{
		{name: "a link name on a name-and-argument command", args: []string{"delete", "--deployment", "test"}, wantID: projectDep, wantLink: "test"},
		{name: "a link's id on a name-and-argument command", args: []string{"delete", "--deployment", projectDep}, wantID: projectDep, wantLink: "test"},
		{name: "an id no link holds", args: []string{"delete", "--deployment", unlinkedDep}, wantID: unlinkedDep},
		{name: "a name no link holds stays a name", args: []string{"delete", "--deployment", "my deployment"}, wantName: "my deployment"},
		{name: "a link name where both spellings exist", args: []string{"variable", "list", "--deployment", "test"}, wantID: projectDep, wantLink: "test"},
		{name: "a link's id where both spellings exist", args: []string{"variable", "list", "--deployment", projectDep}, wantID: projectDep, wantLink: "test"},
		{name: "a link name on an id-only command", args: []string{"team", "list", "--deployment", "test"}, wantID: projectDep, wantLink: "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, testProjectManifest)
			cmd, args := parsePreferred(t, newDeploymentRootCmd(new(bytes.Buffer)), tt.args...)
			pick, err := followProject(cmd, args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, deploymentID, "deploymentID")
			assert.Equal(t, tt.wantName, deploymentName, "deploymentName")
			assert.Equal(t, tt.wantLink, pick.link, "link")
		})
	}
}

func TestEnvTakesALinkNameAsDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.DeploymentId != nil && *p.DeploymentId == projectDep && p.WorkspaceId == nil
	})
	expectWorkspaceName(mc)
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list", "--deployment", "test")
	require.NoError(t, err)
	assert.Contains(t, out, "using workspace Example from pyproject.toml (link test)\n")
	mc.AssertExpectations(t)
}

func TestEnvWorkspaceFlagWinsOverTheProjectToo(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectEnvList(mc, func(p *astrov1.ListEnvironmentObjectsParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == "clexplicitworkspace000001"
	})
	astroV1Client = mc

	out, err := execEnvCmd("variable", "list", "--workspace", "clexplicitworkspace000001")
	require.NoError(t, err)
	assert.NotContains(t, out, "pyproject.toml")
	mc.AssertExpectations(t)
}

// Two spellings naming different things is a mistake the CLI cannot settle
// by picking one: it is a usage error, before anything is looked up.
func TestPreferredAndOldSpellingsThatDisagreeAreAUsageError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	astroV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	t.Cleanup(func() { deploymentID, deploymentName, workspaceID, deploymentArg = "", "", "", ""; resetEnvFlags() })

	for _, args := range [][]string{
		{"delete", "--deployment", "a", "--deployment-name", "b"},
		{"variable", "list", "--deployment", "a", "--deployment-id", "b"},
		{"list", "--workspace", "a", "--workspace-id", "b"},
	} {
		_, err := execDeploymentCmd(args...)
		require.Error(t, err, strings.Join(args, " "))
		assert.True(t, cliout.IsUsage(err), "%s: %v", strings.Join(args, " "), err)
		assert.Contains(t, err.Error(), "disagree")
	}

	_, err := execEnvCmd("variable", "list", "--workspace", "a", "--workspace-id", "b")
	require.Error(t, err)
	assert.True(t, cliout.IsUsage(err))
}

// On a command whose Deployment is its argument, an argument and a --deployment
// id naming two Deployments is refused before anything runs: `delete <a>
// --deployment <b>` used to delete a without a word.
func TestDeploymentArgumentAndFlagThatDisagreeAreAUsageError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc
	t.Cleanup(func() { deploymentID, deploymentName, workspaceID, deploymentArg = "", "", "", ""; resetEnvFlags() })

	const idA, idB = "clm8t5u4q000008l3bzde0w7n", "clm8t5u4q000008l3bzde0w7m"
	_, err := execDeploymentCmd("delete", idA, "--deployment", idB, "--yes")
	require.Error(t, err)
	assert.True(t, cliout.IsUsage(err), "%v", err)
	assert.Contains(t, err.Error(), "disagree")
	mc.AssertExpectations(t) // no calls were expected, so nothing was deleted
}

// Help shows one spelling. The old ones work but are not offered.
func TestHelpShowsOnlyThePreferredSpellings(t *testing.T) {
	for _, tc := range []struct {
		exec func(...string) (string, error)
		args []string
		show []string
		hide []string
	}{
		{execDeploymentCmd, []string{"delete", "--help"}, []string{"--deployment string", "--workspace string"}, []string{"--deployment-name", "--workspace-id"}},
		{execDeploymentCmd, []string{"inspect", "--help"}, []string{"--deployment string"}, []string{"--deployment-name", "--workspace-id"}},
		{execDeploymentCmd, []string{"variable", "list", "--help"}, []string{"--deployment string"}, []string{"--deployment-id", "--deployment-name"}},
		{execEnvCmd, []string{"variable", "list", "--help"}, []string{"--deployment string", "--workspace string"}, []string{"--deployment-id", "--workspace-id"}},
	} {
		out, err := tc.exec(tc.args...)
		require.NoError(t, err)
		for _, s := range tc.show {
			assert.Contains(t, out, s, strings.Join(tc.args, " "))
		}
		for _, s := range tc.hide {
			assert.NotContains(t, out, s, strings.Join(tc.args, " "))
		}
	}
}

// aliasTrees builds, fresh each time, every Astro tree whose commands take a
// 1.x spelling as an alias of --deployment or --workspace.
var aliasTrees = map[string]func() *cobra.Command{
	"deployment":   func() *cobra.Command { return newDeploymentRootCmd(new(bytes.Buffer)) },
	"env":          func() *cobra.Command { return newEnvRootCmd(new(bytes.Buffer)) },
	"workspace":    func() *cobra.Command { return newWorkspaceCmd(new(bytes.Buffer)) },
	"organization": func() *cobra.Command { return newOrganizationCmd(new(bytes.Buffer)) },
	"deploy":       NewDeployCmd,
	"remote":       func() *cobra.Command { return newRemoteRootCmd(new(bytes.Buffer)) },
}

// aliasValue is a value of the kind each 1.x spelling took, and aliasOther a
// second one, for a disagreement.
var (
	aliasValue = map[string]string{"deployment-id": unlinkedDep, "deployment-name": "my-dep", "workspace-id": projectWS}
	aliasOther = map[string]string{"deployment-id": otherDep, "deployment-name": "other-dep", "workspace-id": "clotherworkspace000000001"}
)

// aliasUse is one 1.x spelling on one command.
type aliasUse struct {
	tree      string
	path      []string
	alias     string
	shorthand string
	preferred string
}

// aliasUses walks every runnable command of every alias tree for the 1.x
// spellings it takes, its own and those it inherits.
func aliasUses(t *testing.T) []aliasUse {
	t.Helper()
	var uses []aliasUse
	for name, build := range aliasTrees {
		var walk func(c *cobra.Command, path []string)
		walk = func(c *cobra.Command, path []string) {
			if c.Runnable() && !c.DisableFlagParsing {
				c.Flags().AddFlagSet(c.InheritedFlags())
				c.Flags().VisitAll(func(f *pflag.Flag) {
					if utils.IsAlias(f) {
						uses = append(uses, aliasUse{name, path, f.Name, f.Shorthand, f.Annotations["astro.alias-of"][0]})
					}
				})
			}
			for _, sub := range c.Commands() {
				walk(sub, append(append([]string(nil), path...), sub.Name()))
			}
		}
		walk(build(), nil)
	}
	require.NotEmpty(t, uses)
	return uses
}

// resolvedSpellings is what a command reads once its spellings are applied:
// its Deployment id and name, an id standing for its argument, and its
// workspace.
type resolvedSpellings struct{ id, name, arg, workspace string }

func resolveSpellings(t *testing.T, u *aliasUse, args ...string) (resolvedSpellings, error) {
	t.Helper()
	t.Cleanup(func() {
		deploymentID, deploymentName, workspaceID, deploymentArg = "", "", "", ""
		resetEnvFlags()
	})
	cmd, _, err := aliasTrees[u.tree]().Find(u.path)
	require.NoError(t, err)
	require.NoError(t, cmd.ParseFlags(args))
	if err := applyPreferredFlags(cmd, cmd.Flags().Args()); err != nil {
		return resolvedSpellings{}, err
	}
	r := resolvedSpellings{arg: deploymentArg}
	if d := deploymentFlagOf(cmd.Flags()); d != nil {
		if d.id != nil {
			r.id = *d.id
		}
		if d.name != nil {
			r.name = *d.name
		}
	}
	if w := cmd.Flags().Lookup("workspace"); w != nil {
		r.workspace = w.Value.String()
	}
	return r, nil
}

// Every 1.x spelling, and the shorthand 1.x gave it, reaches exactly what the
// v2 spelling given the same value reaches, on every command that takes it.
// A 1.x spelling and a v2 one naming different things is still refused.
func TestEveryOneXSpellingReachesWhatTheV2SpellingDoes(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	uses := aliasUses(t)
	for i := range uses {
		u := &uses[i]
		v := aliasValue[u.alias]
		where := u.tree + " " + strings.Join(u.path, " ") + " --" + u.alias
		t.Run(where, func(t *testing.T) {
			want, err := resolveSpellings(t, u, "--"+u.preferred, v)
			require.NoError(t, err)
			require.NotEqual(t, resolvedSpellings{}, want, "--%s reached nothing", u.preferred)

			got, err := resolveSpellings(t, u, "--"+u.alias, v)
			require.NoError(t, err)
			assert.Equal(t, want, got, "--%s", u.alias)

			if u.shorthand != "" {
				short, err := resolveSpellings(t, u, "-"+u.shorthand, v)
				require.NoError(t, err)
				assert.Equal(t, want, short, "-%s", u.shorthand)
			}

			got, err = resolveSpellings(t, u, "--"+u.preferred, v, "--"+u.alias, v)
			require.NoError(t, err)
			assert.Equal(t, want, got, "both spellings, agreeing")

			_, err = resolveSpellings(t, u, "--"+u.preferred, v, "--"+u.alias, aliasOther[u.alias])
			require.Error(t, err)
			assert.True(t, cliout.IsUsage(err), "%v", err)
			assert.EqualError(t, err, fmt.Sprintf("--%s %q and --%s %q disagree: pass only --%s", u.preferred, v, u.alias, aliasOther[u.alias], u.preferred))
		})
	}
}

// The shorthands 1.x gave its spellings are still there, on the hidden
// aliases, so a 1.x script's -d and -n keep working.
func TestTheOneXShorthandsAreKept(t *testing.T) {
	short := map[string]string{}
	for _, u := range aliasUses(t) {
		if u.shorthand != "" {
			short[u.tree+" "+strings.Join(u.path, " ")+" -"+u.shorthand] = u.alias
		}
	}
	for cmd, alias := range map[string]string{
		"deployment variable list -d":       "deployment-id",
		"deployment variable list -n":       "deployment-name",
		"deployment variable create -d":     "deployment-id",
		"deployment variable update -n":     "deployment-name",
		"deployment worker-queue create -d": "deployment-id",
		"deployment logs -n":                "deployment-name",
		"deployment delete -n":              "deployment-name",
		"deployment inspect -n":             "deployment-name",
		"deployment hibernate -n":           "deployment-name",
		"deployment wake-up -n":             "deployment-name",
		"deployment team add -w":            "deployment-id",
	} {
		assert.Equal(t, alias, short[cmd], cmd)
	}
}

// 1.x read --deployment-id as an id and --deployment-name as a name whatever
// they looked like, and took both at once. An alias keeps the kind it was
// given as: only a value typed as --deployment is split by its shape.
func TestADeploymentAliasKeepsItsKind(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	list := &aliasUse{tree: "deployment", path: []string{"variable", "list"}}

	r, err := resolveSpellings(t, list, "--deployment-id", "test-id-1")
	require.NoError(t, err)
	assert.Equal(t, resolvedSpellings{id: "test-id-1"}, r, "a non-CUID id is still an id")

	r, err = resolveSpellings(t, list, "-n", unlinkedDep)
	require.NoError(t, err)
	assert.Equal(t, resolvedSpellings{name: unlinkedDep}, r, "a CUID-shaped name is still a name")

	r, err = resolveSpellings(t, list, "-d", unlinkedDep, "-n", "my-dep")
	require.NoError(t, err)
	assert.Equal(t, resolvedSpellings{id: unlinkedDep, name: "my-dep"}, r, "both, as 1.x took them")

	r, err = resolveSpellings(t, list, "--deployment", "test-id-1")
	require.NoError(t, err)
	assert.Equal(t, resolvedSpellings{name: "test-id-1"}, r, "--deployment is split by shape")
}

// On the env link commands the leaf's own --deployment-id is the link's
// target, and it shadows the noun's persistent --deployment-id, which is the
// scope. A 1.x spelling there names the Deployment to link, never the scope.
func TestEnvLinkDeploymentIDIsTheLinkTargetNotTheScope(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	for _, path := range [][]string{
		{"variable", "link", "set"},
		{"variable", "link", "delete"},
		{"connection", "link", "set"},
		{"connection", "link", "delete"},
		{"airflow-variable", "link", "set"},
		{"airflow-variable", "link", "delete"},
	} {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			cmd, _ := parsePreferred(t, newEnvRootCmd(new(bytes.Buffer)), append(path, "--deployment-id", unlinkedDep)...)
			d := deploymentFlagOf(cmd.Flags())
			require.NotNil(t, d)
			assert.Equal(t, unlinkedDep, *d.id, "the link target")
			assert.NotSame(t, &envDeploymentID, d.id, "the leaf's --deployment is not the scope")
			assert.Empty(t, envDeploymentID, "the scope")
			assert.True(t, cmd.Flags().Changed("deployment"), "a required --deployment is met")
		})
	}
}
