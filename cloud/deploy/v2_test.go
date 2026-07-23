package deploy

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// v2ProjectDir builds a throwaway v2 project with a dags/ directory holding one
// DAG file, and returns its root.
func v2ProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "example.py"), []byte("# dag\n"), 0o600))
	return dir
}

// mockV2Deployment stubs GetDeploymentWithResponse (the call behind
// deployment.GetDeploymentByID) with a STANDARD deployment, so the dags
// transport skips the monitoring-DAG injection.
func mockV2Deployment(client *astrov1_mocks.ClientWithResponsesInterface, dagDeployEnabled, cicdEnforced bool) {
	standard := astrov1.DeploymentTypeSTANDARD
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deployment{
			Id:                 "test-deployment-id",
			Name:               "test-deployment",
			OrganizationId:     "test-org-id",
			WorkspaceId:        "test-ws-id",
			RuntimeVersion:     "7.0.0",
			Type:               &standard,
			IsDagDeployEnabled: dagDeployEnabled,
			IsCicdEnforced:     cicdEnforced,
		},
	}, nil)
}

func mockCreateDagDeploy(client *astrov1_mocks.ClientWithResponsesInterface, uploadURL string) {
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deploy{
			Id:            "test-deploy-id",
			DagsUploadUrl: &uploadURL,
		},
	}, nil)
}

func mockFinalizeDeploy(client *astrov1_mocks.ClientWithResponsesInterface) {
	client.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.FinalizeDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
	}, nil)
}

func TestDeployDagsV2_Success(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockCreateDagDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "tarball-v1", nil
	}

	res, err := DeployDagsV2(DagDeployV2Input{
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
		Description:  "a v2 dags deploy",
	}, client)
	require.NoError(t, err)

	assert.Equal(t, "test-ws-id", res.WorkspaceID)
	assert.Equal(t, "7.0.0", res.RuntimeVersion)
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
	assert.NotEmpty(t, res.URL)
	client.AssertExpectations(t)
}

func TestDeployDagsV2_DagDeployDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, false, false)

	_, err := DeployDagsV2(DagDeployV2Input{
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DAG-only deploys are not enabled")
	client.AssertExpectations(t)
}

func TestDeployDagsV2_CiCdEnforcedBlocks(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, true)
	canCiCdDeploy = func(token string) bool { return false }

	_, err := DeployDagsV2(DagDeployV2Input{
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	client.AssertExpectations(t)
}

func TestDeployDagsV2_NoUploadURL(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockCreateDagDeploy(client, "") // server returned no upload URL

	_, err := DeployDagsV2(DagDeployV2Input{
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload URL")
	client.AssertExpectations(t)
}
