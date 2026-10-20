package astro

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The deployment commands that look their Deployment, or a token, up in a
// workspace, and so take --workspace (and 1.x's --workspace-id as its alias).
var deploymentCommandsReadingTheWorkspace = []string{
	"list", "create", "update", "delete", "logs", "inspect", "hibernate", "wake-up",
	"variable list", "variable create", "variable update",
	"worker-queue create", "worker-queue update", "worker-queue delete",
	"bundle create", "bundle list", "bundle update", "bundle delete",
	"token workspace-token add", "token workspace-token remove",
}

// The deployment commands that act on a Deployment by its id alone. 1.x gave
// each of them --workspace-id, through the deployment group, and none read
// it: they keep it, hidden and doing nothing, and take no --workspace.
var deploymentCommandsIgnoringTheWorkspace = []string{
	"team list", "team add", "team update", "team remove",
	"user list", "user add", "user update", "user remove",
	"token list", "token create", "token update", "token rotate", "token delete",
	"token organization-token add", "token organization-token update", "token organization-token remove", "token organization-token list",
	"token workspace-token update", "token workspace-token list",
}

func findDeploymentCmd(t *testing.T, path string) *cobra.Command {
	t.Helper()
	c, rest, err := newDeploymentRootCmd(new(bytes.Buffer)).Find(strings.Fields(path))
	require.NoError(t, err, path)
	require.Empty(t, rest, path)
	require.Equal(t, "deployment "+path, c.CommandPath(), path)
	return c
}

func TestDeploymentWorkspaceFlagIsOnlyWhereItIsRead(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	for _, path := range deploymentCommandsReadingTheWorkspace {
		c := findDeploymentCmd(t, path)
		ws := c.Flag("workspace")
		if assert.NotNil(t, ws, path) {
			assert.False(t, ws.Hidden, path)
		}
		if alias := c.Flag("workspace-id"); assert.NotNil(t, alias, path) {
			assert.True(t, alias.Hidden, path)
			assert.Contains(t, alias.Usage, "Another spelling of --workspace", path)
		}
	}
	for _, path := range deploymentCommandsIgnoringTheWorkspace {
		c := findDeploymentCmd(t, path)
		assert.Nil(t, c.Flag("workspace"), path)
		if noop := c.Flag("workspace-id"); assert.NotNil(t, noop, path) {
			assert.True(t, noop.Hidden, path)
			assert.Equal(t, utils.OneXNoOpUsage, noop.Usage, path)
			assert.Empty(t, noop.Annotations, "%s: the no-op is no alias and chooses no workspace", path)
		}
		require.NoError(t, c.ParseFlags([]string{"--workspace-id", "clexplicitworkspace000001"}), path)
		require.NoError(t, applyPreferredFlags(c, nil), path)
		assert.Empty(t, workspaceID, "%s: the no-op sets nothing", path)
	}
	_, err := execDeploymentCmd("user", "list", "--workspace", "clexplicitworkspace000001")
	require.EqualError(t, err, "unknown flag: --workspace")
}

// Inside a project, a command that ignores the workspace still runs on the
// project's workspace and host when given 1.x's --workspace-id: the no-op is
// no choice of workspace. --workspace-id on a command that reads it is one,
// and keeps the context, as --workspace does.
func TestOneXWorkspaceIDNoOpLeavesTheProjectInCharge(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	pick := func(t *testing.T, path string, args ...string) projectPick {
		t.Helper()
		inProject(t, testProjectManifest)
		c := findDeploymentCmd(t, path)
		require.NoError(t, c.ParseFlags(args), path)
		require.NoError(t, applyPreferredFlags(c, nil), path)
		p, err := followProject(c, nil)
		require.NoError(t, err, path)
		return p
	}
	for _, path := range deploymentCommandsIgnoringTheWorkspace {
		t.Run(path, func(t *testing.T) {
			without := pick(t, path, "--deployment", otherDep)
			with := pick(t, path, "--deployment", otherDep, "--workspace-id", "clexplicitworkspace000001")
			assert.Equal(t, projectPick{workspace: projectWS, domain: "astronomer.io"}, without)
			assert.Equal(t, without, with)
		})
	}
	for _, path := range []string{"variable list", "token workspace-token add"} {
		t.Run(path+" reads it", func(t *testing.T) {
			assert.Equal(t, projectPick{}, pick(t, path, "--deployment", otherDep, "--workspace-id", "clexplicitworkspace000001"))
		})
	}

	// And end to end: the project's workspace is the one named, which only
	// happens when the project chose it.
	t.Run("token list runs on the project's workspace", func(t *testing.T) {
		inProject(t, testProjectManifest)
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listTokensResp(), nil).Once()
		expectWorkspaceName(mc)
		astroV1Client = mc
		_, err := execDeploymentCmd("token", "list", "--deployment", otherDep, "--workspace-id", "clexplicitworkspace000001")
		require.NoError(t, err)
		assert.Empty(t, workspaceID)
		mc.AssertExpectations(t)
	})
}

