package software

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/software/deploy"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func execDeployCmd(args ...string) error {
	cmd := NewDeployCmd()
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return err
}

func TestDeploy(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	assert.NoError(t, err)

	DagsOnlyDeploy = deploy.DagsOnlyDeploy

	t.Run("error should be returned for astro deploy, if DeployAirflowImage throws error", func(t *testing.T) {
		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) (string, error) {
			return deploymentID, deploy.ErrNoWorkspaceID
		}

		err := execDeployCmd([]string{"-f"}...)
		assert.ErrorIs(t, err, deploy.ErrNoWorkspaceID)

		DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) (string, error) {
			return deploymentID, nil
		}
	})

	t.Run("error should be returned for astro deploy, if dags deploy throws error and the feature is enabled", func(t *testing.T) {
		DagsOnlyDeploy = func(houstonClient houston.ClientInterface, appConfig *houston.AppConfig, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool) error {
			return deploy.ErrNoWorkspaceID
		}
		err := execDeployCmd([]string{"-f"}...)
		assert.ErrorIs(t, err, deploy.ErrNoWorkspaceID)
		DagsOnlyDeploy = deploy.DagsOnlyDeploy
	})

	t.Run("No error should be returned for astro deploy, if dags deploy throws error but the feature itself is disabled", func(t *testing.T) {
		err := execDeployCmd([]string{"-f"}...)
		assert.ErrorIs(t, err, nil)
	})

	t.Run("Test for the flag --dags", func(t *testing.T) {
		err := execDeployCmd([]string{"test-deployment-id", "--dags", "--force"}...)
		assert.ErrorIs(t, err, deploy.ErrDagOnlyDeployDisabledInConfig)
	})

	t.Run("error should be returned if BYORegistryEnabled is true but BYORegistryDomain is empty", func(t *testing.T) {
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
		assert.ErrorIs(t, err, deploy.ErrBYORegistryDomainNotSet)
		DagsOnlyDeploy = deploy.DagsOnlyDeploy
	})
}
