package deploy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// fakeImageCmd is an imagebuild.Commander that records calls and never touches a
// real daemon.
type fakeImageCmd struct {
	calls []string
	err   error
}

func (f *fakeImageCmd) Run(_ context.Context, _ []string, _ localrt.Stdio, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.err
}

// withImageSeams wires the engine, build commander, and image handler seams to
// fakes for one test, and restores them after. runtimeVersion is the label the
// image reports.
func withImageSeams(t *testing.T, runtimeVersion string) (*fakeImageCmd, *mocks.ImageHandler) {
	t.Helper()
	cmd := &fakeImageCmd{}
	handler := new(mocks.ImageHandler)
	handler.On("GetLabel", mock.Anything, runtimeImageLabel).Return(runtimeVersion, nil).Maybe()
	handler.On("Push", mock.Anything, registryUsername, mock.Anything, mock.Anything).Return("", nil).Maybe()

	origResolve := resolveContainerEngine
	origCmd := newImageBuildCommander
	origHandler := airflowImageHandler
	resolveContainerEngine = func() (string, []string, error) { return "docker", nil, nil }
	newImageBuildCommander = func() imagebuild.Commander { return cmd }
	airflowImageHandler = func(string) airflow.ImageHandler { return handler }
	t.Cleanup(func() {
		resolveContainerEngine = origResolve
		newImageBuildCommander = origCmd
		airflowImageHandler = origHandler
	})
	return cmd, handler
}

// mockDeploymentOptions stubs the allowed-runtime list the version check reads.
func mockDeploymentOptions(client *astrov1_mocks.ClientWithResponsesInterface, versions ...string) {
	releases := make([]astrov1.RuntimeRelease, 0, len(versions))
	for _, v := range versions {
		releases = append(releases, astrov1.RuntimeRelease{Version: v})
	}
	client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentOptionsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.DeploymentOptions{RuntimeReleases: releases},
	}, nil)
}

// mockCreateImageDeploy stubs the create call with a repository and tag (and an
// upload URL for the both path).
func mockCreateImageDeploy(client *astrov1_mocks.ClientWithResponsesInterface, uploadURL string) {
	var urlPtr *string
	if uploadURL != "" {
		urlPtr = &uploadURL
	}
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.Deploy{
			Id:              "test-deploy-id",
			ImageRepository: "registry.astro/test-deployment",
			ImageTag:        "deploy-2026-07-24",
			DagsUploadUrl:   urlPtr,
		},
	}, nil)
}

func TestDeployImageV2_BuildAndImageAndDag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false) // STANDARD, runtime 7.0.0, dag deploy on
	mockDeploymentOptions(client, "7.0.0")
	mockCreateImageDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	cmd, handler := withImageSeams(t, "7.0.0")

	res, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     v2ProjectDir(t),
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		Dependencies:   []string{"pandas"},
		IncludeDags:    true,
		Description:    "a v2 image deploy",
	}, client)
	require.NoError(t, err)

	assert.Equal(t, "test-ws-id", res.WorkspaceID)
	assert.Equal(t, "7.0.0", res.RuntimeVersion)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
	assert.NotEmpty(t, res.URL)

	// Docker was probed, then a linux/amd64 build ran.
	assert.True(t, hasImageCall(cmd.calls, "docker info"), "expected a docker info probe, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "build --tag astro-deploy/"), "expected a build, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"), "expected linux/amd64, got %v", cmd.calls)
	handler.AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployImageV2_ImageOnlySkipsDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockDeploymentOptions(client, "7.0.0")
	mockCreateImageDeploy(client, "") // image-only: no upload URL needed
	mockFinalizeDeploy(client)

	_, handler := withImageSeams(t, "7.0.0")

	res, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     v2ProjectDir(t),
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		Dependencies:   []string{"pandas"},
		IncludeDags:    false,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	assert.Empty(t, res.DagTarballVersion)
	handler.AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployImageV2_ImageNameSkipsBuild(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockDeploymentOptions(client, "7.0.0")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)

	cmd, _ := withImageSeams(t, "7.0.0")

	res, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
		ImageName:    "astro-package/demo:7.0.0-abc",
		IncludeDags:  false,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	// A prebuilt image is adopted: no build command runs.
	assert.False(t, hasImageCall(cmd.calls, "build --tag"), "prebuilt image must skip the build, got %v", cmd.calls)
	client.AssertExpectations(t)
}

