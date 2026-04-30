package cloud

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
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

func TestDeployProjectDirCheckWithImageName(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return assert.AnError
	}
	defer func() {
		EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	}()

	DeployImage = func(deployInput cloud.InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		return nil
	}

	// --image --image-name is an explicit image-only push of a prebuilt image;
	// no local project files are needed, so the check is skipped.
	err := execDeployCmd("-f", "test-deployment-id", "--image", "--image-name", "custom-image:latest")
	assert.NoError(t, err)

	// --image-name alone keeps the default image-and-dags behavior, which
	// reads DAGs from the working directory and so still requires a project.
	err = execDeployCmd("-f", "test-deployment-id", "--image-name", "custom-image:latest")
	assert.ErrorIs(t, err, assert.AnError)

	// No --image-name: check still runs and propagates.
	err = execDeployCmd("-f", "test-deployment-id")
	assert.ErrorIs(t, err, assert.AnError)
}

func TestDeployImageNameDoesNotForceImageOnly(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }

	var captured cloud.InputDeploy
	DeployImage = func(deployInput cloud.InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		captured = deployInput
		return nil
	}

	err := execDeployCmd("-f", "test-deployment-id", "--image-name", "custom-image:latest")
	assert.NoError(t, err)
	assert.False(t, captured.Image, "--image-name alone must not force image-only; default deploy type is image-and-dags")
	assert.Equal(t, "custom-image:latest", captured.ImageName)
}
