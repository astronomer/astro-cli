package apc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/project"
)

// A prebuilt image's deploy, from a project's root or from outside any
// project, hands the image deploy what the flags say, and fails as it fails.
func TestDeployImageNameHandsOverTheImage(t *testing.T) {
	t.Run("--image-name, from outside any project, to a Deployment by id", func(t *testing.T) {
		inWorkingDir(t, true)
		seen := deployMocks(t, deployPushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--image-name", "img:1", "-f", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		require.Len(t, seen.images, 1)
		in := seen.images[0]
		assert.Equal(t, "test-deployment-id", in.DeploymentID)
		assert.Equal(t, deployWorkspace, in.WorkspaceID)
		assert.Equal(t, "img:1", in.ImageName)
		assert.True(t, in.ImageAlone, "outside a project the image ships alone")
		assert.False(t, in.IncludeDags)
		assert.Equal(t, "Deployed via <astro deploy>", in.Description)
	})

	t.Run("--workspace-id names the workspace", func(t *testing.T) {
		inWorkingDir(t, true)
		seen := deployMocks(t, deployPushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--image-name=test-image-name", "--workspace-id=ws-flag", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		require.Len(t, seen.images, 1)
		assert.Equal(t, "ws-flag", seen.images[0].WorkspaceID)
		assert.Equal(t, "test-image-name", seen.images[0].ImageName)
	})

	t.Run("--remote, from a project, with a description", func(t *testing.T) {
		inProject(t, true)
		seen := deployMocks(t, deployPushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "--description", "Initial deployment", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		require.Len(t, seen.images, 1)
		in := seen.images[0]
		assert.True(t, in.InRegistry)
		assert.Equal(t, "12.1.1", in.RuntimeVersion)
		assert.True(t, in.IncludeDags, "from a project, the DAGs follow the image")
		assert.False(t, in.ImageAlone)
		assert.Equal(t, "Initial deployment", in.Description)
	})

	t.Run("the image deploy's failure is the deploy's", func(t *testing.T) {
		inProject(t, true)
		for _, want := range []error{deploy.ErrNoWorkspaceID, deploy.ErrDeploymentTypeIncorrectForImageOnly, deploy.ErrBYORegistryDomainNotSet} {
			deployMocks(t, deployPushed, nil)
			DeployManifestImage = func(houston.ClientInterface, *deploy.ManifestImage) (deploy.ManifestImageResult, error) {
				return deploy.ManifestImageResult{}, want
			}
			run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--image-name", "img:1", "-o", "json")
			assert.Equal(t, 1, run.code)
			assert.ErrorIs(t, run.err, want)
		}
	})

	t.Run("--dags and --image together", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, deployPushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--dags", "--image", "-o", "json")
		assert.ErrorIs(t, run.err, ErrBothDagsOnlyAndImageOnlySet)
	})

	t.Run("--dags on a cluster without DAG-only deploys", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, deployPushed, deploy.ErrDagOnlyDeployDisabledInConfig)
		run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--dags", "--force", "-o", "json")
		assert.ErrorIs(t, run.err, deploy.ErrDagOnlyDeployDisabledInConfig)
	})
}

// The image flags that cannot go together are refused before anything is
// read or asked: --remote names no image of its own, and an image in a
// registry has no label to read its runtime from. --runtime-version without
// --remote, which 1.x took and read nothing of, is taken, and said to do
// nothing.
func TestDeployImageFlagRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want error
	}{
		{"--remote without --image-name", []string{"--remote", "--runtime-version", "12.1.1"}, ErrImageNameNotPassedForRemoteFlag},
		{"--remote with an empty --image-name", []string{"--remote", "--image-name=", "--runtime-version", "12.1.1"}, ErrImageNameNotPassedForRemoteFlag},
		{"--remote without --runtime-version", []string{"--remote", "--image-name", "img:1"}, deploy.ErrRuntimeVersionNotPassedForRemoteImage},
	} {
		for _, where := range []func(*testing.T, bool) string{inProject, inWorkingDir} {
			t.Run(tc.name, func(t *testing.T) {
				where(t, true)
				seen := deployMocks(t, deployPushed, nil)
				api := newAPCClient()
				run := runAPC(t, api, "", append([]string{"deploy", "test-deployment-id", "-o", "json"}, tc.args...)...)
				assert.Equal(t, 1, run.code)
				assert.ErrorIs(t, run.err, tc.want)
				assert.Empty(t, seen.images)
				for _, method := range []string{"GetWorkspace", "ListDeployments", "GetDeployment", "UpdateDeploymentImage"} {
					api.AssertNotCalled(t, method, mock.Anything)
				}
			})
		}
	}

	t.Run("--runtime-version without --remote", func(t *testing.T) {
		inProject(t, true)
		seen := deployMocks(t, deployPushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "test-deployment-id", "--image-name", "img:1", "--runtime-version", "12.1.1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, []string{"--runtime-version has no effect without --remote: the runtime of an image on this machine, or of one built, is read from its label"}, got.Warnings)
		require.Len(t, seen.images, 1)
		assert.Empty(t, seen.images[0].RuntimeVersion)
		assert.False(t, seen.images[0].InRegistry)
	})
}

