package deploy

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// fakeImageCmd is an imagebuild.Commander that records calls and never touches a
// real daemon. It answers the engine probes as Docker with buildx, a current
// context "desktop" and its docker-driver builder, unless noBuildx.
type fakeImageCmd struct {
	calls    []string
	err      error
	noBuildx bool
}

func (f *fakeImageCmd) Run(_ context.Context, _ []string, s localrt.Stdio, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	switch strings.Join(args, " ") {
	case "buildx version":
		if f.noBuildx {
			return errors.New("docker: 'buildx' is not a docker command")
		}
	case "context show":
		if s.Out != nil {
			_, _ = io.WriteString(s.Out, "desktop\n")
		}
	case "buildx inspect desktop":
		if s.Out != nil {
			_, _ = io.WriteString(s.Out, "Name: desktop\nDriver: docker\n")
		}
	}
	// The ignore-file check build: export the kept file only, as a builder
	// that reads the ignore file does.
	for _, a := range args {
		if dest, ok := strings.CutPrefix(a, "type=local,dest="); ok {
			_ = os.MkdirAll(dest, 0o700)
			_ = os.MkdirAll(filepath.Join(dest, "sub"), 0o700)
			_ = os.WriteFile(filepath.Join(dest, "keep"), nil, 0o600)
			_ = os.WriteFile(filepath.Join(dest, "sub", "keep"), nil, 0o600)
		}
	}
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

func TestDeployManifestImage_BuildAndImageAndDag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false) // STANDARD, runtime 3.1-2, dag deploy on
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	cmd, handler := withImageSeams(t, "3.1-2")

	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
			Dependencies:   []string{"pandas"},
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
		Description:  "a manifest image deploy",
	}, client)
	require.NoError(t, err)

	assert.Equal(t, "test-ws-id", res.WorkspaceID)
	assert.Equal(t, "3.1-2", res.RuntimeVersion)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
	assert.Equal(t, "http://localhost:5000/test-ws-id/deployments/test-deployment-id", res.URL)

	// Docker was probed, then a linux/amd64 build ran.
	assert.True(t, hasImageCall(cmd.calls, "docker info"), "expected a docker info probe, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--tag astro-deploy/"), "expected a build, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"), "expected linux/amd64, got %v", cmd.calls)
	handler.AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployManifestImage_ImageOnlySkipsDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "") // image-only: no upload URL needed
	mockFinalizeDeploy(client)

	_, handler := withImageSeams(t, "3.1-2")

	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
			Dependencies:   []string{"pandas"},
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  false,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	assert.Empty(t, res.DagTarballVersion)
	handler.AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployManifestImage_ImageNameSkipsBuild(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)

	cmd, _ := withImageSeams(t, "3.1-2")

	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir: manifestProjectDir(t),
		},
		DeploymentID: "test-deployment-id",
		ImageName:    "astro-package/demo:7.0.0-abc",
		IncludeDags:  false,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)
	// A prebuilt image is adopted: no build command runs.
	assert.False(t, hasImageCall(cmd.calls, " --tag "), "prebuilt image must skip the build, got %v", cmd.calls)
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
		if hasImageCall(cmd.calls, "--tag "+name+" ") {
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
func TestDeployManifestImage_NothingToInstallBuildsASinglePlatformImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)

	cmd, handlers := containerdStore(t, "3.1-2")

	dir := manifestProjectDir(t)
	res, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     dir,
			AirflowVersion: "3.1",
			Dependencies:   []string{"apache-airflow==3.1.*"},
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  false,
	}, client)
	require.NoError(t, err)
	assert.Equal(t, "deploy-2026-07-24", res.ImageTag)

	tag := builtTag(t, cmd.calls, deployImageTag(dir))
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"), "at linux/amd64, got %v", cmd.calls)
	assert.False(t, hasImageCall(cmd.calls, "docker pull"), "a pulled base is not what ships, got %v", cmd.calls)
	require.Contains(t, handlers, tag, "the built tag is what is inspected and pushed")
	handlers[tag].AssertCalled(t, "Push", mock.Anything, registryUsername, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployManifestImage_RuntimeVersionRejected(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false) // deployment at 3.1-2
	mockDeploymentOptions(client, "3.1-2")

	// The built image reports 3.1-1 — a downgrade the deployment must refuse.
	withImageSeams(t, "3.1-1")

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
			Dependencies:   []string{"pandas"},
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downgrade")
	// The deploy never got created.
	client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}

