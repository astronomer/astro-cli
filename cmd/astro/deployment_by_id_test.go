package astro

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// byIDRun is an Organization token run with no Workspace, and a client that
// knows one Deployment: test-id-1, in workspace-id.
func byIDRun(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	withWorkspaces(t, "", "")
	runWithToken(t, "apiTokenId:token-id", "organizationId:env-org")
	resetTokenCommandFlags(t)
	deploymentName = ""
	t.Cleanup(func() { deploymentName = "" })
	// An id is a CUID, which is what lets a Deployment be fetched without a
	// Workspace; the fixtures answer to this one.
	oldGet, oldList := deploymentResponse.JSON200.Id, mockListDeploymentsResponse.JSON200.Deployments[0].Id
	deploymentResponse.JSON200.Id, mockListDeploymentsResponse.JSON200.Deployments[0].Id = byID, byID
	t.Cleanup(func() {
		deploymentResponse.JSON200.Id, mockListDeploymentsResponse.JSON200.Deployments[0].Id = oldGet, oldList
	})
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc
	return mc
}

// byID is a Deployment id, as the platform spells them.
const byID = "clbyiddeployment000000001"

// listedInDeploymentsWorkspace matches a Deployment list asked of the
// Workspace the Deployment fetched by ID is in, and of no other.
func listedInDeploymentsWorkspace(workspace string) any {
	return mock.MatchedBy(func(p *astrov1.ListDeploymentsParams) bool {
		return p != nil && p.WorkspaceIds != nil && len(*p.WorkspaceIds) == 1 && (*p.WorkspaceIds)[0] == workspace
	})
}

// A Deployment given by its ID is identified by it, so an Organization token
// that selects no Workspace can act on it: the Deployment is fetched by ID, and
// the command goes on in the Workspace it is in.
func TestDeploymentCommandsAddressedByIDNeedNoWorkspaceUnderAnOrganizationToken(t *testing.T) {
	deleted := astrov1.DeleteDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
	value := "test-value-1"
	for name, tc := range map[string]struct {
		args  []string
		setup func(mc *astrov1_mocks.ClientWithResponsesInterface)
		want  string
	}{
		"delete": {
			args: []string{"delete", byID, "--yes"},
			setup: func(mc *astrov1_mocks.ClientWithResponsesInterface) {
				mc.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, byID).Return(&deleted, nil).Once()
			},
		},
		"variable list": {
			args: []string{"variable", "list", "--deployment-id", byID},
			setup: func(*astrov1_mocks.ClientWithResponsesInterface) {
				deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{{Key: "test-key-1", Value: &value}}
			},
			want: "test-key-1",
		},
		"logs": {
			args: []string{"logs", byID, "-w"},
			setup: func(mc *astrov1_mocks.ClientWithResponsesInterface) {
				mc.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			mc := byIDRun(t)
			tc.setup(mc)
			mc.On("GetDeploymentWithResponse", mock.Anything, "env-org", byID).Return(&deploymentResponse, nil)
			mc.On("ListDeploymentsWithResponse", mock.Anything, "env-org", listedInDeploymentsWorkspace("workspace-id")).Return(&mockListDeploymentsResponse, nil)

			out, err := execDeploymentCmd(tc.args...)
			require.NoError(t, err)
			assert.Contains(t, out, tc.want)
			// The ID was fetched before anything was listed, and the list was
			// of the Deployment's own Workspace.
			require.GreaterOrEqual(t, len(mc.Calls), 2)
			assert.Equal(t, "GetDeploymentWithResponse", mc.Calls[0].Method)
			mc.AssertCalled(t, "ListDeploymentsWithResponse", mock.Anything, "env-org", listedInDeploymentsWorkspace("workspace-id"))
		})
	}
}

// A Deployment ID the platform does not know is its error, and nothing is
// changed.
func TestDeploymentByIDThatDoesNotExistFailsBeforeAnyChange(t *testing.T) {
	mc := byIDRun(t)
	notFound := astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNotFound}, Body: []byte(`{"message":"not found"}`)}
	mc.On("GetDeploymentWithResponse", mock.Anything, "env-org", "clmissingdeployment000001").Return(&notFound, nil).Once()

	_, err := execDeploymentCmd("delete", "clmissingdeployment000001", "--yes")
	require.Error(t, err)
	assert.ErrorContains(t, err, "fetching Deployment clmissingdeployment000001")
	mc.AssertNotCalled(t, "DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
	mc.AssertNotCalled(t, "ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything)
}

// A Deployment named, or picked, is looked for in a Workspace, which an
// Organization token has none of: the command says so before it asks or calls
// the platform.
func TestDeploymentByNameOrPickerStillNeedsAWorkspaceUnderAnOrganizationToken(t *testing.T) {
	for name, args := range map[string][]string{
		"delete by name":                {"delete", "--deployment-name", "etl", "--yes"},
		"delete a name given as the id": {"delete", "etl", "--yes"},
		"delete picker":                 {"delete", "--yes"},
		"variable list by name":         {"variable", "list", "--deployment-name", "etl"},
		"variable list picker":          {"variable", "list"},
		"logs by name":                  {"logs", "--deployment-name", "etl", "-w"},
		"logs picker":                   {"logs", "-w"},
		"hibernate by name":             {"hibernate", "--deployment-name", "etl", "--yes"},
		"hibernate picker":              {"hibernate", "--yes"},
		"update by name":                {"update", "--deployment-name", "etl", "--yes"},
		"inspect by name":               {"inspect", "--deployment-name", "etl"},
		"inspect picker":                {"inspect"},
		"worker-queue delete by name":   {"worker-queue", "delete", "--deployment-name", "etl", "--name", "q"},
	} {
		t.Run(name, func(t *testing.T) {
			// A client with no expectations: any call is a failure.
			byIDRun(t)
			var err error
			asked := captureStderr(t, func() { _, err = execDeploymentCmd(args...) })
			require.ErrorIs(t, err, astrosession.ErrOrgTokenNoWorkspace)
			assert.ErrorContains(t, err, "--workspace")
			for _, question := range []string{"Select", "(y/n)", "Enter", "> "} {
				assert.NotContains(t, asked, question, "something was asked before the Workspace was refused")
			}
		})
	}
}

// With a Workspace known, an ID is looked for in it as ever: nothing is fetched
// ahead of the list.
func TestDeploymentByIDWithAWorkspaceIsLookedForInIt(t *testing.T) {
	mc := byIDRun(t)
	deleted := astrov1.DeleteDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
	mc.On("ListDeploymentsWithResponse", mock.Anything, "env-org", listedInDeploymentsWorkspace("flag-ws")).Return(&mockListDeploymentsResponse, nil).Once()
	mc.On("GetDeploymentWithResponse", mock.Anything, "env-org", byID).Return(&deploymentResponse, nil).Once()
	mc.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, byID).Return(&deleted, nil).Once()

	_, err := execDeploymentCmd("delete", byID, "--yes", "--workspace-id", "flag-ws")
	require.NoError(t, err)
	require.NotEmpty(t, mc.Calls)
	assert.Equal(t, "ListDeploymentsWithResponse", mc.Calls[0].Method)
	mc.AssertExpectations(t)
}
