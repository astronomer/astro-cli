package software

import (
	"fmt"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/software/deploy"
	"github.com/spf13/cobra"
)

func execDeployCmd(args ...string) error {
	cmd := NewDeployCmd()
	cmd.SetArgs(args)
	defer testUtil.SetupOSArgsForGinkgo()()
	_, err := cmd.ExecuteC()
	return err
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
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
		if description == "" {
			return deploymentID, fmt.Errorf("description should not be empty")
		}
		return deploymentID, nil
	}

	DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
		return nil
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
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
		expectedDesc := "Deployed via <astro deploy>"
		if description != expectedDesc {
			return deploymentID, fmt.Errorf("expected description to be '%s', but got '%s'", expectedDesc, description)
		}
		return deploymentID, nil
	}

	err = execDeployCmd([]string{"test-deployment-id", "--force"}...)
	s.NoError(err)

	s.Run("error should be returned for astro deploy, if DeployAirflowImage throws error", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}

		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)

		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			return deploymentID, nil
		}
	})

	s.Run("error should be returned for astro deploy, if dags deploy throws error and the feature is enabled", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
			return deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)
	})

	s.Run("No error should be returned for astro deploy, if dags deploy throws error but the feature itself is disabled", func() {
		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, nil)
	})

	s.Run("Test for the flag --dags", func() {
		err := execDeployCmd([]string{"test-deployment-id", "--dags", "--force"}...)
		s.ErrorIs(err, deploy.ErrDagOnlyDeployDisabledInConfig)
	})

	s.Run("Test when both the flags --dags and --image are passed", func() {
		err := execDeployCmd([]string{"test-deployment-id", "--dags", "--image", "--force"}...)
		s.ErrorIs(err, ErrBothDagsOnlyAndImageOnlySet)
	})

	s.Run("Test for the flag --image for image deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			return deploymentID, deploy.ErrDeploymentTypeIncorrectForImageOnly
		}
		err := execDeployCmd([]string{"test-deployment-id", "--image", "--force"}...)
		s.ErrorIs(err, deploy.ErrDeploymentTypeIncorrectForImageOnly)
	})

	s.Run("Test for the flag --image for dags-only deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			return deploymentID, nil
		}
		// This function is not called since --image is passed
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
			return deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"test-deployment-id", "--image", "--force"}...)
		s.ErrorIs(err, nil)
	})

	s.Run("Test for the flag --image-name", func() {
		var capturedImageName string
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			capturedImageName = imageName // Capture the imageName
			return deploymentID, nil
		}
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
			return nil
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd([]string{"test-deployment-id", "--image-name=" + testImageName, "--force", "--workspace-id=" + mockWorkspace.ID}...)

		s.ErrorIs(err, nil)
		s.Equal(testImageName, capturedImageName, "The imageName passed to DeployAirflowImage is incorrect")
	})

	s.Run("Test for the flag --image-name with --remote. Dags should be deployed but DeployAirflowImage shouldn't be called", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
			return nil
		}
		// Create a flag to track if DeployAirflowImage is called
		deployAirflowImageCalled := false

		// Mock function for DeployAirflowImage
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			deployAirflowImageCalled = true // Set the flag if this function is called
			return deploymentID, nil
		}
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string) (string, error) {
			return "", nil
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd([]string{"test-deployment-id", "--image-name=" + testImageName, "--force", "--remote", "--workspace-id=" + mockWorkspace.ID}...)
		s.ErrorIs(err, nil)
		// Assert that DeployAirflowImage was NOT called
		s.False(deployAirflowImageCalled, "DeployAirflowImage should not be called when --remote is specified")
	})

	s.Run("Test for the flag --image-name with --remote. Dags should not be deployed if UpdateDeploymentImage throws an error", func() {
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string) (string, error) {
			return "", errNoWorkspaceFound
		}
		testImageName := "test-image-name" // Set the expected image name
		err := execDeployCmd([]string{"test-deployment-id", "--image-name=" + testImageName, "--force", "--remote", "--workspace-id=" + mockWorkspace.ID}...)
		s.ErrorIs(err, errNoWorkspaceFound)
	})

	s.Run("Test for the flag --remote without --image-name. It should throw an error", func() {
		UpdateDeploymentImage = func(houstonClient houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string) (string, error) {
			return "", errNoWorkspaceFound
		}
		err := execDeployCmd([]string{"test-deployment-id", "--force", "--remote", "--workspace-id=" + mockWorkspace.ID}...)
		s.ErrorIs(err, ErrImageNameNotPassedForRemoteFlag)
	})

	s.Run("error should be returned if BYORegistryEnabled is true but BYORegistryDomain is empty", func() {
		appConfig = &houston.AppConfig{
			BYORegistryDomain: "",
			Flags: houston.FeatureFlags{
				BYORegistryEnabled: true,
			},
		}
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string) error {
			return deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrBYORegistryDomainNotSet)
	})
}
