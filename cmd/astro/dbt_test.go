package astro

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type DbtSuite struct {
	suite.Suite
	mockV1Client     *astrov1_mocks.ClientWithResponsesInterface
	origV1Client     astrov1.APIClient
	origDeployBundle func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error)
	origDeleteBundle func(deleteInput *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error)
	origWorkingPath  string
	origWd           string
	tmpWorkingDir    string
}

func (s *DbtSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// Run each test from an isolated temp directory that is not within an Astro
	// project. dbt deploy/delete default the project path to config.WorkingPath
	// and reject paths nested under an Astro project (detected by walking up the
	// real filesystem for a .astro/config.yaml). Relying on the package's own
	// directory makes the suite fail whenever an ancestor of the repo happens to
	// be an Astro project, so we point WorkingPath/cwd at a clean temp dir.
	tmpDir, err := os.MkdirTemp("", "dbt-test")
	s.Require().NoError(err)
	s.tmpWorkingDir = tmpDir
	s.origWd, err = os.Getwd()
	s.Require().NoError(err)
	s.Require().NoError(os.Chdir(tmpDir))
	s.origWorkingPath = config.WorkingPath
	config.WorkingPath = tmpDir

	// the package depends on global variables so we need to manage overriding those
	s.origV1Client = astroV1Client
	s.origDeployBundle = DeployBundle
	s.origDeleteBundle = DeleteBundle
	s.mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = s.mockV1Client
}

func (s *DbtSuite) TearDownTest() {
	s.mockV1Client.AssertExpectations(s.T())
	astroV1Client = s.origV1Client
	DeployBundle = s.origDeployBundle
	DeleteBundle = s.origDeleteBundle

	config.WorkingPath = s.origWorkingPath
	if s.origWd != "" {
		_ = os.Chdir(s.origWd)
	}
	if s.tmpWorkingDir != "" {
		_ = os.RemoveAll(s.tmpWorkingDir)
	}
}

func TestDbt(t *testing.T) {
	suite.Run(t, new(DbtSuite))
}

func (s *DbtSuite) TestDbtDeploy_PickDeployment() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		return astrodeploy.BundleDeploy{}, nil
	}

	s.mockListTestDeployments()
	s.mockGetTestDeployment()

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeployCmd())
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDeploy_ProvidedDeploymentId() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		return astrodeploy.BundleDeploy{}, nil
	}

	err := testExecCmd(newDbtDeployCmd(), "test-deployment-id")
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDeploy_CustomProjectPath() {
	projectPath, err := os.MkdirTemp("", "")
	assert.NoError(s.T(), err)
	defer os.RemoveAll(projectPath)

	s.createDbtProjectFile(filepath.Join(projectPath, "dbt_project.yml"))
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		if deployInput.BundlePath != projectPath {
			return astrodeploy.BundleDeploy{}, assert.AnError
		}
		return astrodeploy.BundleDeploy{}, nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err = testExecCmd(newDbtDeployCmd(), "test-deployment-id", "--project-path", projectPath)
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDeploy_CustomMountPath() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		if deployInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return astrodeploy.BundleDeploy{}, assert.AnError
		}
		return astrodeploy.BundleDeploy{}, nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeployCmd(), "test-deployment-id", "--mount-path", dbtDefaultMountPathPrefix+"test_dbt_project")
	assert.NoError(s.T(), err)
}

// A dbt project in or below a 1.x project, by the walk every deploy decides
// by (a Dockerfile beside .astro, with or without .astro/config.yaml), is
// refused, as astro deploy --non-dags refuses the same bundle path.
func (s *DbtSuite) TestDbtDeploy_WithinAstroProject() {
	projectDir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(projectDir, "Dockerfile"), []byte("FROM x\n"), 0o600))
	s.Require().NoError(os.MkdirAll(filepath.Join(projectDir, ".astro"), 0o755))
	dbtDir := filepath.Join(projectDir, "include", "dbt")
	s.Require().NoError(os.MkdirAll(dbtDir, 0o755))

	for _, path := range []string{projectDir, dbtDir} {
		err := testExecCmd(newDbtDeployCmd(), "test-deployment-id", "--project-path", path)
		s.Require().Error(err, path)
		s.Contains(err.Error(), "dbt project is within an Astro project", path)

		resetDeployFlagVars()
		nonDags := testExecCmd(NewDeployCmd(), "test-deployment-id", "--non-dags", "--non-dags-mount-path", "/x", "--non-dags-local-path", path)
		s.Require().Error(nonDags, path)
		s.Contains(nonDags.Error(), "within an Astro project", "dbt deploy and --non-dags agree: %s", path)
	}
}

