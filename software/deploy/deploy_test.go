package deploy

import (
	"errors"
	"testing"
	"time"

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
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}
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

func TestValidAirflowImageRepo(t *testing.T) {
	assert.True(t, validAirflowImageRepo("quay.io/astronomer/ap-airflow"))
	assert.True(t, validAirflowImageRepo("astronomerinc/ap-airflow"))
	assert.False(t, validAirflowImageRepo("personal-repo/ap-airflow"))
}

func TestValidRuntimeImageRepo(t *testing.T) {
	assert.False(t, validRuntimeImageRepo("quay.io/astronomer/ap-airflow"))
	assert.True(t, validRuntimeImageRepo("quay.io/astronomer/astro-runtime"))
	assert.False(t, validRuntimeImageRepo("personal-repo/ap-airflow"))
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
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler
	}

	defer testUtil.MockUserInput(t, "y")()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)

	err := buildPushDockerImage(houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false)
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
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler
	}

	defer testUtil.MockUserInput(t, "y")()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)

	err := buildPushDockerImage(houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false)
	assert.NoError(t, err)
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestBuildPushDockerImageSuccessWithBYORegistry(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	dockerfile = "Dockerfile"
	defer func() { dockerfile = "Dockerfile" }()

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", "", runtimeImageLabel).Return("", nil).Once()
		mockImageHandler.On("GetLabel", "", airflowImageLabel).Return("1.10.12", nil).Once()
		return mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io:test-test", AirflowVersion: "1.10.12", RuntimeVersion: ""}).Return(nil, nil)

	err := buildPushDockerImage(houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true)
	assert.NoError(t, err)
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestBuildPushDockerImageFailure(t *testing.T) {
	// invalid dockerfile test
	dockerfile = "Dockerfile.invalid"
	err := buildPushDockerImage(nil, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false)
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
	houstonMock.On("GetDeploymentConfig", nil).Return(nil, errMockHouston).Once()
	houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	// houston GetDeploymentConfig call failure
	err = buildPushDockerImage(houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false)
	assert.Error(t, err, errMockHouston)

	houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Twice()

	mockImageHandler := new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockImageHandler
	}

	// build error test case
	err = buildPushDockerImage(houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false)
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockImageHandler.AssertExpectations(t)

	mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockImageHandler
	}

	// push error test case
	err = buildPushDockerImage(houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false)
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockImageHandler.AssertExpectations(t)
	houstonMock.AssertExpectations(t)
}

func TestGetAirflowUILink(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
	}

	expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/airflow"
	actualResult := getAirflowUILink("testDeploymentID", mockURLs)
	assert.Equal(t, expectedResult, actualResult)
}

func TestGetAirflowUILinkFailure(t *testing.T) {
	actualResult := getAirflowUILink("", []houston.DeploymentURL{})
	assert.Equal(t, actualResult, "")

	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	actualResult = getAirflowUILink("testDeploymentID", []houston.DeploymentURL{})
	assert.Equal(t, actualResult, "")
}

func TestAirflowFailure(t *testing.T) {
	// No workspace ID test case
	err := Airflow(nil, "", "", "", "", false, false, false)
	assert.ErrorIs(t, err, errNoWorkspaceID)

	// houston GetWorkspace failure case
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetWorkspace", mock.Anything).Return(nil, errMockHouston).Once()

	err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errMockHouston)
	houstonMock.AssertExpectations(t)

	// houston ListDeployments failure case
	houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil)
	houstonMock.On("ListDeployments", mock.Anything).Return(nil, errMockHouston).Once()

	err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errMockHouston)
	houstonMock.AssertExpectations(t)

	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil).Times(3)

	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	// config GetCurrentContext failure case
	config.ResetCurrentContext()

	err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	context.Switch("localhost")

	// Invalid deployment name case
	err = Airflow(houstonMock, "", "test-deployment-id", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errInvalidDeploymentID)

	// No deployment in the current workspace case
	err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errDeploymentNotFound)
	houstonMock.AssertExpectations(t)

	// Invalid deployment selection case
	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errInvalidDeploymentSelected)

	// buildPushDockerImage failure case
	houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{}, nil)
	dockerfile = "Dockerfile.invalid"
	err = Airflow(houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false)
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
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	mockRuntimeReleases := houston.RuntimeReleases{
		houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
		houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
	}
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Once()
	houstonMock.On("GetDeployment", mock.Anything).Return(&houston.Deployment{}, nil).Once()
	houstonMock.On("GetRuntimeReleases", "").Return(mockRuntimeReleases, nil)

	err := Airflow(houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false)
	assert.Nil(t, err)
	houstonMock.AssertExpectations(t)
}

func TestDeployDagsOnlyFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "test-deployment-id"

	t.Run("When config flag is set to false", func(t *testing.T) {
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		houstonMock.On("GetAppConfig", nil).Return(appConfig, nil)

		err := DeployDagsOnly(houstonMock, appConfig, deploymentID)
		assert.ErrorIs(t, err, errDagOnlyDeployDisabledInConfig)
	})

	t.Run("When config flag is set to true but an error occurs in the GetDeployment api call", func(t *testing.T) {
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		houstonMock.On("GetAppConfig", nil).Return(appConfig, nil)
		houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()

		err := DeployDagsOnly(houstonMock, appConfig, deploymentID)
		assert.ErrorContains(t, err, "failed to get deployment info: some houston error")
	})

	t.Run("When config flag is set to true but it is disabled at the deployment level", func(t *testing.T) {
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		houstonMock.On("GetAppConfig", nil).Return(appConfig, nil)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.VolumeDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		err := DeployDagsOnly(houstonMock, appConfig, deploymentID)
		assert.ErrorIs(t, err, errDagOnlyDeployNotEnabledForDeployment)
	})

}
