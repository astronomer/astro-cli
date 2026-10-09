package astro

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/internal/project"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execDeployCmd(args ...string) error {
	// --workspace-id, and deploy itself when it resolves one, set the
	// package's workspaceID, which coalesceWorkspace reads before the context.
	// Left set, it would choose the Workspace of whatever runs next.
	defer func() { workspaceID = "" }()
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewDeployCmd()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

// deployIn points the deploy at dir for one test.
func deployIn(t *testing.T, dir string) {
	t.Helper()
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
}

// v2 deploys only pyproject.toml projects. A project in the Astro CLI 1.x
// layout is refused whatever the flags, naming the two ways forward, and
// neither the manifest path nor anything else runs. Outside any project the
// deploy gives the no-project advice. Both are no_project failures.
func TestDeployRefusesA1xProject(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	prevDeployer := newManifestDeployer
	t.Cleanup(func() { newManifestDeployer = prevDeployer })
	newManifestDeployer = func(*deployLogin, io.Reader, io.Writer) manifestdeploy.Deployer {
		t.Fatal("a refused deploy reaches no transport")
		return nil
	}

	oneX := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(oneX, "Dockerfile"), []byte("FROM quay.io/astronomer/astro-runtime:12.0.0\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(oneX, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(oneX, ".astro", "config.yaml"), []byte("project:\n  name: demo\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(oneX, "dags"), 0o755))
	deployIn(t, oneX)

	for _, args := range [][]string{
		{"test-deployment-id"},
		{"test-deployment-id", "--dags"},
		{"test-deployment-id", "--image-name", "img:1"},
		{"test-deployment-id", "--image"},
		{"test-deployment-id", "--force", "--output", "json"},
	} {
		err := execDeployCmd(args...)
		require.Error(t, err, "%v", args)
		assert.EqualError(t, err, utils.Deploy1xRefusedAstro, "%v", args)
		assert.Contains(t, err.Error(), "astro init")
		assert.Contains(t, err.Error(), "Astro CLI 1.x")
		var notFound *project.NotFoundError
		assert.True(t, errors.As(err, &notFound), "%v: reported as no_project", args)
	}

	empty := t.TempDir()
	deployIn(t, empty)
	for _, args := range [][]string{{"test-deployment-id"}, {"test-deployment-id", "--image-name", "img:1"}, {"test-deployment-id", "--dags"}} {
		err := execDeployCmd(args...)
		require.Error(t, err, "%v", args)
		assert.Contains(t, err.Error(), "this is not an Astro project directory.\nChange to an Astro project directory, or run astro init", "%v", args)
		assert.NotContains(t, err.Error(), "1.x", "%v", args)
		var notFound *project.NotFoundError
		assert.True(t, errors.As(err, &notFound), "%v: reported as no_project", args)
	}
}

// The flags only the 1.x deploy read are gone from the command: a run that
// passes one fails as an unknown flag here, and the root's removed-flags
// registry names what replaced it (cmd's TestRemovedFlagsSayWhatReplacedThem).
func TestDeployDropsThe1xOnlyFlags(t *testing.T) {
	cmd := NewDeployCmd()
	for _, name := range []string{"save", "pytest", "parse", "test", "env", "dags-path", "dag-bundle-name", "deployment-name"} {
		assert.Nil(t, cmd.Flags().Lookup(name), "--%s", name)
	}
	for _, letter := range []string{"s", "t", "e", "n"} {
		assert.Nil(t, cmd.Flags().ShorthandLookup(letter), "-%s", letter)
	}
}

type NonDagsDeploySuite struct {
	suite.Suite
	mockV1Client     *astrov1_mocks.ClientWithResponsesInterface
	origV1Client     astrov1.APIClient
	origDeployBundle func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error)
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
	var captured *astrodeploy.DeployBundleInput
	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		captured = deployInput
		return astrodeploy.BundleDeploy{}, nil
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
	var captured *astrodeploy.DeployBundleInput
	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		captured = deployInput
		return astrodeploy.BundleDeploy{}, nil
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

// --deployment names a non-Dag bundle's Deployment as the argument does.
func (s *NonDagsDeploySuite) TestDeploymentFlag() {
	var captured *astrodeploy.DeployBundleInput
	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		captured = deployInput
		return astrodeploy.BundleDeploy{}, nil
	}

	err := testExecCmd(NewDeployCmd(), "--deployment", "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x")
	assert.NoError(s.T(), err)
	s.Require().NotNil(captured)
	assert.Equal(s.T(), "test-deployment-id", captured.DeploymentID)
}

// --non-dags runs before the project decides anything: from a pyproject.toml
// project it deploys a bundle, not the project, so the project directory
// itself is refused as the bundle. It used to run a whole project deploy,
// dropping --non-dags.
func (s *NonDagsDeploySuite) TestFromAManifestProject() {
	s.Require().NoError(os.WriteFile(filepath.Join(s.tmpWorkingDir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	DeployBundle = func(*astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		s.Fail("a bundle inside the project is not deployed")
		return astrodeploy.BundleDeploy{}, nil
	}

	err := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x")
	assert.ErrorContains(s.T(), err, "within an Astro project")
}
