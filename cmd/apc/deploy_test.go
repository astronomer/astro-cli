package apc

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/project"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execDeployCmd(args ...string) error {
	cmd := NewDeployCmd(new(bytes.Buffer))
	cmd.SetArgs(args)
	defer testUtil.SetupOSArgsForGinkgo()()
	_, err := cmd.ExecuteC()
	return err
}

// A checkout with uncommitted changes is refused with an error, so the deploy
// that did not happen exits non-zero, with no usage block under it. It used to
// print a note and return nil: exit 0. --force deploys anyway.
func (s *Suite) TestDeployRefusesUncommittedChanges() {
	appConfig = &houston.AppConfig{}
	prev := hasUncommittedChanges
	hasUncommittedChanges = func(string) bool { return true }
	defer func() { hasUncommittedChanges = prev }()

	deployed := 0
	DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
		deployed++
		return deploy.Deployed{DeploymentID: deploymentID}, nil
	}
	prevDags := DagsOnlyDeploy
	DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
		return deploymentID, nil
	}
	defer func() { DagsOnlyDeploy = prevDags }()

	cmd := NewDeployCmd(new(bytes.Buffer))
	var printed bytes.Buffer
	cmd.SetOut(&printed)
	cmd.SetErr(&printed)
	cmd.SetArgs([]string{"test-deployment-id", "--image-name", "img:1", "--workspace-id", "test-workspace-id"})
	restore := testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	restore()
	s.ErrorIs(err, errUncommittedChanges)
	s.Equal(0, deployed, "a refused deploy ships nothing")
	s.NotContains(printed.String(), "Usage:", "the refusal is not a usage mistake")

	s.NoError(execDeployCmd("test-deployment-id", "--image-name", "img:1", "--workspace-id", "test-workspace-id", "--force"))
	s.Equal(1, deployed, "--force deploys anyway")
}

func (s *Suite) TestDeploy() {
	// From a project, whose DAGs follow the image to a Deployment that takes them.
	inProject(s.T(), true)
	appConfig = &houston.AppConfig{
		BYORegistryDomain: "test.registry.io",
		Flags: houston.FeatureFlags{
			BYORegistryEnabled: true,
		},
	}
	DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
		return deploy.Deployed{DeploymentID: deploymentID}, nil
	}

	var gotDescription string
	DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
		gotDescription = description
		return deploymentID, nil
	}

	s.NoError(execDeployCmd("-f", "--image-name", "img:1"))
	s.NoError(execDeployCmd("-f", "test-deployment-id", "--image-name", "img:1"))

	// The description reaches the DAG upload that follows the image.
	s.NoError(execDeployCmd("test-deployment-id", "--image-name", "img:1", "--description", "Initial deployment", "--force"))
	s.Equal("Initial deployment", gotDescription)
	description = ""
	s.NoError(execDeployCmd("test-deployment-id", "--image-name", "img:1", "--force"))
	s.Equal("Deployed via <astro deploy>", gotDescription)

	// Restore DagsOnlyDeploy to default behavior
	DagsOnlyDeploy = deploy.DagsOnlyDeploy

	s.Run("error should be returned for astro deploy, if DeployAirflowImage throws error", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, deploy.ErrNoWorkspaceID
		}

		err := execDeployCmd("-f", "--image-name", "img:1")
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)

		DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
	})

	s.Run("error should be returned for astro deploy, if dags deploy throws error and the feature is enabled", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd("-f", "--image-name", "img:1")
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)
	})

	s.Run("Test for the flag --dags when the feature is disabled", func() {
		inProject(s.T(), true)
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrDagOnlyDeployDisabledInConfig
		}
		err := execDeployCmd("test-deployment-id", "--dags", "--force")
		s.ErrorIs(err, deploy.ErrDagOnlyDeployDisabledInConfig)
	})

	s.Run("Test when both the flags --dags and --image are passed", func() {
		inProject(s.T(), true)
		err := execDeployCmd("test-deployment-id", "--dags", "--image", "--force")
		s.ErrorIs(err, ErrBothDagsOnlyAndImageOnlySet)
	})

	s.Run("Test for the flag --image for image deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, deploy.ErrDeploymentTypeIncorrectForImageOnly
		}
		err := execDeployCmd("test-deployment-id", "--image", "--image-name", "img:1", "--force")
		s.ErrorIs(err, deploy.ErrDeploymentTypeIncorrectForImageOnly)
	})

	s.Run("Test for the flag --image for dags-only deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
		// This function is not called since --image is passed
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd("test-deployment-id", "--image", "--image-name", "img:1", "--force")
		s.ErrorIs(err, nil)
	})

	s.Run("Test for the flag --image-name", func() {
		var capturedImageName string
		DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			capturedImageName = imageName // Capture the imageName
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, nil
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd("test-deployment-id", "--image-name="+testImageName, "--force", "--workspace-id="+mockWorkspace.ID)

		s.ErrorIs(err, nil)
		s.Equal(testImageName, capturedImageName, "The imageName passed to DeployAirflowImage is incorrect")
	})

	s.Run("Test for the flag --image-name with --remote. Dags should be deployed but DeployAirflowImage shouldn't be called", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, nil
		}
		// Create a flag to track if DeployAirflowImage is called
		deployAirflowImageCalled := false

		// Mock function for DeployAirflowImage
		DeployAirflowImage = func(houstonClient houston.ClientInterface, deploymentID, wsID string, prompt, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			deployAirflowImageCalled = true // Set the flag if this function is called
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{}, nil
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd("test-deployment-id", "--image-name="+testImageName, "--force", "--remote", "--workspace-id="+mockWorkspace.ID)
		s.ErrorIs(err, nil)
		// Assert that DeployAirflowImage was NOT called
		s.False(deployAirflowImageCalled, "DeployAirflowImage should not be called when --remote is specified")
	})

	s.Run("Test for the flag --image-name with --remote. Dags should not be deployed if UpdateDeploymentImage throws an error", func() {
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{}, errNoWorkspaceFound
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd("test-deployment-id", "--image-name="+testImageName, "--force", "--remote", "--workspace-id="+mockWorkspace.ID)
		s.ErrorIs(err, errNoWorkspaceFound)
	})

	s.Run("Test for the flag --remote without --image-name. It should throw an error", func() {
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{}, errNoWorkspaceFound
		}
		err := execDeployCmd("test-deployment-id", "--force", "--remote", "--workspace-id="+mockWorkspace.ID)
		s.ErrorIs(err, ErrImageNameNotPassedForRemoteFlag)
		// An empty --image-name= names no image either.
		err = execDeployCmd("test-deployment-id", "--force", "--remote", "--image-name=", "--workspace-id="+mockWorkspace.ID)
		s.ErrorIs(err, ErrImageNameNotPassedForRemoteFlag)
	})

	s.Run("error should be returned if BYORegistryEnabled is true but BYORegistryDomain is empty", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrBYORegistryDomainNotSet
		}
		err := execDeployCmd("-f", "--image-name", "img:1")
		s.ErrorIs(err, deploy.ErrBYORegistryDomainNotSet)
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

