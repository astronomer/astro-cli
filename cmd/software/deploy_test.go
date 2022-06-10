package software

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func execDeployCmd(args ...string) error {
	cmd := newDeployCmd()
	cmd.SetArgs(args)
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
	ensureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}
	deployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) error {
		return nil
	}

	err := execDeployCmd([]string{"-f"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	assert.NoError(t, err)
}
