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

func TestDeployWithClientFlag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	var capturedDeployInput cloud.InputDeploy

	DeployImage = func(deployInput cloud.InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		capturedDeployInput = deployInput
		return nil
	}

	t.Run("client flag sets ClientDeploy to true", func(t *testing.T) {
		err := execDeployCmd("--client", "--force", "test-deployment-id")
		assert.NoError(t, err)
		assert.True(t, capturedDeployInput.ClientDeploy, "ClientDeploy should be true when --client flag is used")
	})

	t.Run("client flag with custom image name", func(t *testing.T) {
		err := execDeployCmd("--client", "--force", "--image-name", "custom-image:tag", "test-deployment-id")
		assert.NoError(t, err)
		assert.True(t, capturedDeployInput.ClientDeploy, "ClientDeploy should be true when --client flag is used")
		assert.Equal(t, "custom-image:tag", capturedDeployInput.ImageName, "ImageName should be set correctly")
	})

	t.Run("without client flag sets ClientDeploy to false", func(t *testing.T) {
		err := execDeployCmd("--force", "test-deployment-id")
		assert.NoError(t, err)
		assert.False(t, capturedDeployInput.ClientDeploy, "ClientDeploy should be false when --client flag is not used")
	})

	t.Run("platform flag works with client flag", func(t *testing.T) {
		err := execDeployCmd("--client", "--platform", "linux/amd64,linux/arm64", "--force", "test-deployment-id")
		assert.NoError(t, err)
		assert.True(t, capturedDeployInput.ClientDeploy, "ClientDeploy should be true when --client flag is used")
		assert.Equal(t, "linux/amd64,linux/arm64", capturedDeployInput.Platform, "Platform should be set correctly")
	})

	t.Run("platform flag without client flag returns error", func(t *testing.T) {
		err := execDeployCmd("--platform", "linux/amd64", "--force", "test-deployment-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--platform flag can only be used with --client flag")
	})
}
