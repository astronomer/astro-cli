package software

import (
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
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) (string, error) {
		return deploymentID, nil
	}

	DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool) error {
		return nil
	}

	err := execDeployCmd([]string{"-f"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"-f", "test-deployment-id"}...)
	s.NoError(err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	s.NoError(err)

	DagsOnlyDeploy = deploy.DagsOnlyDeploy

	s.Run("error should be returned for astro deploy, if DeployAirflowImage throws error", func() {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}

		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrNoWorkspaceID)

		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) (string, error) {
			return deploymentID, nil
		}
	})

	s.Run("error should be returned for astro deploy, if dags deploy throws error and the feature is enabled", func() {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool) error {
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

	s.Run("error should be returned if BYORegistryEnabled is true but BYORegistryDomain is empty", func() {
		appConfig = &houston.AppConfig{
			BYORegistryDomain: "",
			Flags: houston.FeatureFlags{
				BYORegistryEnabled: true,
			},
		}
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool) error {
			return deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"-f"}...)
		s.ErrorIs(err, deploy.ErrBYORegistryDomainNotSet)
		DagsOnlyDeploy = deploy.DagsOnlyDeploy
	})
}
