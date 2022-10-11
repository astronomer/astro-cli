package cloud

import (
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
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

func TestDeployImage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ensureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	deployImage = func(deployInput cloud.InputDeploy, client astro.Client) error {
		return nil
	}

	err := execDeployCmd([]string{"-f"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"test-deployment-id", "--save"}...)
	assert.NoError(t, err)

	err = execDeployCmd([]string{"-f", "test-deployment-id", "--pytest"}...)
	assert.NoError(t, err)
}
