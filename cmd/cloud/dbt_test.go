package cloud

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type DbtSuite struct {
	suite.Suite
	mockPlatformCoreClient *astroplatformcore_mocks.ClientWithResponsesInterface
	mockCoreClient         *astrocore_mocks.ClientWithResponsesInterface
	origPlatformCoreClient astroplatformcore.CoreClient
	origCoreClient         astrocore.CoreClient
	origDeployBundle       func(deployInput *cloud.DeployBundleInput) error
	origDeleteBundle       func(deleteInput *cloud.DeleteBundleInput) error
}

func (s *DbtSuite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// the package depends on global variables so we need to manage overriding those
	s.origPlatformCoreClient = platformCoreClient
	s.origCoreClient = astroCoreClient
	s.origDeployBundle = DeployBundle
	s.origDeleteBundle = DeleteBundle
	s.mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	s.mockCoreClient = new(astrocore_mocks.ClientWithResponsesInterface)
	platformCoreClient = s.mockPlatformCoreClient
	astroCoreClient = s.mockCoreClient
}

func (s *DbtSuite) TearDownTest() {
	platformCoreClient = s.origPlatformCoreClient
	astroCoreClient = s.origCoreClient
	DeployBundle = s.origDeployBundle
	DeleteBundle = s.origDeleteBundle
}

func TestDbt(t *testing.T) {
	suite.Run(t, new(DbtSuite))
}

func (s *DbtSuite) TestDbtDeploy_PickDeployment() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		return nil
	}

	s.mockListTestDeployments()
	s.mockGetTestDeployment()

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeployCmd())
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDeploy_ProvidedDeploymentId() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		return nil
	}

	err := testExecCmd(newDbtDeployCmd(), "test-deployment-id")
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDeploy_CustomProjectPath() {
	projectPath, err := os.MkdirTemp("", "")
	assert.NoError(s.T(), err)
	defer os.RemoveAll(projectPath)

	s.createDbtProjectFile(filepath.Join(projectPath, "dbt_project.yml"))
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		if deployInput.BundlePath != projectPath {
			return assert.AnError
		}
		return nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err = testExecCmd(newDbtDeployCmd(), "test-deployment-id", "--project-path", projectPath)
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDeploy_CustomMountPath() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeployBundle = func(deployInput *cloud.DeployBundleInput) error {
		if deployInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return assert.AnError
		}
		return nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeployCmd(), "test-deployment-id", "--mount-path", dbtDefaultMountPathPrefix+"test_dbt_project")
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDelete_PickDeployment() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *cloud.DeleteBundleInput) error {
		return nil
	}

	s.mockListTestDeployments()
	s.mockGetTestDeployment()

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeleteCmd())
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDelete_ProvidedDeploymentId() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *cloud.DeleteBundleInput) error {
		return nil
	}

	err := testExecCmd(newDbtDeleteCmd(), "test-deployment-id")
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDelete_CustomProjectPath() {
	projectPath, err := os.MkdirTemp("", "")
	assert.NoError(s.T(), err)
	defer os.RemoveAll(projectPath)

	s.createDbtProjectFile(filepath.Join(projectPath, "dbt_project.yml"))
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *cloud.DeleteBundleInput) error {
		if deleteInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return assert.AnError
		}
		return nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err = testExecCmd(newDbtDeleteCmd(), "test-deployment-id", "--project-path", projectPath)
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func (s *DbtSuite) TestDbtDelete_CustomMountPath() {
	s.createDbtProjectFile("dbt_project.yml")
	defer os.Remove("dbt_project.yml")

	DeleteBundle = func(deleteInput *cloud.DeleteBundleInput) error {
		if deleteInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return assert.AnError
		}
		return nil
	}

	defer testUtil.MockUserInput(s.T(), "1")()
	err := testExecCmd(newDbtDeleteCmd(), "test-deployment-id", "--mount-path", dbtDefaultMountPathPrefix+"test_dbt_project")
	assert.NoError(s.T(), err)

	s.mockPlatformCoreClient.AssertExpectations(s.T())
}

func testExecCmd(cmd *cobra.Command, args ...string) error {
	if args == nil {
		args = []string{}
	}
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
	s.mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{
				{
					Id: "test-deployment-id",
				},
			},
		},
	}, nil)
}

func (s *DbtSuite) mockGetTestDeployment() {
	s.mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id: "test-deployment-id",
		},
	}, nil)
}
