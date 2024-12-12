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

	// Restore DagsOnlyDeploy to default behavior
	DagsOnlyDeploy = deploy.DagsOnlyDeploy

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
		DagsOnlyDeploy = deploy.DagsOnlyDeploy
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

	s.Run("Test for the flag --image-name for image deployment", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool, description string, isImageOnlyDeploy bool, imageName string) (string, error) {
			return deploymentID, nil
		}
		err := execDeployCmd([]string{"test-deployment-id", "--image-name", "--force"}...)
		s.ErrorIs(err, nil)
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
		DagsOnlyDeploy = deploy.DagsOnlyDeploy
	})
}