// v2 builds no project for APC: a deploy that would build one is refused
// before anything is asked or sent, saying to use Astro CLI 1.x, and a
// project in the 1.x layout makes no deploy at all. --image-name and, from a
// pyproject.toml project, --dags still deploy.
func TestDeployRefusesWhatV2DoesNotDeploy(t *testing.T) {
	apc1x := utils.Deploy1xRefusedAPC("this project", "")
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
		{"a build outside any project", func(t *testing.T) string { return inWorkingDir(t, true) }, nil, errBuildDeployNoProject, false, true},
		{"a build of a pyproject.toml project", func(t *testing.T) string { return inProject(t, true) }, nil, errBuildDeployManifest, true, false},
		{"an empty --image-name= is a build", func(t *testing.T) string { return inProject(t, true) }, []string{"--image-name="}, errBuildDeployManifest, true, false},
		{"a build of a 1.x project", in1xProject, nil, apc1x, false, true},
		{"--image-name from a 1.x project", in1xProject, []string{"--image-name", "img:1"}, apc1x, false, true},
		{"--dags from a 1.x project", in1xProject, []string{"--dags"}, apc1x, false, true},
		{"--dags outside any project", func(t *testing.T) string { return inWorkingDir(t, true) }, []string{"--dags"}, "this is not an Astro project directory", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := deployMocks(t, deployPushed, nil)
			imageDeploys := 0
			DeployAirflowImage = func(houston.ClientInterface, string, string, bool, bool, string, deploy.Options) (deploy.Deployed, error) {
				imageDeploys++
				return deployPushed, nil
			}
			tc.setup(t)
			for _, format := range []string{"text", "json"} {
				args := append(append([]string{"deploy", "dep-ac"}, tc.args...), "-o", format)
				run := runAPC(t, newAPCClient(), "", args...)
				require.Error(t, run.err, format)
				assert.Contains(t, run.err.Error(), tc.says, format)
				assert.Equal(t, tc.usage, cliout.IsUsage(run.err), format)
				var notFound *project.NotFoundError
				assert.Equal(t, tc.noProject, errors.As(run.err, &notFound), format)
				assert.Zero(t, imageDeploys, "%s: nothing is deployed", format)
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

// The deploys v2 keeps on APC need no 1.x project: --image-name from
// anywhere else, and --dags from a pyproject.toml project.
func TestDeployKeepsImageNameAndDags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
		args  []string
	}{
		{"--image-name outside any project", func(t *testing.T) string { return inWorkingDir(t, false) }, []string{"--image-name", "img:1"}},
		{"--image-name from a pyproject.toml project", func(t *testing.T) string { return inProject(t, true) }, []string{"--image-name", "img:1"}},
		{"--image-name --remote outside any project", func(t *testing.T) string { return inWorkingDir(t, false) }, []string{"--image-name", "img:1", "--remote", "--runtime-version", "12.1.1"}},
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
