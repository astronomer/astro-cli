package deploy

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/deploybuild"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// fakeImageCmd is an imagebuild.Commander that records calls and never touches a
// real daemon. It is the one runner every image action of a deploy goes
// through: the probe, the build, the label reads, the login, the tag and the
// push.
type fakeImageCmd struct {
	calls []string
	// envs are the env each call ran with, and stdins what each call that
	// was given a stdin read from it, keyed by the call's index.
	envs   [][]string
	stdins map[int]string
	err    error
	// errs fails the calls whose first argument is the key (login, push,
	// tag...) with the error.
	errs map[string]error
	// label is the value a label read of image answers; nil answers "".
	label func(image string) string
	// env is what an environment read answers, a JSON list; "" answers
	// nothing, which the read cannot parse.
	env string
	// pushOut is what a push writes to its stdout: its progress.
	pushOut string
	// pushAuth is the content of the --authfile the last push was given.
	pushAuth string
	// buildOut is what a build writes to its stdout, and buildErr how it
	// ends.
	buildOut string
	buildErr error
	// stdios are the streams each build was wired to.
	stdios []localrt.Stdio
	// dirs are the working directories RunIn was asked for.
	dirs []string
}

func (f *fakeImageCmd) RunIn(ctx context.Context, dir string, env []string, stdio localrt.Stdio, name string, args ...string) error {
	f.dirs = append(f.dirs, dir)
	return f.Run(ctx, env, stdio, name, args...)
}

func (f *fakeImageCmd) Run(_ context.Context, env []string, stdio localrt.Stdio, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.envs = append(f.envs, env)
	if stdio.In != nil {
		in, err := io.ReadAll(stdio.In)
		if err != nil {
			return err
		}
		if f.stdins == nil {
			f.stdins = map[int]string{}
		}
		f.stdins[len(f.calls)-1] = string(in)
	}
	if len(args) > 3 && args[0] == "image" && args[1] == "inspect" && strings.Contains(args[3], ".Config.Labels") && f.label != nil && stdio.Out != nil {
		fmt.Fprintln(stdio.Out, f.label(args[len(args)-1]))
	}
	if len(args) > 3 && args[0] == "image" && args[1] == "inspect" && strings.Contains(args[3], ".Config.Env") && stdio.Out != nil {
		fmt.Fprintln(stdio.Out, f.env)
	}
	if len(args) == 1 && args[0] == "--version" && stdio.Out != nil {
		fmt.Fprintf(stdio.Out, "%s version 5.6.0\n", name)
	}
	if len(args) > 2 && args[0] == "push" && args[1] == "--authfile" {
		data, _ := os.ReadFile(args[2])
		f.pushAuth = string(data)
	}
	if len(args) > 0 && args[0] == "push" && f.pushOut != "" && stdio.Out != nil {
		fmt.Fprint(stdio.Out, f.pushOut)
	}
	if len(args) > 0 && f.errs[args[0]] != nil {
		return f.errs[args[0]]
	}
	if len(args) > 0 && args[0] == "build" {
		f.stdios = append(f.stdios, stdio)
		if f.buildOut != "" && stdio.Out != nil {
			fmt.Fprint(stdio.Out, f.buildOut)
		}
		if f.buildErr != nil {
			return f.buildErr
		}
	}
	return f.err
}

// withImageSeams wires the engine and the image commander seams to fakes for
// one test, and restores them after. runtimeVersion is the label every image
// reports.
func withImageSeams(t *testing.T, runtimeVersion string) *fakeImageCmd {
	t.Helper()
	return withEngine(t, "docker", nil, runtimeVersion)
}

// withEngine is withImageSeams on the engine bin with its connection env.
func withEngine(t *testing.T, bin string, env []string, runtimeVersion string) *fakeImageCmd {
	t.Helper()
	// No push credential lands in, or is read from, the developer's docker
	// config.
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	cmd := &fakeImageCmd{label: func(string) string { return runtimeVersion }}
	origResolve := resolveContainerEngine
	origCmd := newImageBuildCommander
	resolveContainerEngine = func() (string, []string, error) { return bin, env, nil }
	newImageBuildCommander = func() imagebuild.Commander { return cmd }
	t.Cleanup(func() {
		resolveContainerEngine = origResolve
		newImageBuildCommander = origCmd
	})
	return cmd
}

