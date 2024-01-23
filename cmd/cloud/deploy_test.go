package cloud

import (
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func execDeployCmd(args ...string) error {
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewDeployCmd()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestDeployImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	DeployImage = func(deployInput cloud.InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		return nil
	}

	err := execDeployCmd([]string{"-f"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "-f", "--wait"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--pytest"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--parse"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--parse", "--pytest"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--parse", "--pytest"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--dags"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--dags", "--wait"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--dags", "--pytest"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--dags", "--parse"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--dags", "--parse", "--pytest"}...)
	assert.NoError(t, err)
}