func TestDeployManifestImage_NoDockerFailsEarly(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	origResolve := resolveContainerEngine
	resolveContainerEngine = func() (string, []string, error) { return "", nil, errors.New("no engine on PATH") }
	t.Cleanup(func() { resolveContainerEngine = origResolve })

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs Docker")
	// Nothing on the transport was touched.
	client.AssertNotCalled(t, "GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
}

// Podman with no machine up names the podman fix, not "Start Docker", and
// still fails before the transport.
func TestDeployManifestImage_NoPodmanMachineNamesThePodmanFix(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	origResolve := resolveContainerEngine
	resolveContainerEngine = func() (string, []string, error) {
		return "", nil, fmt.Errorf("%w, and none exists yet; create and start one with `podman machine init --now`", container.ErrMachineNotRunning)
	}
	t.Cleanup(func() { resolveContainerEngine = origResolve })

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
		},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
	}, client)
	require.ErrorIs(t, err, container.ErrMachineNotRunning)
	assert.Contains(t, err.Error(), "podman machine init --now")
	assert.Contains(t, err.Error(), "astro deploy --dags")
	assert.NotContains(t, err.Error(), "Start Docker")
	client.AssertNotCalled(t, "GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
}

func TestCheckRuntimeVersion(t *testing.T) {
	// No current version: nothing to check.
	require.NoError(t, checkRuntimeVersion("", "7.0.0", nil, nil))
	// Supported and equal: fine.
	require.NoError(t, checkRuntimeVersion("7.0.0", "7.0.0", []string{"7.0.0"}, nil))
	// Downgrade.
	assert.ErrorContains(t, checkRuntimeVersion("7.0.0", "6.0.0", []string{"6.0.0", "7.0.0"}, nil), "downgrade")
	// Not in the allowed set.
	assert.ErrorContains(t, checkRuntimeVersion("7.0.0", "8.0.0", []string{"7.0.0"}, nil), "unsupported")
	// A downgrade names the fix when the caller knows it.
	err := checkRuntimeVersion("3.3-8", "3.3-7", []string{"3.3-7", "3.3-8"}, func(minimum string) string { return "use " + minimum })
	assert.EqualError(t, err, "cannot deploy Astro Runtime 3.3-7: it is a downgrade from the deployment's current 3.3-8; to deploy, use 3.3-8")
}