// containerdStore models Docker's containerd image store on a host whose
// platform differs from the build's (an arm64 Mac building linux/amd64).
//
// `docker pull --platform linux/amd64 <base>` leaves the base's tag naming its
// multi-platform index with only the amd64 variant local, and `docker image
// inspect` without --platform then reads an empty config: no labels. A tag
// produced by `docker build --platform linux/amd64` names that one platform,
// and inspect reads its labels. So a label is only readable off a tag the fake
// saw built.
func containerdStore(t *testing.T, runtimeVersion string) (cmd *fakeImageCmd, handlers map[string]*mocks.ImageHandler) {
	t.Helper()
	cmd, _ = withImageSeams(t, runtimeVersion)
	handlers = map[string]*mocks.ImageHandler{}
	airflowImageHandler = func(name string) airflow.ImageHandler {
		if h, ok := handlers[name]; ok {
			return h
		}
		label := ""
		if hasImageCall(cmd.calls, "build --tag "+name+" ") {
			label = runtimeVersion
		}
		h := new(mocks.ImageHandler)
		h.On("GetLabel", mock.Anything, runtimeImageLabel).Return(label, nil).Maybe()
		h.On("Push", mock.Anything, registryUsername, mock.Anything, mock.Anything).Return("", nil).Maybe()
		handlers[name] = h
		return h
	}
	return cmd, handlers
}

// No deps and no packages: nothing to install, and the deploy still has to
// read the runtime label and push a linux/amd64 image. Under the containerd
// store a pulled base reads no labels, so this only succeeds when the deploy
// builds a single-platform image at linux/amd64 and inspects and pushes that.
func TestDeployImageV2_NothingToInstallBuildsASinglePlatformImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockDeploymentOptions(client, "7.0.0")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)

	cmd, handlers := containerdStore(t, "7.0.0")

	dir := v2ProjectDir(t)
	res, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     dir,
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		Dependencies:   []string{"apache-airflow==3.1.*"},
		IncludeDags:    false,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)

	tag := deployImageTag(dir)
	assert.True(t, hasImageCall(cmd.calls, "build --tag "+tag+" "), "nothing to install still builds, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"), "at linux/amd64, got %v", cmd.calls)
	assert.False(t, hasImageCall(cmd.calls, "docker pull"), "a pulled base is not what ships, got %v", cmd.calls)
	require.Contains(t, handlers, tag, "the built tag is what is inspected and pushed")
	handlers[tag].AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployImageV2_RuntimeVersionRejected(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false) // deployment at 7.0.0
	mockDeploymentOptions(client, "7.0.0")

	// The built image reports 6.0.0 — a downgrade the deployment must refuse.
	withImageSeams(t, "6.0.0")

	_, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     v2ProjectDir(t),
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		Dependencies:   []string{"pandas"},
		IncludeDags:    true,
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downgrade")
	// The deploy never got created.
	client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployImageV2_NoDockerFailsEarly(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	origResolve := resolveContainerEngine
	resolveContainerEngine = func() (string, []string, error) { return "", nil, errors.New("no engine on PATH") }
	t.Cleanup(func() { resolveContainerEngine = origResolve })

	_, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     v2ProjectDir(t),
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		IncludeDags:    true,
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs Docker")
	// Nothing on the transport was touched.
	client.AssertNotCalled(t, "GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
}

func TestCheckRuntimeVersion(t *testing.T) {
	// No current version: nothing to check.
	require.NoError(t, checkRuntimeVersion("", "7.0.0", nil))
	// Supported and equal: fine.
	require.NoError(t, checkRuntimeVersion("7.0.0", "7.0.0", []string{"7.0.0"}))
	// Downgrade.
	assert.ErrorContains(t, checkRuntimeVersion("7.0.0", "6.0.0", []string{"6.0.0", "7.0.0"}), "downgrade")
	// Not in the allowed set.
	assert.ErrorContains(t, checkRuntimeVersion("7.0.0", "8.0.0", []string{"7.0.0"}), "unsupported")
}

func TestDeployImageTag(t *testing.T) {
	// Two same-named projects in different directories must not share a tag.
	a := filepath.Join(t.TempDir(), "proj")
	b := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(b, 0o755))

	tagA := deployImageTag(a)
	tagB := deployImageTag(b)
	assert.True(t, strings.HasPrefix(tagA, "astro-deploy/proj-"), "got %q", tagA)
	assert.True(t, strings.HasPrefix(tagB, "astro-deploy/proj-"), "got %q", tagB)
	assert.NotEqual(t, tagA, tagB, "same-named projects in different dirs must get different tags")

	// A path with no usable base name falls back to a stable label.
	assert.True(t, strings.HasPrefix(deployImageTag("/"), "astro-deploy/project"), "got %q", deployImageTag("/"))
}

