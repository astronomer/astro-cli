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

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// dagDeployOffClient is a client for a Deployment with DAG deploys off. The
// create call returns no upload URL, so an upload attempt would fail.
func dagDeployOffClient() *astrov1_mocks.ClientWithResponsesInterface {
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", false, false)
	mockDeploymentOptions(client, "3.1-2")
	return client
}

// A default deploy to a Deployment with DAG deploys off ships the image alone,
// with the DAGs inside it, as the 1.x CLI did: nothing is uploaded, and the
// deploy is finalized without a tarball.
func TestDeployManifestImage_DagDeployOffBakesTheDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := dagDeployOffClient()
	mockCreateImageDeploy(client, "")
	client.On("FinalizeDeployWithResponse", mock.Anything, "test-org-id", "test-deployment-id", "test-deploy-id",
		astrov1.FinalizeDeployRequest{}).Return(&astrov1.FinalizeDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
	}, nil)

	cmd, handler := withImageSeams(t, "3.1-2")
	var warned []string
	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
			Dependencies:   []string{"pandas"},
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
		Warn:         func(w string) { warned = append(warned, w) },
	}, client)
	require.NoError(t, err)
	assert.True(t, res.DagsInImage)
	assert.Empty(t, res.DagTarballVersion)
	assert.Empty(t, warned)
	assert.True(t, hasImageCall(cmd.calls, "build --tag astro-deploy/"), "the image is built, got %v", cmd.calls)
	handler.AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	// The deploy type is the 1.x CLI's for this deploy.
	client.AssertCalled(t, "CreateDeployWithResponse", mock.Anything, "test-org-id", "test-deployment-id",
		mock.MatchedBy(func(r astrov1.CreateDeployRequest) bool { return r.Type == astrov1.CreateDeployRequestTypeIMAGEANDDAG }))
	client.AssertExpectations(t)
}

// --image leaves the running DAGs in place, and with DAG deploys off they
// are inside the image: refused, as the 1.x CLI refused it, before any build.
func TestDeployManifestImage_DagDeployOffRefusesImageOnly(t *testing.T) {
	for name, imageName := range map[string]string{"built": "", "prebuilt": "astro-package/demo:3.1-2-abc"} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			mockManifestDeploymentAt(client, "3.1-2", false, false)
			cmd, _ := withImageSeams(t, "3.1-2")

			_, err := DeployManifestImage(ManifestImageDeployInput{
				Build:        imagebuild.ManifestBuild{ProjectDir: manifestProjectDir(t), AirflowVersion: "3.1"},
				DeploymentID: "test-deployment-id",
				ImageName:    imageName,
				IncludeDags:  false,
			}, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--image deploys the image and leaves the running DAGs in place")
			assert.Contains(t, err.Error(), "astro deployment update test-deployment-id --dag-deploy enable")
			assert.False(t, hasImageCall(cmd.calls, "build --tag"), "nothing is built, got %v", cmd.calls)
			client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// A generated image whose .dockerignore leaves out every DAG would replace
// the Deployment's DAGs with none: refused before the build. A rule that
// leaves some DAGs in is the project's choice.
func TestDeployManifestImage_DagDeployOffRefusesAnImageWithoutDags(t *testing.T) {
	for _, rules := range []string{"dags/\n", "dags/**\n", "**/*.py\n"} {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		client := dagDeployOffClient()
		cmd, _ := withImageSeams(t, "3.1-2")

		dir := manifestProjectDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(rules), 0o600))
		_, err := DeployManifestImage(ManifestImageDeployInput{
			Build:        imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1"},
			DeploymentID: "test-deployment-id",
			IncludeDags:  true,
		}, client)
		require.Error(t, err, rules)
		assert.Contains(t, err.Error(), "leaves out every DAG file under dags/", rules)
		assert.False(t, hasImageCall(cmd.calls, "build --tag"), "%q: nothing is built, got %v", rules, cmd.calls)
		client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	}
}

// With DAG deploys on, the DAGs are uploaded whatever .dockerignore says.
func TestDeployManifestImage_DagDeployOnIgnoresTheDagRule(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	orig := azureUploader
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }
	t.Cleanup(func() { azureUploader = orig })
	withImageSeams(t, "3.1-2")

	dir := manifestProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/\n"), 0o600))
	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1"},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.NoError(t, err)
	assert.False(t, res.DagsInImage)
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
}

// A prebuilt image cannot be looked into: it is deployed as it is, with a
// warning that the Deployment will run the DAGs it carries.
func TestDeployManifestImage_DagDeployOffWarnsForAPrebuiltImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := dagDeployOffClient()
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	withImageSeams(t, "3.1-2")

	var warned []string
	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: manifestProjectDir(t)},
		DeploymentID: "test-deployment-id",
		ImageName:    "astro-package/demo:3.1-2-abc",
		IncludeDags:  true,
		Warn:         func(w string) { warned = append(warned, w) },
	}, client)
	require.NoError(t, err)
	assert.True(t, res.DagsInImage)
	assert.Equal(t, []string{"astro deployment test-deployment-id has DAG deploys off, so it will run the DAGs inside astro-package/demo:3.1-2-abc; no DAGs are uploaded"}, warned)
}

// A declared Dockerfile whose ignore file drops dags/ may put DAGs in the
// image some other way, so it is warned about, not refused.
func TestDeployManifestImage_DagDeployOffWarnsForADeclaredDockerfileWithoutDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := dagDeployOffClient()
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	withImageSeams(t, "3.1-2")

	dir := manifestProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("dags/\n"), 0o600))
	var warned []string
	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
		Warn:         func(w string) { warned = append(warned, w) },
	}, client)
	require.NoError(t, err)
	assert.True(t, res.DagsInImage)
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0], "leaves out every DAG file under dags/")
	assert.Contains(t, warned[0], "whatever DAGs Dockerfile itself puts in the image")

	// The same Dockerfile, its ignore file leaving dags/ in: nothing to say.
	require.NoError(t, os.Remove(filepath.Join(dir, ".dockerignore")))
	warned = nil
	_, err = DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
		Warn:         func(w string) { warned = append(warned, w) },
	}, client)
	require.NoError(t, err)
	assert.Empty(t, warned)
}
