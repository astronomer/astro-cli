package astro

import (
	"bytes"
	"net/http"
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

// deploymentsWorkspace is the Workspace the Deployment of the cases below is
// in: neither the project's, nor the explicit one, nor the context's.
const deploymentsWorkspace = "cldeploymentsworkspace0001"

// A Workspace token can hold a role only on Deployments of its own Workspace,
// so workspace-token add looks for the token it is given by name in the
// Deployment's own Workspace, whatever Workspace the project or the context
// names: it reads the Deployment for that. --workspace is the explicit
// override. remove picks the Workspace token among the workspace the command is
// pointed at: --workspace, else the project's, else the context's.
func TestWorkspaceTokenAddSearchesTheDeploymentsWorkspaceAndRemoveTheOneItIsPointedAt(t *testing.T) {
	_, wsTok, _ := tokenFixtures()
	inWS := func(ws string) any {
		return mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
			return p.WorkspaceId != nil && *p.WorkspaceId == ws
		})
	}
	// client answers the token lookup in searched, and, for an add, the read of
	// the Deployment (in deploymentsWorkspace) that finds it.
	client := func(t *testing.T, searched string, readsDeployment bool) *astrov1_mocks.ClientWithResponsesInterface {
		t.Helper()
		// A token id an earlier test's argument left in the package would
		// skip the lookup by name these cases are about.
		tokenID, orgTokenID, workspaceTokenID = "", "", ""
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, inWS(searched)).Return(listTokensResp(wsTok), nil).Once()
		mc.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, wsTok.Id).Return(getTokenResp(wsTok), nil).Once()
		mc.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, wsTok.Id, mock.Anything).Return(rolesOK(), nil).Once()
		if readsDeployment {
			mc.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, otherDep).Return(&astrov1.GetDeploymentResponse{
				HTTPResponse: ok200(), JSON200: &astrov1.Deployment{Id: otherDep, WorkspaceId: deploymentsWorkspace},
			}, nil).Once()
		}
		return mc
	}
	for _, verb := range [][]string{{"add", "--role", "DEPLOYMENT_ADMIN"}, {"remove"}} {
		add := verb[0] == "add"
		args := append([]string{"token", "workspace-token", verb[0], "--name", wsTok.Name, "--deployment", otherDep}, verb[1:]...)
		t.Run(verb[0]+" in a project", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			inProject(t, testProjectManifest)
			// The project's Workspace is A and the Deployment is in B: B for an
			// add, A for a remove.
			searched := projectWS
			if add {
				searched = deploymentsWorkspace
			}
			mc := client(t, searched, add)
			expectWorkspaceName(mc)
			astroV1Client = mc
			_, err := execDeploymentCmd(args...)
			require.NoError(t, err)
			mc.AssertExpectations(t)
		})
		t.Run(verb[0]+" with --workspace in a project", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			inProject(t, testProjectManifest)
			// The override is searched, and the Deployment is not even read.
			mc := client(t, "clexplicitworkspace000001", false)
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
			searched := c.Workspace
			if add {
				searched = deploymentsWorkspace
			}
			mc := client(t, searched, add)
			astroV1Client = mc
			_, err = execDeploymentCmd(args...)
			require.NoError(t, err)
			mc.AssertExpectations(t)
		})
	}
	t.Run("an add with no workspace anywhere still finds the Deployment's", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		noWorkspaceContext(t)
		mc := client(t, deploymentsWorkspace, true)
		astroV1Client = mc
		_, err := execDeploymentCmd("token", "workspace-token", "add", "--name", wsTok.Name, "--deployment", otherDep, "--role", "DEPLOYMENT_ADMIN")
		require.NoError(t, err)
		mc.AssertExpectations(t)
	})
	t.Run("an add that cannot read the Deployment says so, and that --workspace names one", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		tokenID, orgTokenID, workspaceTokenID = "", "", ""
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		mc.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, otherDep).Return(&astrov1.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}, Body: []byte(`{"message":"nope"}`),
		}, nil).Once()
		astroV1Client = mc
		_, err := execDeploymentCmd("token", "workspace-token", "add", "--name", wsTok.Name, "--deployment", otherDep, "--role", "DEPLOYMENT_ADMIN")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not read Deployment "+otherDep+" to find the Workspace its Workspace tokens are in; pass --workspace: ")
		assert.Contains(t, err.Error(), "nope")
		mc.AssertExpectations(t)
	})
	t.Run("a remove with no workspace anywhere says to pass one", func(t *testing.T) {
		t.Setenv("ASTRO_API_TOKEN", "") // a token run gets another message
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		noWorkspaceContext(t)
		tokenID, orgTokenID, workspaceTokenID = "", "", ""
		astroV1Client = new(astrov1_mocks.ClientWithResponsesInterface) // asked nothing
		_, err := execDeploymentCmd("token", "workspace-token", "remove", "--name", wsTok.Name, "--deployment", otherDep)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find a valid workspace to find the Workspace token in; pass --workspace, or the token's ID: ")
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

// deployment token add --name looks for a Workspace token among those of the
// Deployment's own Workspace, not the project's or the context's: with the
// project's Workspace A and the Deployment in B, B is searched and A is not.
func TestDeploymentTokenAddByNameSearchesTheDeploymentsWorkspaceNotTheProjects(t *testing.T) {
	_, wsTok, orgTok := tokenFixtures()
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	inProject(t, testProjectManifest)
	tokenID, orgTokenID, workspaceTokenID = "", "", ""
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, otherDep).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: ok200(), JSON200: &astrov1.Deployment{Id: otherDep, WorkspaceId: deploymentsWorkspace},
	}, nil).Once()
	inAWorkspace := func(p *astrov1.ListApiTokensParams) bool { return p.WorkspaceId != nil }
	mc.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(inAWorkspace)).Return(listTokensResp(wsTok), nil)
	mc.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool { return !inAWorkspace(p) })).Return(listTokensResp(orgTok), nil)
	mc.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, wsTok.Id).Return(getTokenResp(wsTok), nil).Once()
	mc.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, wsTok.Id, mock.Anything).Return(rolesOK(), nil).Once()
	expectWorkspaceName(mc)
	astroV1Client = mc

	_, err := execDeploymentCmd("token", "add", "--name", wsTok.Name, "--deployment", otherDep, "--role", "DEPLOYMENT_ADMIN")
	require.NoError(t, err)

	var searched []string
	for i := range mc.Calls {
		if mc.Calls[i].Method != "ListApiTokensWithResponse" {
			continue
		}
		if p, ok := mc.Calls[i].Arguments.Get(2).(*astrov1.ListApiTokensParams); ok && p.WorkspaceId != nil {
			searched = append(searched, *p.WorkspaceId)
		}
	}
	assert.Equal(t, []string{deploymentsWorkspace}, searched, "the Workspaces whose tokens were listed; the project's is %s", projectWS)
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
