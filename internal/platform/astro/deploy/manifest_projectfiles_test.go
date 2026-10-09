package deploy

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// contextListingCmd is fakeImageCmd that also lists the build context a
// `build` was handed, while it still exists: the deploy removes its build
// directory as soon as the build returns.
type contextListingCmd struct {
	fakeImageCmd
	context []string
}

func (f *contextListingCmd) Run(ctx context.Context, env []string, s localrt.Stdio, name string, args ...string) error {
	if len(args) > 0 && args[0] == "build" {
		dir := args[len(args)-1]
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			f.context = append(f.context, filepath.ToSlash(rel))
			return nil
		})
		sort.Strings(f.context)
	}
	return f.fakeImageCmd.Run(ctx, env, s, name, args...)
}

// withContextListing replaces withImageSeams' build commander with one that
// lists what each build's context held.
func withContextListing(t *testing.T) *contextListingCmd {
	t.Helper()
	withImageSeams(t, "3.1-2")
	cmd := &contextListingCmd{}
	newImageBuildCommander = func() imagebuild.Commander { return cmd }
	return cmd
}

// projectWithCode is a project with a DAG, a plugin and an include file.
func projectWithCode(t *testing.T) string {
	t.Helper()
	dir := manifestProjectDir(t)
	for name, body := range map[string]string{"plugins/x.py": "X = 1\n", "include/y.sql": "select 1;\n"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

// mockDeploymentWith stubs the Deployment lookup with dep's DAG settings over
// the usual fixture.
func mockDeploymentWith(client *astrov1_mocks.ClientWithResponsesInterface, dagDeploy, remoteExecution bool) {
	standard := astrov1.DeploymentTypeSTANDARD
	dep := &astrov1.Deployment{
		Id:                  "test-deployment-id",
		Name:                "test-deployment",
		OrganizationId:      "test-org-id",
		WorkspaceId:         "test-ws-id",
		AstroRuntimeVersion: "3.1-2",
		Type:                &standard,
		IsDagDeployEnabled:  dagDeploy,
	}
	if remoteExecution {
		dep.RemoteExecution = &astrov1.DeploymentRemoteExecution{Enabled: true}
	}
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      dep,
	}, nil)
}

// countUploads replaces the DAG uploader for one test and counts its calls.
func countUploads(t *testing.T) *int {
	t.Helper()
	orig := azureUploader
	n := 0
	azureUploader = func(string, io.Reader) (string, error) { n++; return "tarball-v1", nil }
	t.Cleanup(func() { azureUploader = orig })
	return &n
}

func manifestBuildOf(dir string) imagebuild.ManifestBuild {
	return imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dependencies: []string{"pandas"}}
}

// A Deployment that takes DAG uploads gets plugins/ and include/ in the image
// and its DAGs as the upload, as the 1.x path builds it without dags/.
func TestDeployManifestImage_DagDeployShipsPluginsAndIncludeInTheImageAndUploadsDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	uploads := countUploads(t)
	cmd := withContextListing(t)

	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        manifestBuildOf(projectWithCode(t)),
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.NoError(t, err)

	assert.Equal(t, []string{"include/y.sql", "packages.txt", "plugins/x.py", "requirements.txt"}, cmd.context)
	assert.Equal(t, 1, *uploads)
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
}

// A Deployment that takes no DAG uploads runs the image's DAGs, so a "both"
// deploy bakes dags/ in and uploads nothing, as the 1.x path does, rather than
// refusing.
func TestDeployManifestImage_NoDagDeployBakesDagsIntoTheImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, false, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	uploads := countUploads(t)
	cmd := withContextListing(t)

	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        manifestBuildOf(projectWithCode(t)),
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.NoError(t, err)

	assert.Equal(t, []string{"dags/example.py", "include/y.sql", "packages.txt", "plugins/x.py", "requirements.txt"}, cmd.context)
	assert.Zero(t, *uploads, "nothing is uploaded to a Deployment that takes no DAG deploys")
	assert.Empty(t, res.DagTarballVersion)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	// The deploy is recorded as the 1.x path records it, image and DAGs, and
	// finalized with no bundle.
	client.AssertCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(r astrov1.CreateDeployRequest) bool {
		return r.Type == astrov1.CreateDeployRequestTypeIMAGEANDDAG
	}))
	client.AssertCalled(t, "FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(r astrov1.FinalizeDeployRequest) bool {
		return r.DagTarballVersion == nil
	}))
}