func hasImageCall(calls []string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// Deploying a project that declared its own Dockerfile builds THAT file.
//
// Before this the v2 deploy resolved a runtime base from the manifest pin and
// built a generated image, so a tier-3 project shipped with every RUN and COPY
// step silently dropped. The DAG then worked locally, where the declaration IS
// read, and failed in the Deployment on whatever the Dockerfile installed —
// which is the worst shape for a bug like this, because local success is what
// convinces you the image is right.
func TestDeployImageV2_UsesADeclaredDockerfile(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockDeploymentOptions(client, "7.0.0")
	mockCreateImageDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	cmd, _ := withImageSeams(t, "7.0.0")

	dir := v2ProjectDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docker"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker", "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN apt-get install -y unixodbc-dev\n"), 0o600))

	_, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     dir,
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		Dependencies:   []string{"pandas"},
		Dockerfile:     "docker/Dockerfile",
		BuildSecrets:   []string{"id=tok,env=TOK"},
		IncludeDags:    true,
	}, client)
	require.NoError(t, err)

	declared := filepath.Join(dir, "docker", "Dockerfile")
	assert.True(t, hasImageCall(cmd.calls, "--file "+declared),
		"the declared file has to be the build, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--secret id=tok,env=TOK"),
		"a --build-secret reaches a declared Dockerfile's build, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"),
		"a deploy build stays linux/amd64 in Dockerfile mode too, got %v", cmd.calls)
	// The base is never resolved in this mode, so nothing is pulled for it.
	//
	// Matched as a `docker pull` COMMAND, not the substring "pull": imagebuild's
	// build can pass --pull of its own, so "pull --platform" matches the build's
	// own flags and this assertion passed against a command it was not about.
	assert.False(t, hasImageCall(cmd.calls, "docker pull"),
		"a declared Dockerfile names its own FROM; pulling a base we do not use is a needless network dependency, got %v", cmd.calls)
}

// A declaration naming nothing fails before any build, naming the path.
func TestDeployImageV2_RefusesAnUnreadableDeclaration(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockDeploymentOptions(client, "7.0.0")

	cmd, _ := withImageSeams(t, "7.0.0")

	_, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     v2ProjectDir(t),
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
		Dockerfile:     "docker/Dockerfile", // never written
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dockerfile")
	assert.False(t, hasImageCall(cmd.calls, "build --tag"),
		"nothing should be built when the declared file cannot be read, got %v", cmd.calls)
}
