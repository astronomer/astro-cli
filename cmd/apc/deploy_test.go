package apc

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
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
	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	prev := hasUncommittedChanges
	hasUncommittedChanges = func(string) bool { return true }
	defer func() { hasUncommittedChanges = prev }()

	deployed := 0
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
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
	cmd.SetArgs([]string{"test-deployment-id", "--workspace-id", "test-workspace-id"})
	restore := testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	restore()
	s.ErrorIs(err, errUncommittedChanges)
	s.Equal(0, deployed, "a refused deploy ships nothing")
	s.NotContains(printed.String(), "Usage:", "the refusal is not a usage mistake")

	s.NoError(execDeployCmd("test-deployment-id", "--workspace-id", "test-workspace-id", "--force"))
	s.Equal(1, deployed, "--force deploys anyway")
}

func (s *Suite) TestDeploy() {
	appConfig = &houston.AppConfig{
		BYORegistryDomain: "test.registry.io",
		Flags: houston.FeatureFlags{
			BYORegistryEnabled: true,
		},
	}
	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
		if description == "" {
			return deploy.Deployed{DeploymentID: deploymentID}, fmt.Errorf("description should not be empty")
		}
		return deploy.Deployed{DeploymentID: deploymentID}, nil
	}

	DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
		return deploymentID, nil
	}

	err := execDeployCmd([]string{"-f"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	s.NoError(err)

	// Test when description is provided using the flag --description
	err = execDeployCmd([]string{"test-deployment-id", "--description", "Initial deployment", "--force"}...)
	s.NoError(err)

	// Test when the default description is used
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
		expectedDesc := "Deployed via <astro deploy>"
		if description != expectedDesc {
			return deploy.Deployed{DeploymentID: deploymentID}, fmt.Errorf("expected description to be '%s', but got '%s'", expectedDesc, description)
		}
		return deploy.Deployed{DeploymentID: deploymentID}, nil
	}

	err = execDeployCmd([]string{"test-deployment-id", "--force"}...)
	s.NoError(err)

	// Restore DagsOnlyDeploy to default behavior
	DagsOnlyDeploy = deploy.DagsOnlyDeploy

	s.Run("error should be returned for astro deploy, if DeployAirflowImage throws error", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, deploy.ErrNoWorkspaceID
		}

		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)

		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
	})

	s.Run("error should be returned for astro deploy, if dags deploy throws error and the feature is enabled", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)
	})

	s.Run("Test for the flag --dags when the feature is disabled", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrDagOnlyDeployDisabledInConfig
		}
		err := execDeployCmd([]string{"test-deployment-id", "--dags", "--force"}...)
		s.ErrorIs(err, deploy.ErrDagOnlyDeployDisabledInConfig)
	})

	s.Run("Test when both the flags --dags and --image are passed", func() {
		err := execDeployCmd([]string{"test-deployment-id", "--dags", "--image", "--force"}...)
		s.ErrorIs(err, ErrBothDagsOnlyAndImageOnlySet)
	})

	s.Run("Test for the flag --image for image deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, deploy.ErrDeploymentTypeIncorrectForImageOnly
		}
		err := execDeployCmd([]string{"test-deployment-id", "--image", "--force"}...)
		s.ErrorIs(err, deploy.ErrDeploymentTypeIncorrectForImageOnly)
	})

	s.Run("Test for the flag --image for dags-only deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
		// This function is not called since --image is passed
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"test-deployment-id", "--image", "--force"}...)
		s.ErrorIs(err, nil)
	})

	s.Run("Test for the flag --image-name", func() {
		var capturedImageName string
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			capturedImageName = imageName // Capture the imageName
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, nil
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd([]string{"test-deployment-id", "--image-name=" + testImageName, "--force", "--workspace-id=" + mockWorkspace.ID}...)

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
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID string, ignoreCacheDeploy, prompt bool, description string, isImageOnlyDeploy bool, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			deployAirflowImageCalled = true // Set the flag if this function is called
			return deploy.Deployed{DeploymentID: deploymentID}, nil
		}
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{}, nil
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd([]string{"test-deployment-id", "--image-name=" + testImageName, "--force", "--remote", "--workspace-id=" + mockWorkspace.ID}...)
		s.ErrorIs(err, nil)
		// Assert that DeployAirflowImage was NOT called
		s.False(deployAirflowImageCalled, "DeployAirflowImage should not be called when --remote is specified")
	})

	s.Run("Test for the flag --image-name with --remote. Dags should not be deployed if UpdateDeploymentImage throws an error", func() {
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{}, errNoWorkspaceFound
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd([]string{"test-deployment-id", "--image-name=" + testImageName, "--force", "--remote", "--workspace-id=" + mockWorkspace.ID}...)
		s.ErrorIs(err, errNoWorkspaceFound)
	})

	s.Run("Test for the flag --remote without --image-name. It should throw an error", func() {
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, _ deploy.Options) (deploy.Deployed, error) {
			return deploy.Deployed{}, errNoWorkspaceFound
		}
		err := execDeployCmd([]string{"test-deployment-id", "--force", "--remote", "--workspace-id=" + mockWorkspace.ID}...)
		s.ErrorIs(err, ErrImageNameNotPassedForRemoteFlag)
	})

	s.Run("error should be returned if BYORegistryEnabled is true but BYORegistryDomain is empty", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, _ deploy.Options) (string, error) {
			return deploymentID, deploy.ErrBYORegistryDomainNotSet
		}
		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrBYORegistryDomainNotSet)
	})
}
