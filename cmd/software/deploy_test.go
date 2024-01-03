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
	DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) error {
		return nil
	}

	err := execDeployCmd([]string{"-f"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	assert.NoError(t, err)

	t.Run("Test for the flag --dag-only", func(t *testing.T) {
		err := execDeployCmd([]string{"test-deployment-id", "--dag-only", "--force"}...)
		assert.ErrorIs(t, err, deploy.ErrDagOnlyDeployDisabledInConfig)
	})
}
