package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/cosmosboost"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// preDeployMarker is the file the fake helper below drops into a directory it
// was pointed at. The name is the test's own: what the real helper writes, and
// where, is its business, and these tests assert only that the CLI invoked it
// against the right directory at the right moment.
const preDeployMarker = "cosmosboost-pre-deploy-ran"

// installFakeCosmosBoostHelper puts a shell script at cosmosboost.BinaryPath()
// standing in for the real helper. It answers `version` (so EnsureBinary skips
// the CDN download), drops the marker for `pre-deploy`, removes it again for
// `uninstall`, and records every invocation in the returned log path.
func installFakeCosmosBoostHelper(t *testing.T) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(cosmosboost.BinDir(), 0o755))
	log := filepath.Join(t.TempDir(), "invocations.log")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = version ]; then echo 9.9.9; exit 0; fi
echo "$@" >> %[1]s
case "$1" in
  pre-deploy) : > "$2/%[2]s" ;;
  uninstall)  rm -f "$2/%[2]s" ;;
esac
`, log, preDeployMarker)
	require.NoError(t, os.WriteFile(cosmosboost.BinaryPath(), []byte(script), 0o755))
	return log
}

// setupCosmosBoostEnv isolates ~/.astro for the test and returns a restore func.
func setupCosmosBoostEnv(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake helper does not run on windows")
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	origHome := config.HomeConfigPath
	config.HomeConfigPath = t.TempDir()
	t.Cleanup(func() { config.HomeConfigPath = origHome })
}

func readInvocations(t *testing.T, log string) string {
	t.Helper()
	content, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

func TestUploadBundleRunsPreDeployWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	installFakeCosmosBoostHelper(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(bundleDir, preDeployMarker),
		"the pre-deploy step must run against the bundle before it is tarred")
}

func TestUploadBundleSkipsPreDeployByDefault(t *testing.T) {
	setupCosmosBoostEnv(t)
	log := installFakeCosmosBoostHelper(t)
	// cosmos_boost.pre_deploy defaults to false, so nothing may be produced.

	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(bundleDir, preDeployMarker),
		"opt-in gate is off by default; nothing may be written")
	assert.NotContains(t, readInvocations(t, log), "pre-deploy",
		"the pre-deploy step must not run while the gate is off")
}

// TestDeployBundleDbtPathRunsPreDeployWhenEnabled drives the full
// `astro dbt deploy` bundle path (DeployBundle into UploadBundle) to pin that
// moving the hook out of cmd/cloud/dbt.go did not lose dbt-deploy coverage.
func TestDeployBundleDbtPathRunsPreDeployWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	installFakeCosmosBoostHelper(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	canCiCdDeploy = func(token string) bool { return true }

	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockGetDeployment(mockV1Client, true, false)
	mockCreateDeploy(mockV1Client, "http://bundle-upload-url", nil)
	mockUpdateDeploy(mockV1Client, "version-id")
	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	err := DeployBundle(&DeployBundleInput{
		BundlePath:    bundleDir,
		MountPath:     "dbt/shop",
		DeploymentID:  "test-deployment-id",
		BundleType:    "dbt",
		AstroV1Client: mockV1Client,
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(bundleDir, preDeployMarker),
		"astro dbt deploy must still run the pre-deploy step after the hook moved into UploadBundle")
}

func TestBuildImageRunsPreDeployOnBuildContextWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	installFakeCosmosBoostHelper(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))

	// Fail the build immediately: the assertion below then proves the step ran
	// against the build context BEFORE docker build.
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(errMock).Once()
		return mockImageHandler
	}
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	_, err := buildImage(projectDir, "4.2.5", "", "", "", nil, false, false, mockV1Client)
	assert.ErrorIs(t, err, errMock)

	assert.FileExists(t, filepath.Join(projectDir, preDeployMarker),
		"the pre-deploy step must run against the build context before docker build")
	mockImageHandler.AssertExpectations(t)
}

func TestUploadBundleCleansUpWhenDisabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	log := installFakeCosmosBoostHelper(t)

	// Gate off (default), with an earlier enabled deploy's output still in the
	// tree. The CLI must ask the helper to clean the bundle rather than tar it.
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, preDeployMarker), []byte("stale"), 0o644))

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.Contains(t, readInvocations(t, log), "uninstall "+bundleDir,
		"cleanup must be delegated to the helper, against the bundle path")
	assert.NoFileExists(t, filepath.Join(bundleDir, preDeployMarker),
		"disabling the feature must actively clean up the bundle")
}
