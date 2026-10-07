package deploy

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const manifestGitCommitMessage = "ship the new dag"

// manifestGitProjectDir is manifestProjectDir committed to a git repository whose origin
// is on GitHub. dirty leaves a change to a tracked DAG file uncommitted.
func manifestGitProjectDir(t *testing.T, dirty bool) string {
	t.Helper()
	dir := manifestProjectDir(t)
	runGit := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")
	runGit("add", "-A")
	runGit("commit", "-m", manifestGitCommitMessage)
	runGit("remote", "add", "origin", "https://github.com/account/repo.git")
	if dirty {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "example.py"), []byte("# changed\n"), 0o600))
	}
	return dir
}

// captureCreateDeploy stubs the create call with deploy and returns the
// request it received once the deploy has run.
func captureCreateDeploy(client *astrov1_mocks.ClientWithResponsesInterface, deploy *astrov1.Deploy) *astrov1.CreateDeployRequest {
	got := &astrov1.CreateDeployRequest{}
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { *got = args.Get(3).(astrov1.CreateDeployRequest) }).
		Return(&astrov1.CreateDeployResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: deploy}, nil)
	return got
}

func dagDeployWithCapture(t *testing.T, in *ManifestDagDeployInput) (ManifestDagDeployResult, *astrov1.CreateDeployRequest) {
	t.Helper()
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeployment(client, true, false)
	uploadURL := "https://upload-url"
	req := captureCreateDeploy(client, &astrov1.Deploy{Id: "test-deploy-id", DagsUploadUrl: &uploadURL})
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	in.DeploymentID = "test-deployment-id"
	res, err := DeployManifestDags(*in, client)
	require.NoError(t, err)
	client.AssertExpectations(t)
	return res, req
}

func assertGitHubCommit(t *testing.T, g *astrov1.CreateDeployGitRequest) {
	t.Helper()
	require.NotNil(t, g)
	assert.Equal(t, astrov1.CreateDeployGitRequestProviderGITHUB, g.Provider)
	assert.Len(t, g.CommitSha, 40)
	require.NotNil(t, g.Branch)
	assert.Equal(t, "main", *g.Branch)
	require.NotNil(t, g.Account)
	assert.Equal(t, "account", *g.Account)
	require.NotNil(t, g.Repo)
	assert.Equal(t, "repo", *g.Repo)
	require.NotNil(t, g.CommitUrl)
	assert.Equal(t, "https://github.com/account/repo/commit/"+g.CommitSha, *g.CommitUrl)
}

func TestDeployManifestDags_RecordsTheCommit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	res, req := dagDeployWithCapture(t, &ManifestDagDeployInput{ProjectDir: manifestGitProjectDir(t, false)})

	assertGitHubCommit(t, req.Git)
	require.NotNil(t, req.Description)
	assert.Equal(t, manifestGitCommitMessage, *req.Description)
	assert.Same(t, req.Git, res.Git.Commit)
	assert.False(t, res.Git.Uncommitted)
}

func TestDeployManifestDags_KeepsAGivenDescription(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, req := dagDeployWithCapture(t, &ManifestDagDeployInput{ProjectDir: manifestGitProjectDir(t, false), Description: "hotfix"})

	assertGitHubCommit(t, req.Git)
	assert.Equal(t, "hotfix", *req.Description)
}

func TestDeployManifestDags_UncommittedChangesRecordNoCommit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	res, req := dagDeployWithCapture(t, &ManifestDagDeployInput{ProjectDir: manifestGitProjectDir(t, true)})

	assert.Nil(t, req.Git)
	assert.Empty(t, *req.Description)
	assert.Nil(t, res.Git.Commit)
	assert.True(t, res.Git.Uncommitted)
}

func TestDeployManifestDags_GitMetadataSettingOff(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	require.NoError(t, config.CFG.DeployGitMetadata.SetHomeString("false"))
	t.Cleanup(func() { _ = config.CFG.DeployGitMetadata.SetHomeString("true") })

	res, req := dagDeployWithCapture(t, &ManifestDagDeployInput{ProjectDir: manifestGitProjectDir(t, false)})

	assert.Nil(t, req.Git)
	assert.Empty(t, *req.Description)
	assert.Equal(t, ManifestDeployGit{}, res.Git)
}

func TestDeployManifestDags_OutsideAGitCheckoutRecordsNoCommit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	res, req := dagDeployWithCapture(t, &ManifestDagDeployInput{ProjectDir: manifestProjectDir(t)})

	assert.Nil(t, req.Git)
	assert.Equal(t, ManifestDeployGit{}, res.Git)
}

func imageDeployWithCapture(t *testing.T, in *ManifestImageDeployInput) (ManifestImageDeployResult, *astrov1.CreateDeployRequest) {
	t.Helper()
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	req := captureCreateDeploy(client, &astrov1.Deploy{
		Id:              "test-deploy-id",
		ImageRepository: "registry.astro/test-deployment",
		ImageTag:        "deploy-2026-07-24",
	})
	mockFinalizeDeploy(client)
	withImageSeams(t, "3.1-2")

	in.DeploymentID = "test-deployment-id"
	res, err := DeployManifestImage(*in, client)
	require.NoError(t, err)
	client.AssertExpectations(t)
	return res, req
}

func TestDeployManifestImage_RecordsTheCommit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	res, req := imageDeployWithCapture(t, &ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestGitProjectDir(t, false),
			AirflowVersion: "3.1",
		},
	})

	assertGitHubCommit(t, req.Git)
	assert.Equal(t, manifestGitCommitMessage, *req.Description)
	assert.Same(t, req.Git, res.Git.Commit)
}

func TestDeployManifestImage_PrebuiltImageRecordsNoCommit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	res, req := imageDeployWithCapture(t, &ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir: manifestGitProjectDir(t, false),
		},
		ImageName: "astro-package/demo:7.0.0-abc",
	})

	assert.Nil(t, req.Git)
	assert.Empty(t, *req.Description)
	assert.Equal(t, ManifestDeployGit{}, res.Git)
}
