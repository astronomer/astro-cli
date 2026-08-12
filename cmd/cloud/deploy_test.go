package cloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
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

	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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

func TestDeployReturnsErrorWhenSaveConfigFails(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
		return nil
	}

	// config.InitConfig reads and writes real files on disk (via afero.NewOsFs()),
	// including the home config that stores the current context. Point ASTRO_HOME at
	// a throwaway directory with its own valid context, so the test does not depend on
	// (or clobber) whatever real ~/.astro/config.yaml happens to exist on the machine
	// running the test - on a fresh CI runner there is no such file, so without this
	// the deploy fails earlier with "no context set" instead of exercising the save path.
	//
	// ASTRO_HOME is restored (and config's cached home-config path resynced) before
	// testUtil.InitTestConfig runs in the deferred cleanup below, otherwise the fake
	// config it writes for later tests lands at the wrong path and leaves them with no
	// context set either.
	origAstroHome, hadAstroHome := os.LookupEnv("ASTRO_HOME")
	homeDir := t.TempDir()
	require.NoError(t, os.Setenv("ASTRO_HOME", homeDir))
	homeAstroDir := filepath.Join(homeDir, config.ConfigDir)
	require.NoError(t, os.MkdirAll(homeAstroDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeAstroDir, config.ConfigFileNameWithExt), testUtil.NewTestConfig(testUtil.LocalPlatform), 0o600))

	// Point the project config at a real directory, but make the config file itself
	// a directory instead of a file, so the write that saves the deployment ID fails.
	// This confirms that a save failure stops the deploy instead of returning nil.
	tmpDir := t.TempDir()
	astroDir := filepath.Join(tmpDir, config.ConfigDir)
	require.NoError(t, os.MkdirAll(filepath.Join(astroDir, config.ConfigFileNameWithExt), 0o755))

	origWorkingPath := config.WorkingPath
	config.WorkingPath = tmpDir
	config.InitConfig(afero.NewOsFs())
	defer func() {
		if hadAstroHome {
			os.Setenv("ASTRO_HOME", origAstroHome)
		} else {
			os.Unsetenv("ASTRO_HOME")
		}
		config.WorkingPath = origWorkingPath
		config.InitConfig(afero.NewOsFs())
		testUtil.InitTestConfig(testUtil.LocalPlatform)
	}()

	err := execDeployCmd("test-deployment-id", "--save")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save deployment id in config")
}

func TestDeploySkipsEnsureProjectDirWhenImageNameSet(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return assert.AnError
	}
	defer func() {
		EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	}()

	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
		return nil
	}

	// With --image-name, the project-dir check should be skipped.
	err := execDeployCmd("-f", "test-deployment-id", "--image-name", "custom-image:latest")
	assert.NoError(t, err)

	// Without --image-name, the project-dir check should still run and propagate.
	err = execDeployCmd("-f", "test-deployment-id")
	assert.ErrorIs(t, err, assert.AnError)
}

func TestDeploySkipsEnsureProjectDirWhenDagsPathSet(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return assert.AnError
	}
	defer func() {
		EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	}()

	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
		return nil
	}

	// With --dags and --dags-path, the DAGs are read entirely from --dags-path, so the
	// project-dir check should be skipped.
	err := execDeployCmd("-f", "test-deployment-id", "--dags", "--dags-path", "./external-dags")
	assert.NoError(t, err)

	// --dags alone (DAGs read from the working directory) still needs the project-dir check.
	err = execDeployCmd("-f", "test-deployment-id", "--dags")
	assert.ErrorIs(t, err, assert.AnError)

	// --dags-path alone (without --dags, i.e. a full image+dags deploy) still needs the project
	// root as the Docker build context.
	err = execDeployCmd("-f", "test-deployment-id", "--dags-path", "./external-dags")
	assert.ErrorIs(t, err, assert.AnError)

	// --pytest/--parse build and run a local image to test-parse the DAGs, so the project-dir
	// check should still run even with --dags-path set.
	err = execDeployCmd("-f", "test-deployment-id", "--dags", "--dags-path", "./external-dags", "--pytest")
	assert.ErrorIs(t, err, assert.AnError)

	err = execDeployCmd("-f", "test-deployment-id", "--dags", "--dags-path", "./external-dags", "--parse")
	assert.ErrorIs(t, err, assert.AnError)
}

func TestDeployImageNameRejectsIncompatibleFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	DeployImage = func(deployInput cloud.InputDeploy, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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
		{"build-secret", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--build-secret", "id=mysecret,src=secrets.txt"}},
		{"multiple build-secret", []string{"-f", "test-deployment-id", "--image-name", "img:1", "--build-secret", "id=mysecret,src=secrets.txt", "--build-secret", "id=aws,src=credentials"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execDeployCmd(tc.args...)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "--image-name")
		})
	}
}

type NonDagsDeploySuite struct {
	suite.Suite
	mockV1Client     *astrov1_mocks.ClientWithResponsesInterface
	origV1Client     astrov1.APIClient
	origDeployBundle func(deployInput *cloud.DeployBundleInput) error
	origWorkingPath  string
	origWd           string
	tmpWorkingDir    string
}

func (s *NonDagsDeploySuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// Run from an isolated temp directory that is not within an Astro project, so
	// the non-DAG bundle path validation (which walks up to a .astro/config.yaml)
	// is deterministic regardless of where the repo lives.
	tmpDir, err := os.MkdirTemp("", "non-dags-test")
	s.Require().NoError(err)
	s.tmpWorkingDir = tmpDir
	s.origWd, err = os.Getwd()
	s.Require().NoError(err)
	s.Require().NoError(os.Chdir(tmpDir))
	s.origWorkingPath = config.WorkingPath
	config.WorkingPath = tmpDir

	s.origV1Client = astroV1Client
	s.origDeployBundle = DeployBundle
	s.mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = s.mockV1Client
}

func (s *NonDagsDeploySuite) TearDownTest() {
	s.mockV1Client.AssertExpectations(s.T())
	astroV1Client = s.origV1Client
	DeployBundle = s.origDeployBundle
	config.WorkingPath = s.origWorkingPath
	if s.origWd != "" {
		_ = os.Chdir(s.origWd)
	}
	if s.tmpWorkingDir != "" {
		_ = os.RemoveAll(s.tmpWorkingDir)
	}
}

func TestNonDagsDeploy(t *testing.T) {
	suite.Run(t, new(NonDagsDeploySuite))
}

func (s *NonDagsDeploySuite) TestRequiresMountPath() {
	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags")
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "--non-dags-mount-path is required")
}

func (s *NonDagsDeploySuite) TestBundleTypeDefaultsToNone() {
	var captured *cloud.DeployBundleInput
	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		captured = deployInput
		return nil
	}

	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x")
	assert.NoError(s.T(), err)
	s.Require().NotNil(captured)
	assert.Equal(s.T(), "none", captured.BundleType)
}

func (s *NonDagsDeploySuite) TestRejectsIncompatibleFlag() {
	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x", "--non-dags-bundle-type", "dbt", "--dags")
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "cannot use --dags with --non-dags")
}

func (s *NonDagsDeploySuite) TestProvidedDeploymentId() {
	var captured *cloud.DeployBundleInput
	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		captured = deployInput
		return nil
	}

	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x", "--non-dags-bundle-type", "dbt")
	assert.NoError(s.T(), err)
	s.Require().NotNil(captured)
	assert.Equal(s.T(), "test-deployment-id", captured.DeploymentID)
	assert.Equal(s.T(), "/usr/local/airflow/x", captured.MountPath)
	assert.Equal(s.T(), "dbt", captured.BundleType)
	assert.Equal(s.T(), s.tmpWorkingDir, captured.BundlePath)
}

func (s *NonDagsDeploySuite) TestWithinAstroProject() {
	projectDir, cleanup, err := config.CreateTempProject()
	assert.NoError(s.T(), err)
	defer cleanup()

	err = testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x", "--non-dags-bundle-type", "dbt", "--non-dags-local-path", projectDir)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "within an Astro project")
}

func (s *NonDagsDeploySuite) TestBundlePathDoesNotExist() {
	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x", "--non-dags-local-path", filepath.Join(s.tmpWorkingDir, "missing"))
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "does not exist")
}

func (s *NonDagsDeploySuite) TestBundlePathNotADirectory() {
	file := filepath.Join(s.tmpWorkingDir, "a-file")
	s.Require().NoError(os.WriteFile(file, []byte("x"), 0o600))

	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x", "--non-dags-local-path", file)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "is not a directory")
}
