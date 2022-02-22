package cmd

import (
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDeployRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deploy")
	assert.EqualError(t, err, "not in a project directory")
	assert.Contains(t, output, "astro deploy")
}

func TestDeploymentNameExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ReleaseName: "dev"},
		{ReleaseName: "dev1"},
	}
	exists := deploymentNameExists("dev", deployments)
	if !exists {
		t.Errorf("deploymentNameExists(dev) = %t; want true", exists)
	}
}

func TestDeploymentNameDoesntExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ReleaseName: "dummy"},
	}
	exists := deploymentNameExists("dev", deployments)
	if exists {
		t.Errorf("deploymentNameExists(dev) = %t; want false", exists)
	}
}

func Test_validImageRepo(t *testing.T) {
	assert.True(t, validImageRepo("quay.io/astronomer/ap-airflow"))
	assert.True(t, validImageRepo("astronomerinc/ap-airflow"))
	assert.False(t, validImageRepo("personal-repo/ap-airflow"))
}

func TestBuildPushDockerImageSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) (airflow.ImageHandler, error) {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler, nil
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowVersions: []string{
			"2.1.0",
			"2.0.2",
			"2.0.0",
			"1.10.15",
			"1.10.14",
			"1.10.12",
			"1.10.10",
			"1.10.7",
			"1.10.5",
		},
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig").Return(mockedDeploymentConfig, nil)
	houstonClient = houstonMock

	err := buildPushDockerImage(config.Context{}, "test", "./testfiles/", "test", "test")
	assert.NoError(t, err)
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestBuildPushDockerImageFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowVersions: []string{
			"2.1.0",
			"2.0.2",
			"2.0.0",
			"1.10.15",
			"1.10.14",
			"1.10.12",
			"1.10.10",
			"1.10.7",
			"1.10.5",
		},
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig").Return(mockedDeploymentConfig, nil).Twice()
	houstonClient = houstonMock

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) (airflow.ImageHandler, error) {
		mockImageHandler.On("Build", mock.Anything).Return(errSomeContainerIssue)
		return mockImageHandler, nil
	}

	err := buildPushDockerImage(config.Context{}, "test", "./testfiles/", "test", "test")
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockImageHandler.AssertExpectations(t)

	mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) (airflow.ImageHandler, error) {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockImageHandler, nil
	}

	err = buildPushDockerImage(config.Context{}, "test", "./testfiles/", "test", "test")
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestGetAirflowUILink(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockDeployment = &houston.Deployment{
		ID:                    "cknz133ra49758zr9w34b87ua",
		Type:                  "airflow",
		Label:                 "test",
		ReleaseName:           "testDeploymentName",
		Version:               "0.15.6",
		AirflowVersion:        "2.0.0",
		DesiredAirflowVersion: "2.0.0",
		DeploymentInfo:        houston.DeploymentInfo{},
		Workspace: houston.Workspace{
			ID:    "ckn4phn1k0104v5xtrer5lpli",
			Label: "w1",
		},
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
		},
		CreatedAt: "2021-04-26T20:03:36.262Z",
		UpdatedAt: "2021-04-26T20:03:36.262Z",
	}

	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeployment", mock.Anything).Return(mockDeployment, nil)
	houstonClient = houstonMock

	expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/airflow"
	actualResult := getAirflowUILink("testDeploymentID")
	assert.Equal(t, expectedResult, actualResult)
	houstonMock.AssertExpectations(t)
}
