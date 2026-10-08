package astro

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
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
	assert.True(t, cmd.Flags().Changed("workspace-id"), "followProject reads --workspace-id")
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
