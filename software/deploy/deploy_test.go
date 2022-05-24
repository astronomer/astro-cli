package deploy

import (
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errSomeContainerIssue = errors.New("some container issue")
	errMockHouston        = errors.New("some houston error")
)

var mockAirflowImageList = []houston.AirflowImage{
	{Version: "2.1.0", Tag: "2.1.0-onbuild"},
	{Version: "2.0.2", Tag: "2.0.2-onbuild"},
	{Version: "2.0.0", Tag: "2.0.0-onbuild"},
	{Version: "1.10.15", Tag: "1.10.15-onbuild"},
	{Version: "1.10.14", Tag: "1.10.14-onbuild"},
	{Version: "1.10.12", Tag: "1.10.12-onbuild"},
	{Version: "1.10.7", Tag: "1.10.7-onbuild"},
	{Version: "1.10.5", Tag: "1.10.5-onbuild"},
}

func TestDeploymentExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ID: "dev-test-1"},
		{ID: "dev-test-2"},
	}
	exists := deploymentExists("dev-test-1", deployments)
	if !exists {
		t.Errorf("deploymentNameExists(dev) = %t; want true", exists)
	}
}

func TestDeploymentNameDoesntExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ID: "dev-test-1"},
		{ID: "dev-test-2"},
	}
	exists := deploymentExists("dev-test", deployments)
	if exists {
		t.Errorf("deploymentExists(dev-test) = %t; want false", exists)
	}
}

func Test_validImageRepo(t *testing.T) {
	assert.True(t, validImageRepo("quay.io/astronomer/ap-airflow"))
	assert.True(t, validImageRepo("astronomerinc/ap-airflow"))
	assert.False(t, validImageRepo("personal-repo/ap-airflow"))
}

func TestBuildPushDockerImageSuccessWithTagWarning(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	dockerfile = "Dockerfile.warning"
	defer func() { dockerfile = "Dockerfile" }()

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler
	}

	defer testUtil.MockUserInput(t, "y")()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig").Return(mockedDeploymentConfig, nil)

	err := buildPushDockerImage(houstonMock, &config.Context{}, "test", "./testfiles/", "test", "test", false)
	assert.NoError(t, err)
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestBuildPushDockerImageSuccessWithImageRepoWarning(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	dockerfile = "Dockerfile.privateImageRepo"
	defer func() { dockerfile = "Dockerfile" }()

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler
	}

	defer testUtil.MockUserInput(t, "y")()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig").Return(mockedDeploymentConfig, nil)

	err := buildPushDockerImage(houstonMock, &config.Context{}, "test", "./testfiles/", "test", "test", false)
	assert.NoError(t, err)
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestBuildPushDockerImageFailure(t *testing.T) {
	// invalid dockerfile test
	dockerfile = "Dockerfile.invalid"
	err := buildPushDockerImage(nil, &config.Context{}, "test", "./testfiles/", "test", "test", false)
	assert.EqualError(t, err, "failed to parse dockerfile: testfiles/Dockerfile.invalid: when using JSON array syntax, arrays must be comprised of strings only")
	dockerfile = "Dockerfile"

	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig").Return(nil, errMockHouston).Once()
	// houston GetDeploymentConfig call failure
	err = buildPushDockerImage(houstonMock, &config.Context{}, "test", "./testfiles/", "test", "test", false)
	assert.Error(t, err, errMockHouston)

	houstonMock.On("GetDeploymentConfig").Return(mockedDeploymentConfig, nil).Twice()

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(errSomeContainerIssue)
		return mockImageHandler
	}

	// build error test case
	err = buildPushDockerImage(houstonMock, &config.Context{}, "test", "./testfiles/", "test", "test", false)
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockImageHandler.AssertExpectations(t)

	mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockImageHandler
	}

	// push error test case
	err = buildPushDockerImage(houstonMock, &config.Context{}, "test", "./testfiles/", "test", "test", false)
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestGetAirflowUILink(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockDeployment := &houston.Deployment{
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

	expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/airflow"
	actualResult := getAirflowUILink(houstonMock, "testDeploymentID")
	assert.Equal(t, expectedResult, actualResult)
	houstonMock.AssertExpectations(t)
}

func TestGetAirflowUILinkFailure(t *testing.T) {
	actualResult := getAirflowUILink(nil, "")
	assert.Equal(t, actualResult, "")

	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()
	actualResult = getAirflowUILink(houstonMock, "testDeploymentID")
	assert.Equal(t, actualResult, "")
	houstonMock.AssertExpectations(t)

	mockDeployment := &houston.Deployment{
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
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
		},
		CreatedAt: "2021-04-26T20:03:36.262Z",
		UpdatedAt: "2021-04-26T20:03:36.262Z",
	}

	houstonMock.On("GetDeployment", mock.Anything).Return(mockDeployment, nil).Once()
	actualResult = getAirflowUILink(houstonMock, "testDeploymentID")
	assert.Equal(t, actualResult, "")
	houstonMock.AssertExpectations(t)
}

func TestAirflowFailure(t *testing.T) {
	// No workspace ID test case
	err := Airflow(nil, "", "", "", false, false)
	assert.ErrorIs(t, err, errNoWorkspaceID)

	// houston GetWorkspace failure case
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetWorkspace", mock.Anything).Return(nil, errMockHouston).Once()

	err = Airflow(houstonMock, "", "", "test-workspace-id", false, false)
	assert.ErrorIs(t, err, errMockHouston)
	houstonMock.AssertExpectations(t)

	// houston ListDeployments failure case
	houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil)
	houstonMock.On("ListDeployments", mock.Anything).Return(nil, errMockHouston).Once()

	err = Airflow(houstonMock, "", "", "test-workspace-id", false, false)
	assert.ErrorIs(t, err, errMockHouston)
	houstonMock.AssertExpectations(t)

	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil).Times(3)

	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	// config GetCurrentContext failure case
	config.ResetCurrentContext()

	err = Airflow(houstonMock, "", "", "test-workspace-id", false, false)
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	context.Switch("localhost")

	// Invalid deployment name case
	err = Airflow(houstonMock, "", "test-deployment-id", "test-workspace-id", false, false)
	assert.ErrorIs(t, err, errInvalidDeploymentName)

	// No deployment in the current workspace case
	err = Airflow(houstonMock, "", "", "test-workspace-id", false, false)
	assert.ErrorIs(t, err, errDeploymentNotFound)
	houstonMock.AssertExpectations(t)

	// Invalid deployment selection case
	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	err = Airflow(houstonMock, "", "", "test-workspace-id", false, false)
	assert.ErrorIs(t, err, errInvalidDeploymentSelected)

	// buildPushDockerImage failure case
	dockerfile = "Dockerfile.invalid"
	err = Airflow(houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false)
	dockerfile = "Dockerfile"
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse dockerfile")
}

func TestAirflowSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	houstonMock.On("GetDeploymentConfig").Return(mockedDeploymentConfig, nil).Once()
	houstonMock.On("GetDeployment", mock.Anything).Return(nil, nil).Once()

	err := Airflow(houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false)
	assert.Nil(t, err)
	houstonMock.AssertExpectations(t)
}