// --image leaves the running DAGs in place, which a Deployment whose DAGs are
// in its image cannot do: a new image without them would remove them. The 1.x
// path refuses it, before anything is built.
func TestDeployManifestImage_ImageOnlyIsRefusedWithoutDagDeploy(t *testing.T) {
	for name, imageName := range map[string]string{"built": "", "prebuilt": "astro-package/demo:latest"} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			mockDeploymentWith(client, false, false)
			cmd := withContextListing(t)

			_, err := DeployManifestImage(ManifestImageDeployInput{
				Build:        manifestBuildOf(projectWithCode(t)),
				DeploymentID: "test-deployment-id",
				ImageName:    imageName,
				IncludeDags:  false,
			}, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "without --image")
			assert.Contains(t, err.Error(), "astro deployment update test-deployment-id --dag-deploy enable")
			assert.False(t, hasImageCall(cmd.calls, "build --tag"), "refused before the build, got %v", cmd.calls)
			client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// With DAG deploys on, --image keeps DAGs out of the image and uploads none.
func TestDeployManifestImage_ImageOnlyWithDagDeployShipsNoDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	uploads := countUploads(t)
	cmd := withContextListing(t)

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        manifestBuildOf(projectWithCode(t)),
		DeploymentID: "test-deployment-id",
		IncludeDags:  false,
	}, client)
	require.NoError(t, err)

	assert.Equal(t, []string{"include/y.sql", "packages.txt", "plugins/x.py", "requirements.txt"}, cmd.context)
	assert.Zero(t, *uploads)
}

// Remote execution runs the DAGs elsewhere, so, as on the 1.x path, the image
// carries none even with DAG deploys off, and --image is not refused.
func TestDeployManifestImage_RemoteExecutionKeepsDagsOutOfTheImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, false, true)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	uploads := countUploads(t)
	cmd := withContextListing(t)

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        manifestBuildOf(projectWithCode(t)),
		DeploymentID: "test-deployment-id",
		IncludeDags:  false,
	}, client)
	require.NoError(t, err)

	assert.Equal(t, []string{"include/y.sql", "packages.txt", "plugins/x.py", "requirements.txt"}, cmd.context)
	assert.Zero(t, *uploads)
}

// A prebuilt image to a Deployment without DAG deploys ships as it is, with
// whatever DAGs it carries, and nothing is uploaded.
func TestDeployManifestImage_PrebuiltImageWithoutDagDeployUploadsNothing(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, false, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	uploads := countUploads(t)
	cmd := withContextListing(t)

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: projectWithCode(t)},
		DeploymentID: "test-deployment-id",
		ImageName:    "astro-package/demo:latest",
		IncludeDags:  true,
	}, client)
	require.NoError(t, err)
	assert.False(t, hasImageCall(cmd.calls, "build --tag"), "a prebuilt image is not built, got %v", cmd.calls)
	assert.Zero(t, *uploads)
}

// A declared Dockerfile's context is the project already: nothing is staged.
func TestDeployManifestImage_DeclaredDockerfileStagesNothing(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockDeploymentWith(client, false, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	cmd := withContextListing(t)
	dir := projectWithCode(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600))

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.NoError(t, err)
	var build string
	for _, c := range cmd.calls {
		if strings.HasPrefix(c, "docker build") {
			build = c
		}
	}
	assert.True(t, strings.HasSuffix(build, " "+dir), "the project is the context: %q", build)
}