// manifestPush is the push of a manifest image deploy to the repository and
// tag mockCreateImageDeploy returns.
const manifestPush = "docker push registry.astro/test-deployment:deploy-2026-07-24"

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

	cmd := withImageSeams(t, "3.1-2")

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
	assert.True(t, hasImageCall(cmd.calls, "build --tag astro-deploy/"), "expected a build, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"), "expected linux/amd64, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, manifestPush), "expected the push, got %v", cmd.calls)
	client.AssertExpectations(t)
}

// A failed install says "see the build output above", so the build's lines
// reach Progress, labeled as astro local labels them.
func TestDeployManifestImage_StreamsTheBuildOutput(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")

	cmd := withImageSeams(t, "3.1-2")
	cmd.buildOut = "#9 error: Git executable not found\n"
	cmd.buildErr = errors.New("exit status 1")

	var progress bytes.Buffer
	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build: imagebuild.ManifestBuild{
			ProjectDir:     manifestProjectDir(t),
			AirflowVersion: "3.1",
			Dependencies:   []string{"x @ git+https://example.com/x.git"},
		},
		DeploymentID: "test-deployment-id",
		Progress:     &progress,
	}, client)
	require.Error(t, err)
	assert.Contains(t, progress.String(), "[build] #9 error: Git executable not found\n")
}

func TestDeployManifestImage_ImageOnlySkipsDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "") // image-only: no upload URL needed
	mockFinalizeDeploy(client)

	cmd := withImageSeams(t, "3.1-2")

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
	assert.True(t, hasImageCall(cmd.calls, manifestPush), "expected the push, got %v", cmd.calls)
	client.AssertExpectations(t)
}