// A project with a pyproject.toml carries no .astro/config.yaml, so the 1.x walk answers false at
// every level of it. Before this, a dbt project nested inside one bundled and
// deployed instead of being refused.
func (s *DbtSuite) TestDbtDeploy_WithinManifestProject() {
	projectDir := s.T().TempDir()
	manifest := "[project]\nname = \"demo\"\n\n[tool.astro]\n"
	assert.NoError(s.T(), os.WriteFile(filepath.Join(projectDir, "pyproject.toml"), []byte(manifest), 0o600))
	nested := filepath.Join(projectDir, "analytics")
	assert.NoError(s.T(), os.MkdirAll(nested, 0o755))

	err := testExecCmd(newDbtDeployCmd(), "test-deployment-id", "--project-path", nested)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "dbt project is within an Astro project")
}

func (s *DbtSuite) TestDbtDelete_PickDeployment() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		return astrodeploy.BundleDelete{}, nil
	}

	s.mockListTestDeployments()
	s.mockGetTestDeployment()

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeleteCmd())
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDelete_ProvidedDeploymentId() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		return astrodeploy.BundleDelete{}, nil
	}

	err := testExecCmd(newDbtDeleteCmd(), "test-deployment-id")
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDelete_CustomProjectPath() {
	projectPath, err := os.MkdirTemp("", "")
	assert.NoError(s.T(), err)
	defer os.RemoveAll(projectPath)

	s.createDbtProjectFile(filepath.Join(projectPath, "dbt_project.yml"))
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		if deleteInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return astrodeploy.BundleDelete{}, assert.AnError
		}
		return astrodeploy.BundleDelete{}, nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err = testExecCmd(newDbtDeleteCmd(), "test-deployment-id", "--project-path", projectPath)
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDelete_NoMountPathOrProjectPath() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		if deleteInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return astrodeploy.BundleDelete{}, assert.AnError
		}
		return astrodeploy.BundleDelete{}, nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeleteCmd(), "test-deployment-id")
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) TestDbtDelete_NoMountPathOrProjectPath_MissingProject() {
	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeleteCmd(), "test-deployment-id")
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "dbt project file not found")
}

func (s *DbtSuite) TestDbtDelete_CustomMountPath() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		if deleteInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return astrodeploy.BundleDelete{}, assert.AnError
		}
		return astrodeploy.BundleDelete{}, nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeleteCmd(), "test-deployment-id", "--mount-path", dbtDefaultMountPathPrefix+"test_dbt_project")
	assert.NoError(s.T(), err)
}

func testExecCmd(cmd *cobra.Command, args ...string) error {
	if args == nil {
		args = []string{}
	}
	// deploy and dbt set the package's workspaceID from --workspace-id or the
	// one they resolve, and coalesceWorkspace reads it before the context.
	// Left set, it would choose the Workspace of whatever runs next.
	defer func() { workspaceID = "" }()
	testUtil.SetupOSArgsForGinkgo()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func (s *DbtSuite) createDbtProjectFile(path string) {
	file, err := os.Create(path)
	assert.NoError(s.T(), err)
	defer file.Close()
	_, err = file.WriteString("name: test_dbt_project")
	assert.NoError(s.T(), err)
}

func (s *DbtSuite) mockListTestDeployments() {
	s.mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: []astrov1.Deployment{
				{
					Id: "test-deployment-id",
				},
			},
		},
	}, nil)
}

func (s *DbtSuite) mockGetTestDeployment() {
	s.mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
		JSON200: &astrov1.Deployment{
			Id: "test-deployment-id",
		},
	}, nil)
}

func (s *DbtSuite) TestDbtCleanup_RemovesArtifacts() {
	dir := s.T().TempDir()
	artifact := filepath.Join(dir, ".astro", "dbt_metadata.json")
	assert.NoError(s.T(), os.MkdirAll(filepath.Dir(artifact), 0o755))
	assert.NoError(s.T(), os.WriteFile(artifact, []byte(`{"generated_by": {"application": "astro"}}`), 0o644))

	err := testExecCmd(newDbtCleanupCmd(), dir)
	assert.NoError(s.T(), err)
	assert.NoFileExists(s.T(), artifact)
}

func (s *DbtSuite) TestDbtCleanup_NonexistentPath() {
	err := testExecCmd(newDbtCleanupCmd(), filepath.Join(s.T().TempDir(), "does-not-exist"))
	assert.Error(s.T(), err)
}
