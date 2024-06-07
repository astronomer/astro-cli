package cloud

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDbtDeploy_PickDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	defer createDbtProjectFile(t, "dbt_project.yml")()

	DeployBundle = func(deployInput *cloud.DeployBundleInput, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		return nil
	}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockListTestDeployments(mockPlatformCoreClient)
	mockGetTestDeployment(mockPlatformCoreClient)
	defer setPlatformCoreClient(mockPlatformCoreClient)()

	defer testUtil.MockUserInput(t, "1")()
	err := execDbtDeployCmd()
	assert.NoError(t, err)

	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDbtDeploy_ProvidedDeploymentId(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	defer createDbtProjectFile(t, "dbt_project.yml")()

	DeployBundle = func(deployInput *cloud.DeployBundleInput, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		return nil
	}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	defer setPlatformCoreClient(mockPlatformCoreClient)()

	err := execDbtDeployCmd("test-deployment-id")
	assert.NoError(t, err)

	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDbtDeploy_CustomProjectPath(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	projectPath := "/tmp/test_dbt_project"
	err := os.MkdirAll(projectPath, os.ModePerm)
	assert.NoError(t, err)
	defer os.RemoveAll(projectPath)
	createDbtProjectFile(t, filepath.Join(projectPath, "dbt_project.yml"))

	DeployBundle = func(deployInput *cloud.DeployBundleInput, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		if deployInput.BundlePath != projectPath {
			return assert.AnError
		}
		return nil
	}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	defer setPlatformCoreClient(mockPlatformCoreClient)()

	defer testUtil.MockUserInput(t, "1")()
	err = execDbtDeployCmd("test-deployment-id", "--project-path", "/tmp/test_dbt_project")
	assert.NoError(t, err)

	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDbtDeploy_CustomMountPath(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	defer createDbtProjectFile(t, "dbt_project.yml")()

	DeployBundle = func(deployInput *cloud.DeployBundleInput, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
		if deployInput.MountPath != dbtDefaultMountPathPrefix+"test_dbt_project" {
			return assert.AnError
		}
		return nil
	}

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	defer setPlatformCoreClient(mockPlatformCoreClient)()

	defer testUtil.MockUserInput(t, "1")()
	err := execDbtDeployCmd("test-deployment-id", "--mount-path", dbtDefaultMountPathPrefix+"test_dbt_project")
	assert.NoError(t, err)

	mockPlatformCoreClient.AssertExpectations(t)
}

func execDbtDeployCmd(args ...string) error {
	if len(args) == 0 {
		args = []string{}
	}
	testUtil.SetupOSArgsForGinkgo()
	cmd := newDbtDeployCmd()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func createDbtProjectFile(t *testing.T, path string) func() {
	file, err := os.Create(path)
	assert.NoError(t, err)
	defer file.Close()
	_, err = file.WriteString("name: test_dbt_project")
	assert.NoError(t, err)
	return func() { os.Remove(path) }
}

func mockListTestDeployments(client *astroplatformcore_mocks.ClientWithResponsesInterface) {
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListDeploymentsResponse{
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

func mockGetTestDeployment(client *astroplatformcore_mocks.ClientWithResponsesInterface) {
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: astrocore.HTTPStatus200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id: "test-deployment-id",
		},
	}, nil)
}

func setPlatformCoreClient(client astroplatformcore.CoreClient) func() {
	platformCoreClient = client
	return func() { platformCoreClient = nil }
}
