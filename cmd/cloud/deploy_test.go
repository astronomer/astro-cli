package cloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/astro-client-v1"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/config"
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

func TestDeployInclude(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }

	// Isolated project dir containing an include/ directory.
	projectDir, err := os.MkdirTemp("", "include-deploy")
	assert.NoError(t, err)
	defer os.RemoveAll(projectDir)
	assert.NoError(t, os.Mkdir(filepath.Join(projectDir, includeDirName), 0o755))

	origWorkingPath := config.WorkingPath
	config.WorkingPath = projectDir
	defer func() { config.WorkingPath = origWorkingPath }()

	origDeployBundle := DeployBundle
	defer func() { DeployBundle = origDeployBundle }()

	var captured *cloud.DeployBundleInput
	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		captured = deployInput
		return nil
	}

	err = execDeployCmd("test-deployment-id", "--include", "--workspace-id", "test-ws")
	assert.NoError(t, err)
	if assert.NotNil(t, captured) {
		assert.Equal(t, includeBundleType, captured.BundleType)
		assert.Equal(t, includeMountPath, captured.MountPath)
		assert.Equal(t, filepath.Join(projectDir, includeDirName), captured.BundlePath)
		assert.Equal(t, "test-deployment-id", captured.DeploymentID)
	}
}

func TestDeployIncludeMissingDir(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }

	// Project dir without an include/ directory.
	projectDir, err := os.MkdirTemp("", "include-deploy-missing")
	assert.NoError(t, err)
	defer os.RemoveAll(projectDir)

	origWorkingPath := config.WorkingPath
	config.WorkingPath = projectDir
	defer func() { config.WorkingPath = origWorkingPath }()

	err = execDeployCmd("test-deployment-id", "--include", "--workspace-id", "test-ws")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "include directory not found")
}

func TestDeployIncludeRejectsIncompatibleFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient) error {
		return nil
	}

	cases := []struct {
		name string
		args []string
	}{
		{"dags", []string{"-f", "test-deployment-id", "--include", "--dags"}},
		{"image", []string{"-f", "test-deployment-id", "--include", "--image"}},
		{"image-name", []string{"-f", "test-deployment-id", "--include", "--image-name", "img:1"}},
		{"dags-path", []string{"-f", "test-deployment-id", "--include", "--dags-path", "./dags"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execDeployCmd(tc.args...)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "--include")
		})
	}
}

func TestDeploySkipsEnsureProjectDirWhenImageNameSet(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return assert.AnError
	}
	defer func() {
		EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	}()

	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient) error {
		return nil
	}

	// With --image-name, the project-dir check should be skipped.
	err := execDeployCmd("-f", "test-deployment-id", "--image-name", "custom-image:latest")
	assert.NoError(t, err)

	// Without --image-name, the project-dir check should still run and propagate.
	err = execDeployCmd("-f", "test-deployment-id")
	assert.ErrorIs(t, err, assert.AnError)
}

func TestDeployImageNameRejectsIncompatibleFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient) error {
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
