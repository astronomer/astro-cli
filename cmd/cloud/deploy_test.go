package cloud

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/astro-client-v1"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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

	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient) error {
		return nil
	}

	err := execDeployCmd("-f")
	assert.NoError(t, err)

	err = execDeployCmd("test-deployment-id", "-f", "--wait")
	assert.NoError(t, err)

	err = execDeployCmd("test-deployment-id", "--save")
	assert.NoError(t, err)

	err = execDeployCmd("-f", "test-deployment-id", "--pytest")
	assert.NoError(t, err)

	err = execDeployCmd("-f", "test-deployment-id", "--parse")
	assert.NoError(t, err)

	err = execDeployCmd("-f", "test-deployment-id", "--parse", "--pytest")
	assert.NoError(t, err)

	err = execDeployCmd("test-deployment-id", "--parse", "--pytest")
	assert.NoError(t, err)

	err = execDeployCmd("test-deployment-id", "--dags")
	assert.NoError(t, err)

	err = execDeployCmd("test-deployment-id", "--dags", "--wait")
	assert.NoError(t, err)

	err = execDeployCmd("-f", "test-deployment-id", "--dags", "--pytest")
	assert.NoError(t, err)

	err = execDeployCmd("-f", "test-deployment-id", "--dags", "--parse")
	assert.NoError(t, err)

	err = execDeployCmd("-f", "test-deployment-id", "--dags", "--parse", "--pytest")
	assert.NoError(t, err)
}
