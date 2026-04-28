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

func TestDeploySkipsEnsureProjectDirWhenImageNameSet(t *testing.T) {
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

	// With --image-name, the project-dir check should be skipped.
	err := execDeployCmd("-f", "test-deployment-id", "--image-name", "custom-image:latest")
	assert.NoError(t, err)

	// Without --image-name, the project-dir check should still run and propagate.
	err = execDeployCmd("-f", "test-deployment-id")
	assert.ErrorIs(t, err, assert.AnError)
}

func TestDeployImageNameForcesImageOnly(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }

	var captured cloud.InputDeploy
	DeployImage = func(deployInput cloud.InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		captured = deployInput
		return nil
	}

	err := execDeployCmd("-f", "test-deployment-id", "--image-name", "custom-image:latest")
	assert.NoError(t, err)
	assert.True(t, captured.Image, "--image-name should force image-only deploy")
	assert.Equal(t, "custom-image:latest", captured.ImageName)
}

func TestDeployImageNameRejectsIncompatibleFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	DeployImage = func(deployInput cloud.InputDeploy, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		return nil
	}

	cases := []struct {
		name string
		args []string
	}{
		{"dags", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--dags"}},
		{"dags-path", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--dags-path", "./dags"}},
		{"no-dags-base-dir", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--no-dags-base-dir"}},
		{"pytest", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--pytest"}},
		{"parse", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--parse"}},
		{"build-secrets", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--build-secrets", "id=mysecret,src=secrets.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execDeployCmd(tc.args...)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "--image-name")
		})
	}
}
