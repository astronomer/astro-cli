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
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// cosmosBoostArtifact is the sidecar the pre-deploy step writes next to a dbt
// project; these tests assert it is produced (or removed) at the right moment
// in each deploy path.
const cosmosBoostArtifact = ".astro/dbt_metadata.json"

// setupCosmosBoostEnv isolates ~/.astro for the test.
func setupCosmosBoostEnv(t *testing.T) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	origHome := config.HomeConfigPath
	config.HomeConfigPath = t.TempDir()
	t.Cleanup(func() { config.HomeConfigPath = origHome })
}

func writeDbtBundle(t *testing.T) string {
	t.Helper()
	bundleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "dbt_project.yml"), []byte("name: shop\n"), 0o644))
	return bundleDir
}

func TestUploadBundleRunsPreDeployWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	bundleDir := writeDbtBundle(t)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(bundleDir, cosmosBoostArtifact),
		"the pre-deploy step must stamp the bundle before it is tarred")
}

func TestUploadBundleSkipsPreDeployByDefault(t *testing.T) {
	setupCosmosBoostEnv(t)
	// cosmos_boost.pre_deploy defaults to false, so nothing may be produced.

	bundleDir := writeDbtBundle(t)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(bundleDir, cosmosBoostArtifact),
		"opt-in gate is off by default; nothing may be written")
}

// TestDeployBundleDbtPathRunsPreDeployWhenEnabled drives the full
// `astro dbt deploy` bundle path (DeployBundle into UploadBundle) to pin that
// moving the hook out of cmd/cloud/dbt.go did not lose dbt-deploy coverage.
func TestDeployBundleDbtPathRunsPreDeployWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	canCiCdDeploy = func(token string) bool { return true }

	bundleDir := writeDbtBundle(t)

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

	assert.FileExists(t, filepath.Join(bundleDir, cosmosBoostArtifact),
		"astro dbt deploy must still run the pre-deploy step after the hook moved into UploadBundle")
}

func TestBuildImageRunsPreDeployOnBuildContextWhenEnabled(t *testing.T) {
	setupCosmosBoostEnv(t)
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

	assert.FileExists(t, filepath.Join(projectDir, cosmosBoostArtifact),
		"the pre-deploy step must run against the build context before docker build")
	mockImageHandler.AssertExpectations(t)
}

// TestUploadBundleFailsWhenStaleArtifactUnremovable pins the safety property
// of ENABLED deploys: a deploy that cannot guarantee the payload is free of
// stale artifacts must fail rather than ship one, because consumers cannot
// tell fresh from stale.
func TestUploadBundleFailsWhenStaleArtifactUnremovable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission semantics differ on windows")
	}
	setupCosmosBoostEnv(t)
	require.NoError(t, config.CFG.CosmosBoostPreDeploy.SetHomeString("true"))

	bundleDir := writeDbtBundle(t)
	stale := filepath.Join(bundleDir, cosmosBoostArtifact)
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte(`{"generated_by": {"application": "astro"}}`), 0o644))
	require.NoError(t, os.Chmod(filepath.Dir(stale), 0o555)) // file cannot be unlinked
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(stale), 0o755) })

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")

	require.Error(t, err, "the deploy must not proceed with a stale artifact it could not remove")
	assert.ErrorContains(t, err, "Cosmos Boost")
}

// TestUploadBundleIsANoOpWhenDisabled pins the opt-out contract (review ask on
// astronomer/astro-cli#2236): with the gate off the deploy neither walks nor
// mutates the tree - even an artifact left by an earlier enabled deploy stays
// in place, and removing it is `astro dbt cleanup`'s job.
func TestUploadBundleIsANoOpWhenDisabled(t *testing.T) {
	setupCosmosBoostEnv(t)

	bundleDir := writeDbtBundle(t)
	leftover := filepath.Join(bundleDir, cosmosBoostArtifact)
	require.NoError(t, os.MkdirAll(filepath.Dir(leftover), 0o755))
	require.NoError(t, os.WriteFile(leftover, []byte(`{"generated_by": {"application": "astro"}}`), 0o644))

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	_, err := UploadBundle(t.TempDir(), bundleDir, "http://upload-url", false, "0.0.0")
	require.NoError(t, err)

	assert.FileExists(t, leftover,
		"a disabled deploy must not touch the tree; leftovers are cleaned by astro dbt cleanup")
}