func TestCheckPlannedRuntime(t *testing.T) {
	offered := []string{"3.2-10", "3.3-7", "3.3-8", "3.10-1"}
	raise := func(minimum string) string { return "raise to " + minimum }
	tests := []struct {
		name    string
		current string
		planned plannedRuntime
		wantErr string
	}{
		{name: "nothing planned", current: "3.3-8", planned: plannedRuntime{}},
		{name: "no current version", current: "", planned: plannedRuntime{version: "3.2", series: true}},
		{name: "older series", current: "3.3-8", planned: plannedRuntime{version: "3.2", series: true, raise: raise}, wantErr: "cannot deploy Astro Runtime 3.2: it is a downgrade from the deployment's current 3.3-8; to deploy, raise to 3.3-8"},
		{name: "same series leaves the patch to the label", current: "3.3-8", planned: plannedRuntime{version: "3.3", series: true}},
		{name: "series not offered", current: "3.3-8", planned: plannedRuntime{version: "3.5", series: true}, wantErr: "unsupported"},
		{name: "series compared as numbers", current: "3.9-1", planned: plannedRuntime{version: "3.10", series: true}},
		{name: "series over an Airflow 2 deployment below the floor", current: "11.0.0", planned: plannedRuntime{version: "3.3", series: true}, wantErr: "Airflow 2 to Airflow 3"},
		{name: "series over an Airflow 2 deployment at the floor", current: "12.1.0", planned: plannedRuntime{version: "3.3", series: true}},
		{name: "exact downgrade", current: "3.3-8", planned: plannedRuntime{version: "3.3-7", raise: raise}, wantErr: "downgrade from the deployment's current 3.3-8; to deploy, raise to 3.3-8"},
		{name: "exact not offered", current: "3.3-8", planned: plannedRuntime{version: "3.3-9"}, wantErr: "unsupported"},
		{name: "exact fine", current: "3.3-7", planned: plannedRuntime{version: "3.3-8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPlannedRuntime(tt.current, &tt.planned, offered)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPlanRuntimeReadsTheGeneratedBase(t *testing.T) {
	series := planRuntime(&ManifestImageDeployInput{}, &imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.3"})
	assert.Equal(t, "3.3", series.version)
	assert.True(t, series.series)

	exact := planRuntime(&ManifestImageDeployInput{}, &imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.3-8-python-3.13"})
	assert.Equal(t, "3.3-8", exact.version)
	assert.False(t, exact.series)
	assert.Equal(t, "set [tool.astro] runtime to 3.3-9 or newer in pyproject.toml", exact.raise("3.3-9"))
}

// A deploy the deployment would refuse for its runtime is refused before the
// build, which is minutes for a real project, and names what to change.
func TestDeployManifestImage_RefusesAnOlderRuntimeBeforeBuilding(t *testing.T) {
	tests := []struct {
		name       string
		in         ManifestImageDeployInput
		dockerfile string
		wantErr    string
	}{
		{
			name:    "an older Airflow pin",
			in:      ManifestImageDeployInput{Build: imagebuild.ManifestBuild{AirflowVersion: "3.2"}},
			wantErr: "cannot deploy Astro Runtime 3.2: it is a downgrade from the deployment's current 3.3-8; to deploy, pin apache-airflow to 3.3 or newer in pyproject.toml",
		},
		{
			name:    "an older runtime build in another series",
			in:      ManifestImageDeployInput{Build: imagebuild.ManifestBuild{AirflowVersion: "3.2", Runtime: "3.2-10"}},
			wantErr: "cannot deploy Astro Runtime 3.2-10: it is a downgrade from the deployment's current 3.3-8; to deploy, pin apache-airflow to 3.3 and set [tool.astro] runtime to 3.3-8 or newer in pyproject.toml",
		},
		{
			name:    "an older runtime build",
			in:      ManifestImageDeployInput{Build: imagebuild.ManifestBuild{AirflowVersion: "3.3", Runtime: "3.3-7"}},
			wantErr: "cannot deploy Astro Runtime 3.3-7: it is a downgrade from the deployment's current 3.3-8; to deploy, set [tool.astro] runtime to 3.3-8 or newer in pyproject.toml",
		},
		{
			name:       "a Dockerfile FROM an older runtime",
			in:         ManifestImageDeployInput{Build: imagebuild.ManifestBuild{AirflowVersion: "3.2", Dockerfile: "Dockerfile"}},
			dockerfile: "FROM astrocrpublic.azurecr.io/runtime:3.2-10-python-3.12\n",
			wantErr:    "cannot deploy Astro Runtime 3.2-10: it is a downgrade from the deployment's current 3.3-8; to deploy, change the FROM line in Dockerfile to Astro Runtime 3.3-8 or newer, and pin apache-airflow to 3.3 in pyproject.toml",
		},
		{
			name:       "a Dockerfile whose final stage is FROM an older series tag",
			in:         ManifestImageDeployInput{Build: imagebuild.ManifestBuild{AirflowVersion: "3.2", Dockerfile: "Dockerfile"}},
			dockerfile: "FROM astrocrpublic.azurecr.io/runtime:3.3 AS deps\nFROM astrocrpublic.azurecr.io/runtime:3.2\n",
			wantErr:    "cannot deploy Astro Runtime 3.2: it is a downgrade from the deployment's current 3.3-8; to deploy, change the FROM line in Dockerfile to Astro Runtime 3.3-8 or newer, and pin apache-airflow to 3.3 in pyproject.toml",
		},
		{
			name:    "a series the deployment does not offer",
			in:      ManifestImageDeployInput{Build: imagebuild.ManifestBuild{AirflowVersion: "3.5"}},
			wantErr: "cannot deploy unsupported Astro Runtime 3.5; supported versions: 3.2-10, 3.3-8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			mockManifestDeploymentAt(client, "3.3-8", true, false)
			mockDeploymentOptions(client, "3.2-10", "3.3-8")
			cmd, _ := withImageSeams(t, "3.2-10")

			in := tt.in
			in.OnBuild = func() { t.Error("a refused deploy must not announce a build") }
			in.Build.ProjectDir = manifestProjectDir(t)
			in.DeploymentID = "test-deployment-id"
			if tt.dockerfile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(in.Build.ProjectDir, "Dockerfile"), []byte(tt.dockerfile), 0o600))
			}

			_, err := DeployManifestImage(in, client)
			require.EqualError(t, err, tt.wantErr)
			assert.False(t, hasImageCall(cmd.calls, " --tag "), "nothing should be built, got %v", cmd.calls)
			client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// The pin's series tag can serve an older build than the deployment runs. Only
// the label shows that, and the pin is already right, so the fix is a runtime
// build rather than a newer pin.
func TestDeployManifestImage_ASeriesTagBehindTheDeploymentNamesARuntimeBuild(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.3-8", true, false)
	mockDeploymentOptions(client, "3.3-7", "3.3-8")
	cmd, _ := withImageSeams(t, "3.3-7")

	built := false
	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.3",
		},
		DeploymentID: "test-deployment-id",
		OnBuild:      func() { built = true },
	}, client)
	require.EqualError(t, err, "cannot deploy Astro Runtime 3.3-7: it is a downgrade from the deployment's current 3.3-8; to deploy, set [tool.astro] runtime to 3.3-8 or newer in pyproject.toml")
	assert.True(t, built, "the build was announced before it ran")
	assert.True(t, hasImageCall(cmd.calls, " --tag "), "a same-series pin is built, got %v", cmd.calls)
	client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// A prebuilt image is only known by its label, and the refusal names how to
// rebuild it.
func TestDeployManifestImage_ImageNameDowngradeNamesTheFix(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.3-8", true, false)
	mockDeploymentOptions(client, "3.2-10", "3.3-8")
	withImageSeams(t, "3.2-10")

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir: manifestProjectDir(t),
		},
		DeploymentID: "test-deployment-id",
		ImageName:    "astro-package/demo:3.2-10-abc",
	}, client)
	require.EqualError(t, err, "cannot deploy Astro Runtime 3.2-10: it is a downgrade from the deployment's current 3.3-8; to deploy, rebuild astro-package/demo:3.2-10-abc FROM Astro Runtime 3.3-8 or newer")
	client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
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
// Before this the manifest deploy resolved a runtime base from the manifest pin and
// built a generated image, so a tier-3 project shipped with every RUN and COPY
// step silently dropped. The DAG then worked locally, where the declaration IS
// read, and failed in the Deployment on whatever the Dockerfile installed —
// which is the worst shape for a bug like this, because local success is what
// convinces you the image is right.
func TestDeployManifestImage_UsesADeclaredDockerfile(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "https://upload-url")
	mockFinalizeDeploy(client)
	azureUploader = func(string, io.Reader) (string, error) { return "tarball-v1", nil }

	cmd, _ := withImageSeams(t, "3.1-2")

	dir := manifestProjectDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docker"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker", "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN apt-get install -y unixodbc-dev\n"), 0o600))

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     dir,
			AirflowVersion: "3.1",
			Dependencies:   []string{"pandas"},
			Dockerfile:     "docker/Dockerfile",
		},
		DeploymentID: "test-deployment-id",
		BuildSecrets: []string{"id=tok,env=TOK"},
		IncludeDags:  true,
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
func TestDeployManifestImage_RefusesAnUnreadableDeclaration(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")

	cmd, _ := withImageSeams(t, "3.1-2")

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
			Dockerfile:     "docker/Dockerfile", // never written
		},
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dockerfile")
	assert.False(t, hasImageCall(cmd.calls, " --tag "),
		"nothing should be built when the declared file cannot be read, got %v", cmd.calls)
}

// builtTag is the tag the deploy built its final image under, in repo: the
// one that is not the intermediate -deps tag.
func builtTag(t *testing.T, calls []string, repo string) string {
	t.Helper()
	for _, c := range calls {
		_, rest, ok := strings.Cut(c, " --tag "+repo+":")
		if !ok {
			continue
		}
		tag, _, _ := strings.Cut(rest, " ")
		if !strings.HasSuffix(tag, "-deps") {
			return repo + ":" + tag
		}
	}
	t.Fatalf("nothing was built under %s: %v", repo, calls)
	return ""
}
