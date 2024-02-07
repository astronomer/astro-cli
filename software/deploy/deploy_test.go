package deploy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/fileutil"
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
	_, err := Airflow(nil, "", "", "", "", false, false, false)
	assert.ErrorIs(t, err, ErrNoWorkspaceID)

	// houston GetWorkspace failure case
	houstonMock := new(houston_mocks.ClientInterface)
	houstonMock.On("GetWorkspace", mock.Anything).Return(nil, errMockHouston).Once()

	_, err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errMockHouston)
	houstonMock.AssertExpectations(t)

	// houston ListDeployments failure case
	houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil)
	houstonMock.On("ListDeployments", mock.Anything).Return(nil, errMockHouston).Once()

	_, err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errMockHouston)
	houstonMock.AssertExpectations(t)

	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil).Times(3)

	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig("localhost")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	// config GetCurrentContext failure case
	config.ResetCurrentContext()

	_, err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	context.Switch("localhost")

	// Invalid deployment name case
	_, err = Airflow(houstonMock, "", "test-deployment-id", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errInvalidDeploymentID)

	// No deployment in the current workspace case
	_, err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errDeploymentNotFound)
	houstonMock.AssertExpectations(t)

	// Invalid deployment selection case
	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	_, err = Airflow(houstonMock, "", "", "test-workspace-id", "", false, false, false)
	assert.ErrorIs(t, err, errInvalidDeploymentSelected)

	// return error When houston get deployment throws an error
	houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()
	_, err = Airflow(houstonMock, "", "test-deployment-id", "test-workspace-id", "", false, false, false)
	assert.Equal(t, err.Error(), "failed to get deployment info: "+errMockHouston.Error())

	// buildPushDockerImage failure case
	houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{}, nil)
	dockerfile = "Dockerfile.invalid"
	_, err = Airflow(houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false)
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

	_, err := Airflow(houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false)
	assert.Nil(t, err)
	houstonMock.AssertExpectations(t)
}

func TestDeployDagsOnlyFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "test-deployment-id"
	wsID := "test-workspace-id"

	t.Run("When config flag is set to false", func(t *testing.T) {
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)

		err := DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false)
		assert.ErrorIs(t, err, ErrDagOnlyDeployDisabledInConfig)
		houstonMock.AssertExpectations(t)
	})

	t.Run("When getDeploymentIdForCurrentCommandVar gives an error", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, errDeploymentNotFound
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)

		err := DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false)
		assert.ErrorIs(t, err, errDeploymentNotFound)
		houstonMock.AssertExpectations(t)
	})

	t.Run("When config flag is set to true but an error occurs in the GetDeployment api call", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()

		err := DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false)
		assert.ErrorContains(t, err, "failed to get deployment info: some houston error")
		houstonMock.AssertExpectations(t)
	})

	t.Run("When config flag is set to true but it is disabled at the deployment level", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.VolumeDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		err := DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false)
		assert.ErrorIs(t, err, errDagOnlyDeployNotEnabledForDeployment)
		houstonMock.AssertExpectations(t)
	})

	t.Run("Valid Houston config, but unable to get context from astro-cli config", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		config.ResetCurrentContext()

		err := DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false)
		assert.EqualError(t, err, "could not get current context! Error: no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		houstonMock.AssertExpectations(t)
		context.Switch("localhost")
	})

	t.Run("Valid Houston config, able to get context from config but no release name present", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		err := DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false)
		houstonMock.AssertExpectations(t)
		assert.ErrorIs(t, err, errInvalidDeploymentID)
	})

	t.Run("Valid Houston config. Valid Houston deployment. The Dags folder is empty. User doesn't give operation confirmation", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "n")()

		// create the empty dags folder
		err = os.Mkdir("dags", os.ModePerm)
		assert.NoError(t, err)
		defer os.RemoveAll("dags")

		err = DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, ".", nil, false)
		assert.EqualError(t, err, ErrEmptyDagFolderUserCancelledOperation.Error())

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		assert.True(t, os.IsNotExist(err))
		houstonMock.AssertExpectations(t)
	})

	t.Run("Valid Houston config. Valid Houston deployment. The Dags folder is empty. User gives the operation confirmation", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "y")()

		// create the empty dags folder
		err = os.Mkdir("dags", os.ModePerm)
		assert.NoError(t, err)
		defer os.RemoveAll("dags")

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			assert.Equal(t, http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			assert.NoError(t, err, "Error parsing multipart form")
			assert.NotNil(t, r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err = DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, ".", &server.URL, false)
		assert.NoError(t, err)
		houstonMock.AssertExpectations(t)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		assert.NoError(t, err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was created
		destFilePath = "./dags.tar.gz"
		_, err = os.ReadFile(destFilePath)
		assert.NoError(t, err, "Error reading gZipped file")
		defer os.Remove(destFilePath)
	})

	t.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. Tar creation throws an error", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(t, "y")()

		err = DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, "./dags", nil, false)
		assert.EqualError(t, err, "open dags/dags.tar: no such file or directory")
		houstonMock.AssertExpectations(t)

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat("./dags.tar.gz")
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. Tar is successfully created. But gzip creation throws an error", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		assert.NoError(t, err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		assert.NoError(t, err)

		gzipMockError := errors.New("some gzip error") //nolint

		// mock the gzip creation to throw an error
		gzipFile = func(srcFilePath, destFilePath string) error {
			return gzipMockError
		}

		err = DagsOnlyDeploy(houstonMock, appConfig, deploymentID, wsID, ".", nil, false)
		assert.ErrorIs(t, err, gzipMockError)
		houstonMock.AssertExpectations(t)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		assert.NoError(t, err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was not created
		destFilePath = "./dags.tar.gz"
		_, err = os.Stat(destFilePath)
		assert.True(t, os.IsNotExist(err))

		gzipFile = fileutil.GzipFile
	})

	t.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. No need of User confirmation", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		assert.NoError(t, err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		assert.NoError(t, err)

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			assert.Equal(t, http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			assert.NoError(t, err, "Error parsing multipart form")
			assert.NotNil(t, r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err = DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, ".", &server.URL, false)
		assert.NoError(t, err)
		houstonMock.AssertExpectations(t)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		assert.NoError(t, err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was created
		destFilePath = "./dags.tar.gz"
		_, err = os.ReadFile(destFilePath)
		assert.NoError(t, err, "Error reading gZipped file")
		defer os.Remove(destFilePath)
	})

	t.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. No need of User confirmation. Files should be auto-cleaned", func(t *testing.T) {
		getDeploymentIdForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID string, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		houstonMock := new(houston_mocks.ClientInterface)
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		assert.NoError(t, err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		assert.NoError(t, err)

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			assert.Equal(t, http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			assert.NoError(t, err, "Error parsing multipart form")
			assert.NotNil(t, r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err = DagsOnlyDeploy(houstonMock, appConfig, wsID, deploymentID, ".", &server.URL, true)
		assert.NoError(t, err)
		houstonMock.AssertExpectations(t)

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat("./dags.tar.gz")
		assert.True(t, os.IsNotExist(err))
	})
}
