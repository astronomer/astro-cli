package deploy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
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
	mockV2DeploymentAt(client, "7.0.0", dagDeployEnabled, cicdEnforced)
}

func mockV2DeploymentAt(client *astrov1_mocks.ClientWithResponsesInterface, runtimeVersion string, dagDeployEnabled, cicdEnforced bool) {
	standard := astrov1.DeploymentTypeSTANDARD
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deployment{
			Id:                  "test-deployment-id",
			Name:                "test-deployment",
			OrganizationId:      "test-org-id",
			WorkspaceId:         "test-ws-id",
			AstroRuntimeVersion: runtimeVersion,
			Type:                &standard,
			IsDagDeployEnabled:  dagDeployEnabled,
			IsCicdEnforced:      cicdEnforced,
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
	assert.Equal(t, "http://localhost:5000/test-ws-id/deployments/test-deployment-id", res.URL)
	client.AssertExpectations(t)
}

func TestDashboardURLHasAScheme(t *testing.T) {
	for domain, want := range map[string]string{
		"astronomer.io":     "https://cloud.astronomer.io/ws/deployments/dep",
		"astronomer-dev.io": "https://cloud.astronomer-dev.io/ws/deployments/dep",
		"localhost":         "http://localhost:5000/ws/deployments/dep",
	} {
		assert.Equal(t, want, dashboardURL(domain, "dep", "ws"), domain)
	}
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

// A deploy handed a login for another host looks the Deployment up under that
// login's organization and links to that host's cloud UI, whatever host the
// current context names.
func TestDeployDagsV2_UsesTheLoginItIsHanded(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudStagePlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	standard := astrov1.DeploymentTypeSTANDARD
	client.On("GetDeploymentWithResponse", mock.Anything, "prod-org", "test-deployment-id").Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deployment{
			Id:                  "test-deployment-id",
			OrganizationId:      "prod-org",
			WorkspaceId:         "test-ws-id",
			AstroRuntimeVersion: "7.0.0",
			Type:                &standard,
			IsDagDeployEnabled:  true,
		},
	}, nil)
	mockCreateDagDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	res, err := DeployDagsV2(DagDeployV2Input{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "prod-org", Token: "Bearer prod-token"},
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "https://cloud.astronomer.io/test-ws-id/deployments/test-deployment-id", res.URL)
	client.AssertExpectations(t)
}

// --wait polls the Deployment in the login's org, and asks its Airflow with
// the login's token, never the current context's.
func TestDeployDagsV2_WaitUsesTheLoginItIsHanded(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudStagePlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	origSleep, origTick := dagOnlyDeploySleepTime, tickNum
	dagOnlyDeploySleepTime, tickNum = 0, 1
	t.Cleanup(func() { dagOnlyDeploySleepTime, tickNum = origSleep, origTick })

	var auth string
	airflow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
	}))
	defer airflow.Close()

	standard := astrov1.DeploymentTypeSTANDARD
	client.On("GetDeploymentWithResponse", mock.Anything, "prod-org", "test-deployment-id").Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deployment{
			Id:                     "test-deployment-id",
			Name:                   "test-deployment",
			OrganizationId:         "prod-org",
			WorkspaceId:            "test-ws-id",
			AstroRuntimeVersion:    "7.0.0",
			Type:                   &standard,
			IsDagDeployEnabled:     true,
			Status:                 astrov1.DeploymentStatusHEALTHY,
			WebServerAirflowApiUrl: airflow.URL,
		},
	}, nil)
	mockCreateDagDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	_, err := DeployDagsV2(DagDeployV2Input{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "prod-org", Token: "Bearer prod-token"},
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
		Wait:         true,
		WaitTime:     30 * time.Second,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "Bearer prod-token", auth)
	client.AssertExpectations(t)
}

// Whether the monitoring DAG ships follows the org of the login handed in, not
// the current context's: a hybrid Deployment in a hosted org gets none.
func TestDeployDagsV2_MonitoringDagFollowsTheLoginsOrg(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudStagePlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	hybrid := astrov1.DeploymentTypeHYBRID
	client.On("GetDeploymentWithResponse", mock.Anything, "prod-org", "test-deployment-id").Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deployment{
			Id:                  "test-deployment-id",
			OrganizationId:      "prod-org",
			AstroRuntimeVersion: "7.0.0",
			Type:                &hybrid,
			IsDagDeployEnabled:  true,
		},
	}, nil)
	mockCreateDagDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	dir := v2ProjectDir(t)
	azureUploader = func(string, io.Reader) (string, error) {
		_, err := os.Stat(filepath.Join(dir, "dags", "astronomer_monitoring_dag.py"))
		assert.True(t, os.IsNotExist(err), "a hosted org's deploy must not add the monitoring DAG")
		return "tarball-v1", nil
	}

	_, err := DeployDagsV2(DagDeployV2Input{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "prod-org", OrganizationProduct: "HOSTED", Token: "Bearer prod-token"},
		ProjectDir:   dir,
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)
}