// in1xProject points the deploy at a project in the Astro CLI 1.x layout.
func in1xProject(t *testing.T) string {
	t.Helper()
	dir := inWorkingDir(t, true)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM quay.io/astronomer/astro-runtime:12.0.0\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "config.yaml"), []byte("project:\n  name: demo\n"), 0o600))
	return dir
}

// v2 builds only pyproject.toml projects for APC: a deploy that would build
// with no project here is refused before anything is asked or sent, and a
// project in the 1.x layout makes no deploy at all, saying to convert it with
// astro init or deploy it with Astro CLI 1.x, as on Astro.
func TestDeployRefusesWhatV2DoesNotDeploy(t *testing.T) {
	apc1x := utils.Deploy1xRefused("this project", "")
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
		args  []string
		says  string
		// usage is whether the refusal is a usage error (exit 2).
		usage bool
		// noProject is whether it is a no_project failure.
		noProject bool
	}{
		{"a build outside any project", func(t *testing.T) string { return inWorkingDir(t, true) }, nil, "this is not an Astro project directory", false, true},
		{"a build of a 1.x project", in1xProject, nil, apc1x, false, true},
		{"--image-name from a 1.x project", in1xProject, []string{"--image-name", "img:1"}, apc1x, false, true},
		{"--image-name --remote from a 1.x project", in1xProject, []string{"--image-name", "img:1", "--remote", "--runtime-version", "12.1.1"}, apc1x, false, true},
		{"--dags from a 1.x project", in1xProject, []string{"--dags"}, apc1x, false, true},
		{"--dags outside any project", func(t *testing.T) string { return inWorkingDir(t, true) }, []string{"--dags"}, "this is not an Astro project directory", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := deployMocks(t, deployPushed, nil)
			tc.setup(t)
			for _, format := range []string{"text", "json"} {
				args := append(append([]string{"deploy", "dep-ac"}, tc.args...), "-o", format)
				run := runAPC(t, newAPCClient(), "", args...)
				require.Error(t, run.err, format)
				assert.Contains(t, run.err.Error(), tc.says, format)
				assert.Equal(t, tc.usage, cliout.IsUsage(run.err), format)
				var notFound *project.NotFoundError
				assert.Equal(t, tc.noProject, errors.As(run.err, &notFound), format)
				assert.Empty(t, seen.images, "%s: nothing is deployed", format)
				assert.Zero(t, seen.dagUploads, "%s: nothing is uploaded", format)
				if format == "json" {
					var obj struct {
						Error string `json:"error"`
					}
					require.NoError(t, json.Unmarshal([]byte(run.stdout), &obj), "stdout: %s", run.stdout)
					assert.Contains(t, obj.Error, tc.says)
				}
			}
		})
	}
}

// The deploys v2 keeps on APC need no 1.x project: --image-name, local or
// --remote, from anywhere else, and --dags from a pyproject.toml project.
func TestDeployKeepsImageNameAndDags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
		args  []string
	}{
		{"--image-name outside any project", func(t *testing.T) string { return inWorkingDir(t, false) }, []string{"--image-name", "img:1"}},
		{"--image-name from a pyproject.toml project", func(t *testing.T) string { return inProject(t, true) }, []string{"--image-name", "img:1"}},
		{"--image-name --remote outside any project", func(t *testing.T) string { return inWorkingDir(t, false) }, []string{"--image-name", "img:1", "--remote", "--runtime-version", "12.1.1"}},
		{"--image-name --remote from a pyproject.toml project", func(t *testing.T) string { return inProject(t, true) }, []string{"--image-name", "img:1", "--remote", "--runtime-version", "12.1.1"}},
		{"--dags from a pyproject.toml project", func(t *testing.T) string { return inProject(t, true) }, []string{"--dags"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deployMocks(t, deployPushed, nil)
			tc.setup(t)
			run := runAPC(t, newAPCClient(), "", append(append([]string{"deploy", "dep-ac"}, tc.args...), "-o", "json")...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			var got deployJSON
			decodeOne(t, run.stdout, &got)
			assert.Equal(t, "dep-ac", got.Deployment)
		})
	}
}