func TestDeployManifestImage_ImageNameSkipsBuild(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)

	cmd := withImageSeams(t, "3.1-2")

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
	assert.False(t, hasImageCall(cmd.calls, "build --tag"), "prebuilt image must skip the build, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "docker tag astro-package/demo:7.0.0-abc registry.astro/test-deployment:deploy-2026-07-24"), "the prebuilt image is what is pushed, got %v", cmd.calls)
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
func containerdStore(t *testing.T, runtimeVersion string) *fakeImageCmd {
	t.Helper()
	cmd := withImageSeams(t, runtimeVersion)
	cmd.label = func(image string) string {
		if hasImageCall(cmd.calls, "build --tag "+image+" ") {
			return runtimeVersion
		}
		return ""
	}
	return cmd
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

	cmd := containerdStore(t, "3.1-2")

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

	tag := deploybuild.ImageTag(dir)
	assert.True(t, hasImageCall(cmd.calls, "build --tag "+tag+" "), "nothing to install still builds, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "--platform linux/amd64"), "at linux/amd64, got %v", cmd.calls)
	assert.False(t, hasImageCall(cmd.calls, "docker pull"), "a pulled base is not what ships, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, "docker tag "+tag+" registry.astro/test-deployment:deploy-2026-07-24"), "the built tag is what is pushed, got %v", cmd.calls)
	assert.True(t, hasImageCall(cmd.calls, manifestPush), "expected the push, got %v", cmd.calls)
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

// The prefix matches what the published agent image sets (astro-agent's
// Dockerfile.standalone), and is the namespace with its separator: the
// refusal test's fake answers whatever environment it is given.
func TestAgentEnvVarPrefix(t *testing.T) {
	assert.True(t, strings.HasPrefix("ASTRO_AGENT_CLIENT_PROCESS_SPAWNER__PYTHON_EXECUTABLE", agentEnvVarPrefix))
	assert.False(t, strings.HasPrefix("ASTRO_AGENT_CLIENTS", agentEnvVarPrefix))
}

// An agent image is built FROM Runtime and passes the label check, but would
// crash-loop as the Deployment's scheduler: it is refused before the deploy
// is created, whether built or named with --image-name.
func TestDeployManifestImage_RefusesAnAgentImage(t *testing.T) {
	for name, imageName := range map[string]string{"built": "", "--image-name": "agent:1"} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			client := new(astrov1_mocks.ClientWithResponsesInterface)
			mockManifestDeploymentAt(client, "3.1-2", true, false)
			mockDeploymentOptions(client, "3.1-2")

			cmd := withImageSeams(t, "3.1-2")
			cmd.env = `["PATH=/bin","ASTRO_AGENT_CLIENT_PROCESS_SPAWNER__PYTHON_EXECUTABLE=/usr/local/bin/python"]`

			_, err := DeployManifestImage(ManifestImageDeployInput{
				Build: imagebuild.ManifestBuild{
					ProjectDir:     manifestProjectDir(t),
					AirflowVersion: "3.1",
					Dependencies:   []string{"pandas"},
				},
				DeploymentID: "test-deployment-id",
				ImageName:    imageName,
			}, client)
			require.ErrorIs(t, err, errAgentImageInHostedDeploy)
			assert.ErrorContains(t, err, "astro remote deploy")
			assert.NotContains(t, err.Error(), "show_warnings", "the refusal does not steer users to the global switch")
			client.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			assert.False(t, hasImageCall(cmd.calls, "push"), "nothing is pushed, got %v", cmd.calls)
			client.AssertExpectations(t)
		})
	}
}

// With warnings off the check is skipped, as in Astro CLI 1.x: an image whose
// variables only look like an agent's is the user's to deploy. The image is
// not even inspected.
func TestRefuseAgentImage_SkippedWithWarningsOff(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// The setting below is package state: put it back for the tests after.
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.LocalPlatform) })
	cmd := withImageSeams(t, "3.1-2")
	cmd.env = `["ASTRO_AGENT_CLIENT_PROCESS_SPAWNER__PYTHON_EXECUTABLE=/usr/local/bin/python"]`
	engine := imagebuild.Engine{Cmd: newImageBuildCommander(), Bin: "docker"}

	require.ErrorIs(t, refuseAgentImage(context.Background(), engine, "agent:1"), errAgentImageInHostedDeploy)

	require.NoError(t, config.CFG.ShowWarnings.SetHomeString("false"))
	calls := len(cmd.calls)
	assert.NoError(t, refuseAgentImage(context.Background(), engine, "agent:1"))
	assert.Len(t, cmd.calls, calls, "nothing is inspected with warnings off")
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
		return "", nil, fmt.Errorf("%w, and none exists yet; create and start one with podman machine init --now", container.ErrMachineNotRunning)
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
			cmd := withImageSeams(t, "3.2-10")

			in := tt.in
			in.OnBuild = func() { t.Error("a refused deploy must not announce a build") }
			in.Build.ProjectDir = manifestProjectDir(t)
			in.DeploymentID = "test-deployment-id"
			if tt.dockerfile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(in.Build.ProjectDir, "Dockerfile"), []byte(tt.dockerfile), 0o600))
			}

			_, err := DeployManifestImage(in, client)
			require.EqualError(t, err, tt.wantErr)
			assert.False(t, hasImageCall(cmd.calls, "build --tag"), "nothing should be built, got %v", cmd.calls)
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
	cmd := withImageSeams(t, "3.3-7")

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
	assert.True(t, hasImageCall(cmd.calls, "build --tag"), "a same-series pin is built, got %v", cmd.calls)
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

	cmd := withImageSeams(t, "3.1-2")

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

	cmd := withImageSeams(t, "3.1-2")

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
	assert.False(t, hasImageCall(cmd.calls, "build --tag"),
		"nothing should be built when the declared file cannot be read, got %v", cmd.calls)
}