// workspace-token add and remove pick the Workspace token among the
// workspace the command is pointed at: --workspace, else the project's, else
// the context's.
func TestWorkspaceTokenAddAndRemovePickInTheWorkspaceTheyArePointedAt(t *testing.T) {
	_, wsTok, _ := tokenFixtures()
	inWS := func(ws string) any {
		return mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
			return p.WorkspaceId != nil && *p.WorkspaceId == ws
		})
	}
	client := func(t *testing.T, ws string) *astrov1_mocks.ClientWithResponsesInterface {
		t.Helper()
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, inWS(ws)).Return(listTokensResp(wsTok), nil).Once()
		mc.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, wsTok.Id).Return(getTokenResp(wsTok), nil).Once()
		mc.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, wsTok.Id, mock.Anything).Return(rolesOK(), nil).Once()
		return mc
	}
	for _, verb := range [][]string{{"add", "--role", "DEPLOYMENT_ADMIN"}, {"remove"}} {
		args := append([]string{"token", "workspace-token", verb[0], "--workspace-token-name", wsTok.Name, "--deployment", otherDep}, verb[1:]...)
		t.Run(verb[0]+" in a project", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			inProject(t, testProjectManifest)
			mc := client(t, projectWS)
			expectWorkspaceName(mc)
			astroV1Client = mc
			_, err := execDeploymentCmd(args...)
			require.NoError(t, err)
			mc.AssertExpectations(t)
		})
		t.Run(verb[0]+" with --workspace in a project", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			inProject(t, testProjectManifest)
			mc := client(t, "clexplicitworkspace000001")
			astroV1Client = mc
			_, err := execDeploymentCmd(append(args, "--workspace", "clexplicitworkspace000001")...)
			require.NoError(t, err)
			mc.AssertExpectations(t)
		})
		t.Run(verb[0]+" outside a project", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			t.Cleanup(func() { workspaceID = "" })
			c, err := config.GetCurrentContext()
			require.NoError(t, err)
			mc := client(t, c.Workspace)
			astroV1Client = mc
			_, err = execDeploymentCmd(args...)
			require.NoError(t, err)
			mc.AssertExpectations(t)
		})
	}
	t.Run("a token given by name with no workspace anywhere says to pass one", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		noWorkspaceContext(t)
		astroV1Client = new(astrov1_mocks.ClientWithResponsesInterface) // asked nothing
		for _, verb := range [][]string{{"add", "--role", "DEPLOYMENT_ADMIN"}, {"remove"}} {
			args := append([]string{"token", "workspace-token", verb[0], "--workspace-token-name", wsTok.Name, "--deployment", otherDep}, verb[1:]...)
			_, err := execDeploymentCmd(args...)
			require.Error(t, err, verb[0])
			assert.Contains(t, err.Error(), "failed to find a valid workspace to find the Workspace token in; pass --workspace, or the token's ID: ", verb[0])
		}
	})
	t.Run("a token given by id needs no workspace", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		t.Cleanup(func() { workspaceTokenID = "" })
		noWorkspaceContext(t)
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, wsTok.Id).Return(getTokenResp(wsTok), nil).Once()
		mc.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, wsTok.Id, mock.Anything).Return(rolesOK(), nil).Once()
		astroV1Client = mc
		_, err := execDeploymentCmd("token", "workspace-token", "remove", wsTok.Id, "--deployment", tokDeploymentID)
		require.NoError(t, err)
		mc.AssertExpectations(t)
	})
}

// noWorkspaceContext leaves the current context with no workspace, as an
// Organization-scoped API token does.
func noWorkspaceContext(t *testing.T) {
	t.Helper()
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetContextKey("workspace", ""))
}

// --all lists every workspace's Deployments, so it needs no workspace to
// find: a login with none, such as an Organization token's, still lists them.
func TestDeploymentListAllNeedsNoWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	noWorkspaceContext(t)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListDeploymentsParams) bool {
		return p.WorkspaceIds == nil
	})).Return(projectDeploymentsResponse(otherDep), nil).Once()
	astroV1Client = mc

	out, err := execDeploymentCmd("list", "--all")
	require.NoError(t, err)
	assert.Contains(t, out, otherDep)
	mc.AssertExpectations(t)

	_, err = execDeploymentCmd("list")
	require.ErrorContains(t, err, "failed to find a valid workspace")
}
