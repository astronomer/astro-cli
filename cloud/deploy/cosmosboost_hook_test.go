package deploy

import (
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

// installFakeCosmosBoostHelper puts a shell script at cosmosboost.BinaryPath()
// standing in for the real helper: it answers `version` (so EnsureBinary skips
// the CDN download) and writes a sidecar for `pre-deploy <path>`.
func installFakeCosmosBoostHelper(t *testing.T) {
	t.Helper()
	require.NoError(t, os.MkdirAll(cosmosboost.BinDir(), 0o755))
	script := `#!/bin/sh
if [ "$1" = version ]; then echo 9.9.9; exit 0; fi
mkdir -p "$2/.astro"
echo '{"schema":1}' > "$2/.astro/dbt_metadata.json"
`
	require.NoError(t, os.WriteFile(cosmosboost.BinaryPath(), []byte(script), 0o755))
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

func TestUploadBundleStampsSidecarWhenEnabled(t *testing.T) {
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

	assert.FileExists(t, filepath.Join(bundleDir, ".astro", "dbt_metadata.json"),
		"the sidecar must be written into the bundle before it is tarred")
}

func TestUploadBundleDoesNotStampByDefault(t *testing.T) {
	setupCosmosBoostEnv(t)
	installFakeCosmosBoostHelper(t)
	// cosmos_boost.pre_deploy defaults to false — the hook must be a no-op.

	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(bundleDir, ".astro", "dbt_metadata.json"),
		"opt-in gate is off by default; nothing may be written")
}

// TestDeployBundleDbtPathStampsSidecarWhenEnabled drives the full
// `astro dbt deploy` bundle path (DeployBundle → UploadBundle) to pin that
// moving the hook out of cmd/cloud/dbt.go did not lose dbt-deploy coverage.
func TestDeployBundleDbtPathStampsSidecarWhenEnabled(t *testing.T) {
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

	assert.FileExists(t, filepath.Join(bundleDir, ".astro", "dbt_metadata.json"),
		"astro dbt deploy must still stamp the dbt project after the hook moved into UploadBundle")
}

func TestBuildImageStampsBuildContextWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	installFakeCosmosBoostHelper(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))

	// Fail the build right after the stamp: the assertion below proves the
	// sidecar lands in the build context BEFORE docker build runs.
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(errMock).Once()
		return mockImageHandler
	}
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	_, err := buildImage(projectDir, "4.2.5", "", "", "", "", false, false, mockV1Client)
	assert.ErrorIs(t, err, errMock)

	assert.FileExists(t, filepath.Join(projectDir, ".astro", "dbt_metadata.json"),
		"the sidecar must be written into the build context before docker build")
	mockImageHandler.AssertExpectations(t)
}