// The probe, the build, the label read, the registry login and the push of a
// manifest image deploy all run on the one engine it resolves, its connection
// env included: the image pushed is the one built, from where the login was
// made. The token reaches the push in a throwaway --authfile, never as an
// argument, without the "Bearer " prefix the context stores it with.
func TestDeployManifestImage_RunsOnOneEngine(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	podmanEnv := []string{"CONTAINER_HOST=unix:///run/podman/podman.sock"}
	cmd := withEngine(t, "podman", podmanEnv, "3.1-2")

	dir := manifestProjectDir(t)
	_, err := DeployManifestImage(ManifestImageDeployInput{
		Login:        &config.Context{Domain: "localhost", Organization: "test-org-id", Token: "Bearer ctx-token"},
		Build:        imagebuild.ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1", Dependencies: []string{"pandas"}},
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)

	tag := deploybuild.ImageTag(dir)
	remote := "registry.astro/test-deployment:deploy-2026-07-24"
	verbs := make([]string, 0, len(cmd.calls))
	for i, call := range cmd.calls {
		assert.True(t, strings.HasPrefix(call, "podman "), call)
		assert.Equal(t, podmanEnv, cmd.envs[i][:1], call)
		assert.NotContains(t, call, "ctx-token", "the token is never an argument")
		verbs = append(verbs, strings.Fields(call)[1])
	}
	assert.Equal(t, []string{"info", "build", "image", "image", "tag", "--version", "push", "rmi"}, verbs, "%v", cmd.calls)
	assert.Contains(t, cmd.calls, "podman tag "+tag+" "+remote)
	push := slices.IndexFunc(cmd.calls, func(c string) bool { return strings.HasPrefix(c, "podman push --authfile ") })
	require.NotEqual(t, -1, push, "%v", cmd.calls)
	authfile := strings.Fields(cmd.calls[push])[3]
	assert.Equal(t, "podman push --authfile "+authfile+" "+remote, cmd.calls[push])
	assert.Contains(t, cmd.pushAuth, base64.StdEncoding.EncodeToString([]byte("cli:ctx-token")), "the push's credential")
	assert.NoFileExists(t, authfile, "removed after the push")
}

// A push the registry refuses is ErrPushForbidden, with its advice, and the
// deploy is not finalized.
func TestDeployManifestImage_PushForbidden(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	cmd := withImageSeams(t, "3.1-2")
	cmd.errs = map[string]error{"push": errors.New("unexpected status from PUT request: 403 Forbidden")}

	_, err := DeployManifestImage(ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: manifestProjectDir(t), AirflowVersion: "3.1", Dependencies: []string{"pandas"}},
		DeploymentID: "test-deployment-id",
	}, client)
	require.ErrorIs(t, err, imagebuild.ErrPushForbidden)
	// The push carried the session token, so the advice is about that, not
	// the user's cached logins.
	assert.Contains(t, err.Error(), "the registry refused the credential this push was made with")
	assert.Contains(t, err.Error(), "unexpected status from PUT request: 403 Forbidden")
	assert.NotContains(t, err.Error(), "docker logout")
	client.AssertNotCalled(t, "FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The push's progress goes to Progress with the build's, never to stdout:
// under --output json the command's stdout carries the result object alone.
func TestDeployManifestImage_PushProgressStaysOffStdout(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateImageDeploy(client, "")
	mockFinalizeDeploy(client)
	cmd := withImageSeams(t, "3.1-2")
	cmd.pushOut = "deploy-2026-07-24: digest: sha256:abc size: 527\n"

	var progress bytes.Buffer
	var err error
	stdout := captureStdout(t, func() {
		_, err = DeployManifestImage(ManifestImageDeployInput{
			Build:        imagebuild.ManifestBuild{ProjectDir: manifestProjectDir(t), AirflowVersion: "3.1", Dependencies: []string{"pandas"}},
			DeploymentID: "test-deployment-id",
			Progress:     &progress,
		}, client)
	})
	require.NoError(t, err)
	assert.Contains(t, progress.String(), "digest: sha256:abc")
	assert.Empty(t, stdout)
}
