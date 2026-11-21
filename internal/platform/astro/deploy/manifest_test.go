package deploy

import (
	"bytes"
	"context"
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

// manifestProjectDir builds a throwaway project with a dags/ directory holding one
// DAG file, and returns its root.
func manifestProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "example.py"), []byte("# dag\n"), 0o600))
	return dir
}

// mockManifestDeployment stubs GetDeploymentWithResponse (the call behind
// deployment.GetDeploymentByID) with a STANDARD deployment, so the dags
// transport skips the monitoring-DAG injection.
func mockManifestDeployment(client *astrov1_mocks.ClientWithResponsesInterface, dagDeployEnabled, cicdEnforced bool) {
	mockManifestDeploymentAt(client, "7.0.0", dagDeployEnabled, cicdEnforced)
}

func mockManifestDeploymentAt(client *astrov1_mocks.ClientWithResponsesInterface, runtimeVersion string, dagDeployEnabled, cicdEnforced bool) {
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

func createdDeploy(uploadURL string) *astrov1.CreateDeployResponse {
	return &astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deploy{
			Id:            "test-deploy-id",
			DagsUploadUrl: &uploadURL,
		},
	}
}

func finalizedDeploy() *astrov1.FinalizeDeployResponse {
	return &astrov1.FinalizeDeployResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
}

func mockCreateDagDeploy(client *astrov1_mocks.ClientWithResponsesInterface, uploadURL string) {
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(createdDeploy(uploadURL), nil)
}

func mockFinalizeDeploy(client *astrov1_mocks.ClientWithResponsesInterface) {
	client.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(finalizedDeploy(), nil)
}

func TestDeployManifestDags_Success(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeployment(client, true, false)
	mockCreateDagDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)

	azureUploader = func(_ context.Context, sasLink string, file io.Reader) (string, error) {
		return "tarball-v1", nil
	}

	res, err := DeployManifestDags(ManifestDagDeployInput{
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
		Description:  "a manifest dags deploy",
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

func TestDeployManifestDags_DagDeployDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeployment(client, false, false)

	_, err := DeployManifestDags(ManifestDagDeployInput{
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dag-only deploys are not enabled")
	client.AssertExpectations(t)
}

func TestDeployManifestDags_CiCdEnforcedBlocks(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeployment(client, true, true)
	canCiCdDeploy = func(token string) bool { return false }

	_, err := DeployManifestDags(ManifestDagDeployInput{
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	client.AssertExpectations(t)
}

func TestDeployManifestDags_NoUploadURL(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeployment(client, true, false)
	mockCreateDagDeploy(client, "") // server returned no upload URL

	_, err := DeployManifestDags(ManifestDagDeployInput{
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload URL")
	client.AssertExpectations(t)
}

// A deploy handed a login for another host looks the Deployment up under that
// login's organization and links to that host's cloud UI, whatever host the
// current context names.
func TestDeployManifestDags_UsesTheLoginItIsHanded(t *testing.T) {
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
	azureUploader = func(context.Context, string, io.Reader) (string, error) { return "tarball-v1", nil }

	res, err := DeployManifestDags(ManifestDagDeployInput{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "prod-org", Token: "Bearer prod-token"},
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "https://cloud.astronomer.io/test-ws-id/deployments/test-deployment-id", res.URL)
	client.AssertExpectations(t)
}

// --wait polls the Deployment in the login's org, and asks its Airflow with
// the login's token, never the current context's.
func TestDeployManifestDags_WaitUsesTheLoginItIsHanded(t *testing.T) {
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
	azureUploader = func(context.Context, string, io.Reader) (string, error) { return "tarball-v1", nil }

	var progress bytes.Buffer
	_, err := DeployManifestDags(ManifestDagDeployInput{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "prod-org", Token: "Bearer prod-token"},
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
		Wait:         true,
		WaitTime:     30 * time.Second,
		Progress:     &progress,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "Bearer prod-token", auth)
	// The wait's progress goes where the caller says (stderr, from cmd), so
	// stdout stays the deploy's result.
	assert.Contains(t, progress.String(), "Waiting for the Deployment to become healthy")
	assert.Contains(t, progress.String(), "Deployment test-deployment is now healthy")
	client.AssertExpectations(t)
}

// Whether the monitoring DAG ships follows the org of the login handed in, not
// the current context's: a hybrid Deployment in a hosted org gets none.
func TestDeployManifestDags_MonitoringDagFollowsTheLoginsOrg(t *testing.T) {
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
	dir := manifestProjectDir(t)
	azureUploader = func(context.Context, string, io.Reader) (string, error) {
		_, err := os.Stat(filepath.Join(dir, "dags", "astronomer_monitoring_dag.py"))
		assert.True(t, os.IsNotExist(err), "a hosted org's deploy must not add the monitoring DAG")
		return "tarball-v1", nil
	}

	_, err := DeployManifestDags(ManifestDagDeployInput{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "prod-org", OrganizationProduct: "HOSTED", Token: "Bearer prod-token"},
		ProjectDir:   dir,
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)
}

// The monitoring Dag goes at a name the CLI picks in the project's dags/, so a
// symlink already there (a project's, pointing elsewhere) is replaced rather
// than written through: the file it points at keeps its content.
func TestWriteMonitoringDagReplacesASymlinkInsteadOfFollowingIt(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "precious")
	require.NoError(t, os.WriteFile(outside, []byte("keep me"), 0o600))
	path := filepath.Join(t.TempDir(), "dags", "astronomer_monitoring_dag.py")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	require.NoError(t, writeMonitoringDag(path, af3MonitoringDag))

	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(got), "the symlink's target was overwritten")
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "the monitoring Dag is a file of its own, not a link")
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, af3MonitoringDag, string(got))
}

// Through a whole Dag deploy that carries the monitoring Dag: a symlink at its
// name leaves the file it points at untouched, and the deploy removes what it
// wrote.
func TestDeployManifestDags_MonitoringDagNeverWritesThroughASymlink(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudStagePlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	hybrid := astrov1.DeploymentTypeHYBRID
	client.On("GetDeploymentWithResponse", mock.Anything, "hybrid-org", "test-deployment-id").Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deployment{
			Id:                  "test-deployment-id",
			OrganizationId:      "hybrid-org",
			AstroRuntimeVersion: "7.0.0",
			Type:                &hybrid,
			IsDagDeployEnabled:  true,
		},
	}, nil)
	mockCreateDagDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	dir := manifestProjectDir(t)
	outside := filepath.Join(t.TempDir(), "precious")
	require.NoError(t, os.WriteFile(outside, []byte("keep me"), 0o600))
	link := filepath.Join(dir, "dags", "astronomer_monitoring_dag.py")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	azureUploader = func(context.Context, string, io.Reader) (string, error) {
		info, err := os.Lstat(link)
		require.NoError(t, err, "the monitoring Dag is in the upload")
		assert.True(t, info.Mode().IsRegular(), "the upload carries the monitoring Dag itself, not the link")
		return "tarball-v1", nil
	}

	_, err := DeployManifestDags(ManifestDagDeployInput{
		Login:        &config.Context{Domain: "astronomer.io", Organization: "hybrid-org", OrganizationProduct: "HYBRID", Token: "Bearer hybrid-token"},
		ProjectDir:   dir,
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)

	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(got), "the symlink's target was overwritten")
	_, err = os.Lstat(link)
	assert.True(t, os.IsNotExist(err), "the deploy removes the monitoring Dag it wrote")
}
