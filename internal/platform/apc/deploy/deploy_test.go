package deploy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	errSomeContainerIssue          = errors.New("some container issue")
	errMockHouston                 = errors.New("some houston error")
	description                    = "Deployed via <astro deploy>"
	deployRevisionDescriptionLabel = "io.astronomer.deploy.revision.description"

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
			{URL: "registry.local.astronomer.io", Type: "registry"},
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

type Suite struct {
	suite.Suite
	fsForDockerConfig afero.Fs
	fsForLocalConfig  afero.Fs
	mockImageHandler  *mocks.ImageHandler
	houstonMock       *houston_mocks.ClientInterface
}

func TestDeploy(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestDeploymentExists() {
	deployments := []houston.Deployment{
		{ID: "dev-test-1"},
		{ID: "dev-test-2"},
	}
	s.True(deploymentExists("dev-test-1", deployments))
}

func (s *Suite) TestDeploymentNameDoesntExists() {
	deployments := []houston.Deployment{
		{ID: "dev-test-1"},
		{ID: "dev-test-2"},
	}
	s.False(deploymentExists("dev-test", deployments))
}

func (s *Suite) SetupSuite() {
	// Common setup logic for the test suite
	s.fsForLocalConfig = afero.NewMemMapFs()
	afero.WriteFile(s.fsForLocalConfig, config.HomeConfigFile, testUtil.NewTestConfig("localhost"), 0o777)

	s.fsForDockerConfig = afero.NewMemMapFs()
	afero.WriteFile(s.fsForLocalConfig, config.HomeConfigFile, testUtil.NewTestConfig("docker"), 0o777)
}

func (s *Suite) SetupTest() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	s.mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		return s.mockImageHandler
	}
	s.houstonMock = new(houston_mocks.ClientInterface)
}

func (s *Suite) SetupSubTest() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	s.mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		return s.mockImageHandler
	}
	s.houstonMock = new(houston_mocks.ClientInterface)
}

func (s *Suite) TearDownSubTest() {
	s.houstonMock.AssertExpectations(s.T())
	s.mockImageHandler.AssertExpectations(s.T())
}

func (s *Suite) TearDownSuite() {
	// Cleanup logic, if any (e.g., clearing mocks)
	s.mockImageHandler = nil
	s.houstonMock = nil
	s.fsForDockerConfig = nil
	s.fsForLocalConfig = nil
	imageHandlerInit = airflow.ImageHandlerInit
}

func (s *Suite) TearDownTest() {
	s.houstonMock.AssertExpectations(s.T())
	s.mockImageHandler.AssertExpectations(s.T())
}

func (s *Suite) TestBuildPushDockerImageSuccessWithTagWarning() {
	config.InitConfig(s.fsForDockerConfig)
	context.Switch("localhost")
	dockerfile = "Dockerfile.warning"
	defer func() { dockerfile = "Dockerfile" }()

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return s.mockImageHandler
	}

	defer testUtil.MockUserInput(s.T(), "y")()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	vars := make(map[string]interface{})
	vars["clusterId"] = ""
	s.houstonMock.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Once()

	_, err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "", Options{})
	s.NoError(err)
}

func (s *Suite) TestBuildPushDockerImageSuccessWithImageRepoWarning() {
	config.InitConfig(s.fsForDockerConfig)
	context.Switch("localhost")
	dockerfile = "Dockerfile.privateImageRepo"
	defer func() { dockerfile = "Dockerfile" }()

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return s.mockImageHandler
	}

	defer testUtil.MockUserInput(s.T(), "y")()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	vars := make(map[string]interface{})
	vars["clusterId"] = ""
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	s.houstonMock.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Once()

	_, err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "", Options{})
	s.NoError(err)
}

func (s *Suite) TestBuildPushDockerImageSuccessWithBYORegistry() {
	config.InitConfig(s.fsForDockerConfig)
	dockerfile = "Dockerfile"
	defer func() { dockerfile = "Dockerfile" }()

	var capturedBuildConfig types.ImageBuildConfig

	imageHandlerInit = func(image string) airflow.ImageHandler {
		// Mock the Build function, capturing the buildConfig
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.MatchedBy(func(buildConfig types.ImageBuildConfig) bool {
			// Capture buildConfig for later assertions
			capturedBuildConfig = buildConfig
			// Check if the deploy label contains the correct description
			for _, label := range buildConfig.Labels {
				if label == deployRevisionDescriptionLabel+"="+description {
					return true
				}
			}
			return false
		})).Return(nil).Once()

		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("image_sha", nil)
		s.mockImageHandler.On("GetLabel", "", runtimeImageLabel).Return("", nil).Once()
		s.mockImageHandler.On("GetLabel", "", airflowImageLabel).Return("1.10.12", nil).Once()

		return s.mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	vars := make(map[string]interface{})
	vars["clusterId"] = ""
	s.houstonMock.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io:test-test", AirflowVersion: "1.10.12", RuntimeVersion: ""}).Return(nil, nil)

	_, err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true, description, "", Options{})
	s.NoError(err)

	expectedLabel := deployRevisionDescriptionLabel + "=" + description
	assert.Contains(s.T(), capturedBuildConfig.Labels, expectedLabel)

	// Set up expectations for SHA tag test
	s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io@image_sha", AirflowVersion: "1.10.12", RuntimeVersion: ""}).Return(nil, nil)

	// Reset image handler for SHA tag test
	s.mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		// Mock the Build function, capturing the buildConfig
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.MatchedBy(func(buildConfig types.ImageBuildConfig) bool {
			// Capture buildConfig for later assertions
			capturedBuildConfig = buildConfig
			// Check if the deploy label contains the correct description
			for _, label := range buildConfig.Labels {
				if label == deployRevisionDescriptionLabel+"="+description {
					return true
				}
			}
			return false
		})).Return(nil).Once()

		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("image_sha", nil)
		s.mockImageHandler.On("GetLabel", "", runtimeImageLabel).Return("", nil).Once()
		s.mockImageHandler.On("GetLabel", "", airflowImageLabel).Return("1.10.12", nil).Once()
		return s.mockImageHandler
	}
	config.CFG.ShaAsTag.SetHomeString("true")
	defer config.CFG.ShaAsTag.SetHomeString("false")
	_, err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true, description, "", Options{})
	s.NoError(err)
	expectedLabel = deployRevisionDescriptionLabel + "=" + description
	assert.Contains(s.T(), capturedBuildConfig.Labels, expectedLabel)
}

func (s *Suite) TestBuildPushDockerImageSuccessWithBYORegistryAndCustomImageName() {
	config.InitConfig(s.fsForDockerConfig)
	dockerfile = "Dockerfile"
	defer func() { dockerfile = "Dockerfile" }()

	customImageName := "test-image-name:latest"
	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("TagLocalImage", customImageName).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("image_sha", nil)
		s.mockImageHandler.On("GetLabel", "", runtimeImageLabel).Return("12.2.0", nil)
		s.mockImageHandler.On("GetLabel", "", airflowImageLabel).Return("1.10.12", nil)

		return s.mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	vars := make(map[string]interface{})
	vars["clusterId"] = ""
	s.houstonMock.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io:latest", AirflowVersion: "1.10.12", RuntimeVersion: "12.2.0"}).Return(nil, nil)

	_, err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true, description, customImageName, Options{})
	s.NoError(err)
}

func (s *Suite) TestBuildPushDockerImageFailure() {
	// invalid dockerfile test
	dockerfile = "Dockerfile.invalid"
	_, err := buildPushDockerImage(nil, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "", Options{})
	s.EqualError(err, "failed to parse dockerfile: testfiles/Dockerfile.invalid: when using JSON array syntax, arrays must be comprised of strings only")
	dockerfile = "Dockerfile"

	config.InitConfig(s.fsForDockerConfig)
	context.Switch("localhost")
	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	s.houstonMock.On("GetDeploymentConfig", nil).Return(nil, errMockHouston).Once()
	vars := make(map[string]interface{})
	vars["clusterId"] = ""
	s.houstonMock.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, nil)
	// houston GetDeploymentConfig call failure
	_, err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "", Options{})
	s.Error(err, errMockHouston)

	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Twice()

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return s.mockImageHandler
	}

	// build error test case
	_, err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "", Options{})
	s.Error(err, errSomeContainerIssue.Error())
	s.mockImageHandler.AssertExpectations(s.T())

	s.mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", errSomeContainerIssue)
		return s.mockImageHandler
	}
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Once()

	// push error test case
	_, err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "", Options{})
	s.Error(err, errSomeContainerIssue.Error())
}

func (s *Suite) TestGetAirflowUILink() {
	config.InitConfig(s.fsForDockerConfig)
	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
	}

	expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/airflow"
	actualResult := getAirflowUILink("testDeploymentID", mockURLs)
	s.Equal(expectedResult, actualResult)
}

func (s *Suite) TestGetDeploymentRegistryURL() {
	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
		{URL: "registry.local.astronomer.io", Type: "registry"},
	}
	expectedResult := "registry.local.astronomer.io"
	actualResult, _ := getDeploymentRegistryURL(mockURLs)
	s.Equal(expectedResult, actualResult)
}

func (s *Suite) TestGetDeploymentRegistryURLFailure() {
	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
	}
	_, err := getDeploymentRegistryURL(mockURLs)
	s.EqualError(err, "no valid registry url found failed to push")
}

func (s *Suite) TestGetAirflowUILinkFailure() {
	actualResult := getAirflowUILink("", []houston.DeploymentURL{})
	s.Equal(actualResult, "")

	config.InitConfig(s.fsForLocalConfig)

	actualResult = getAirflowUILink("testDeploymentID", []houston.DeploymentURL{})
	s.Equal(actualResult, "")
}

func (s *Suite) TestGetDagDeployURL() {
	s.Run("Returns dagserver URL when available in deployment", func() {
		mockDeployment := &houston.Deployment{
			ReleaseName: "test-deployment",
			Urls: []houston.DeploymentURL{
				{URL: "https://deployments.local.astronomer.io/testDeploymentName/dags/upload", Type: houston.DagServerURLType},
				{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: houston.AirflowURLType},
				{URL: "registry.local.astronomer.io", Type: "registry"},
			},
		}

		expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/dags/upload"
		actualResult := getDagDeployURL(mockDeployment)
		s.Equal(expectedResult, actualResult)
	})

	s.Run("Constructs URL from airflow URL when dagserver URL not available in deployment", func() {
		mockDeployment := &houston.Deployment{
			ReleaseName: "test-deployment",
			Urls: []houston.DeploymentURL{
				{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: houston.AirflowURLType},
				{URL: "registry.local.astronomer.io", Type: "registry"},
			},
		}

		expectedResult := "https://deployments.local.astronomer.io/test-deployment/dags/upload"
		actualResult := getDagDeployURL(mockDeployment)
		s.Equal(expectedResult, actualResult)
	})

	s.Run("Returns empty string when no valid URLs available", func() {
		mockDeployment := &houston.Deployment{
			ReleaseName: "test-deployment",
			Urls: []houston.DeploymentURL{
				{URL: "https://flower.example.com", Type: "flower"},
				{URL: "registry.local.astronomer.io", Type: "registry"},
			},
		}

		expectedResult := ""
		actualResult := getDagDeployURL(mockDeployment)
		s.Equal(expectedResult, actualResult)
	})
}

func (s *Suite) TestAirflowFailure() {
	// No workspace ID test case
	_, err := Airflow(nil, "", "", "", false, false, description, false, "", Options{})
	s.ErrorIs(err, ErrNoWorkspaceID)

	// houston GetWorkspace failure case
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(nil, errMockHouston).Once()

	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", false, false, description, false, "", Options{})
	s.ErrorIs(err, errMockHouston)
	s.houstonMock.AssertExpectations(s.T())

	// houston ListDeployments failure case
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil)
	s.houstonMock.On("ListDeployments", mock.Anything).Return(nil, errMockHouston).Once()

	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", false, false, description, false, "", Options{})
	s.ErrorIs(err, errMockHouston)
	s.houstonMock.AssertExpectations(s.T())

	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil).Times(3)

	config.InitConfig(s.fsForLocalConfig)

	// config GetCurrentContext failure case
	config.ResetCurrentContext()

	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", false, false, description, false, "", Options{})
	s.EqualError(err, "no context set, have you authenticated to Astro or APC? Run astro login and try again")

	context.Switch("localhost")

	// Invalid deployment name case
	_, err = Airflow(s.houstonMock, "", "test-deployment-id", "test-workspace-id", false, false, description, false, "", Options{})
	s.ErrorIs(err, errInvalidDeploymentID)

	// No deployment in the current workspace case
	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", false, false, description, false, "", Options{})
	s.ErrorIs(err, errDeploymentNotFound)
	s.houstonMock.AssertExpectations(s.T())

	// Invalid deployment selection case
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", false, false, description, false, "", Options{})
	s.ErrorIs(err, errInvalidDeploymentSelected)

	// return error When houston get deployment throws an error
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	s.houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()
	_, err = Airflow(s.houstonMock, "", "test-deployment-id", "test-workspace-id", false, false, description, false, "", Options{})
	s.Equal(err.Error(), "failed to get deployment info: "+errMockHouston.Error())

	// buildPushDockerImage failure case
	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{ClusterID: "test-cluster-id"}, nil)
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil)
	dockerfile = "Dockerfile.invalid"
	_, err = Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, false, "", Options{})
	dockerfile = "Dockerfile"
	s.Error(err)
	s.Contains(err.Error(), "failed to parse dockerfile")
}

func (s *Suite) TestAirflowSuccess() {
	config.InitConfig(s.fsForLocalConfig)

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil).Once()
		return s.mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	mockRuntimeReleases := houston.RuntimeReleases{
		houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
		houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
	}
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Once()
	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{
		ClusterID: "test-cluster-id",
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
			{URL: "registry.local.astronomer.io", Type: "registry"},
		},
	}, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Once()
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Once()
	vars := make(map[string]interface{})
	vars["clusterId"] = "test-cluster-id"
	s.houstonMock.On("GetRuntimeReleases", vars).Return(mockRuntimeReleases, nil)

	progress := new(bytes.Buffer)
	var deployed Deployed
	var err error
	printed := stdoutOf(s, func() {
		deployed, err = Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, false, "", Options{Progress: progress})
	})
	s.NoError(err)
	s.Contains(progress.String(), "Pushing image to configured registry")
	s.Contains(progress.String(), "Successfully pushed Docker image")
	s.Empty(printed, "a deploy given a progress writer prints nothing on stdout")
	s.Equal("test-deployment-id", deployed.DeploymentID)
	s.Equal("https://deployments.local.astronomer.io/testDeploymentName/airflow", deployed.URL)
	s.True(strings.HasPrefix(deployed.Image, "registry.local.astronomer.io/"), "the image pushed to the Deployment's registry: %q", deployed.Image)
}

func (s *Suite) TestAirflowSuccessForBYORegistry() {
	config.InitConfig(s.fsForLocalConfig)

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		s.mockImageHandler.On("Push", mock.MatchedBy(func(remoteImage string) bool { return strings.Contains(remoteImage, "my.registry.domain") }), mock.Anything, mock.Anything, mock.Anything).Return("", nil).Once()
		s.mockImageHandler.On("GetLabel", "", airflow.RuntimeImageLabel).Return("4.2.5", nil)
		s.mockImageHandler.On("GetLabel", "", airflowImageLabel).Return("2.2.5", nil)
		return s.mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	mockRuntimeReleases := houston.RuntimeReleases{
		houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
		houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
	}
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Once()
	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{
		ClusterID: "test-cluster-id",
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
			{URL: "registry.local.astronomer.io", Type: "registry"},
		},
	}, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{
		Flags: houston.FeatureFlags{
			BYORegistryEnabled: true,
		},
		BYORegistryDomain: "my.registry.domain",
	}, nil).Once()
	vars := make(map[string]interface{})
	vars["clusterId"] = "test-cluster-id"
	s.houstonMock.On("GetRuntimeReleases", vars).Return(mockRuntimeReleases, nil)
	s.houstonMock.On("UpdateDeploymentImage", mock.Anything).Return(&houston.UpdateDeploymentImageResp{}, nil).Once()

	deployed, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, false, "", Options{})

	s.NoError(err)
	s.True(strings.HasPrefix(deployed.Image, "my.registry.domain:"), "the image pushed to the BYO registry: %q", deployed.Image)
}

func (s *Suite) TestAirflowFailureForNoBYORegistryDomain() {
	config.InitConfig(s.fsForLocalConfig)

	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{
		ClusterID: "test-cluster-id",
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
			{URL: "registry.local.astronomer.io", Type: "registry"},
		},
	}, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{
		Flags: houston.FeatureFlags{
			BYORegistryEnabled: true,
		},
	}, nil).Once()

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, false, "", Options{})

	s.ErrorIs(err, ErrBYORegistryDomainNotSet)
}

func (s *Suite) TestAirflowSuccessForImageOnly() {
	config.InitConfig(s.fsForLocalConfig)

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil).Once()
		return s.mockImageHandler
	}
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Once()

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	mockRuntimeReleases := houston.RuntimeReleases{
		houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
		houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
	}
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Once()
	dagDeployment := &houston.DagDeploymentConfig{
		Type: "dag-only",
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
		ClusterID:     "test-cluster-id",
		ID:            "test-deployment-id",
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
			{URL: "registry.local.astronomer.io", Type: "registry"},
		},
	}

	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(deployment, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Once()
	vars := make(map[string]interface{})
	vars["clusterId"] = "test-cluster-id"
	s.houstonMock.On("GetRuntimeReleases", vars).Return(mockRuntimeReleases, nil)

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, true, "", Options{})
	s.NoError(err)
}

func (s *Suite) TestAirflowSuccessForImageName() {
	config.InitConfig(s.fsForLocalConfig)
	customImageName := "test-image-name"
	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		s.mockImageHandler.On("TagLocalImage", customImageName).Return(nil)
		s.mockImageHandler.On("GetLabel", "", airflow.RuntimeImageLabel).Return("test", nil)
		return s.mockImageHandler
	}

	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	mockRuntimeReleases := houston.RuntimeReleases{
		houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
		houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
	}
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Once()
	dagDeployment := &houston.DagDeploymentConfig{
		Type: "dag-only",
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
		ClusterID:     "test-cluster-id",
		ID:            "test-deployment-id",
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
			{URL: "registry.local.astronomer.io", Type: "registry"},
		},
	}

	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(deployment, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Once()
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Once()
	vars := make(map[string]interface{})
	vars["clusterId"] = "test-cluster-id"
	s.houstonMock.On("GetRuntimeReleases", vars).Return(mockRuntimeReleases, nil)

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, true, customImageName, Options{})
	s.NoError(err)
}

func (s *Suite) TestAirflowFailForImageNameWhenImageHasNoRuntimeLabel() {
	config.InitConfig(s.fsForLocalConfig)
	customImageName := "test-image-name"
	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("TagLocalImage", customImageName).Return(nil)
		s.mockImageHandler.On("GetLabel", "", airflow.RuntimeImageLabel).Return("", nil)
		return s.mockImageHandler
	}
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	dagDeployment := &houston.DagDeploymentConfig{
		Type: "dag-only",
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
		ClusterID:     "test-cluster-id",
		ID:            "test-deployment-id",
	}

	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(deployment, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Once()

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, true, customImageName, Options{})
	s.Error(err, ErrNoRuntimeLabelOnCustomImage)
}

func (s *Suite) TestAirflowFailureForImageOnly() {
	config.InitConfig(s.fsForLocalConfig)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return s.mockImageHandler
	}
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil).Once()
	dagDeployment := &houston.DagDeploymentConfig{
		Type: "image",
	}
	deployment := &houston.Deployment{
		DagDeployment: *dagDeployment,
		ClusterID:     "test-cluster-id",
		ID:            "test-deployment-id",
	}

	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(deployment, nil).Once()
	s.houstonMock.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Once()

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", false, false, description, true, "", Options{})
	s.Error(err, ErrDeploymentTypeIncorrectForImageOnly)
}

func (s *Suite) TestDeployDagsOnlyFailure() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "test-deployment-id"
	wsID := "test-workspace-id"

	s.Run("When config flag is set to false on Houston before 2.0.0", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}

		appConfig := &houston.AppConfig{
			Version: "1.0.0",
			Flags:   *featureFlags,
		}
		deployment := &houston.Deployment{
			ClusterID: "test-cluster-id",
			ID:        deploymentID,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, ErrDagOnlyDeployDisabledInConfigLegacy)
	})

	s.Run("When config flag is set to false on Houston 2.0.0+", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}

		appConfig := &houston.AppConfig{
			Version: "2.0.0",
			Flags:   *featureFlags,
		}
		deployment := &houston.Deployment{
			ClusterID: "test-cluster-id",
			ID:        deploymentID,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, ErrDagOnlyDeployDisabledInConfig)
	})

	s.Run("When getDeploymentIDForCurrentCommandVar gives an error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, errDeploymentNotFound
		}
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, errDeploymentNotFound)
	})

	s.Run("When config flag is set to true but an error occurs in the GetDeployment api call", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(nil, errMockHouston).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorContains(err, "failed to get deployment info: some houston error")
	})

	s.Run("When config flag is set to true but it is disabled at the deployment level", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.VolumeDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
			ID:            deploymentID,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, ErrDagOnlyDeployNotEnabledForDeployment)
		// A volume Deployment gets its DAGs from the volume, not its image.
		s.False(errors.As(err, new(DagsInImageError)))
	})

	// An image Deployment's refusal says its DAGs come from its image, whether
	// the Deployment or the cluster refuses, and still reads as the refusal.
	for _, tc := range []struct {
		name    string
		enabled bool
		want    error
	}{
		{"at the deployment level", true, ErrDagOnlyDeployNotEnabledForDeployment},
		{"at the cluster level", false, ErrDagOnlyDeployDisabledInConfig},
	} {
		s.Run("When DAG-only deploy is refused "+tc.name+" for an image Deployment", func() {
			getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
				return deploymentID, nil, nil
			}
			appConfig := &houston.AppConfig{
				Version: "2.0.0",
				Flags:   houston.FeatureFlags{DagOnlyDeployment: tc.enabled},
			}
			deployment := &houston.Deployment{
				DagDeployment: houston.DagDeploymentConfig{Type: houston.ImageDeploymentType},
				ClusterID:     "test-cluster-id",
				ID:            deploymentID,
			}
			s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
			s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
			_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
			s.ErrorIs(err, tc.want)
			s.True(errors.As(err, new(DagsInImageError)))
			s.Equal(tc.want.Error(), err.Error())
		})
	}

	s.Run("Valid Houston config, but unable to get context from astro-cli config", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		config.ResetCurrentContext()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.EqualError(err, "could not get current context! Error: no context set, have you authenticated to Astro or APC? Run astro login and try again")
		context.Switch("localhost")
	})

	s.Run("Valid Houston config, able to get context from config but no release name present", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, errInvalidDeploymentID)
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is empty. User doesn't give operation confirmation", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// mock os.Stdin
		answer := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(answer)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "n")()

		// create the empty dags folder
		err = os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", nil, false, description, Options{})
		s.EqualError(err, ErrEmptyDagFolderUserCancelledOperation.Error())

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		s.True(os.IsNotExist(err))
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is empty. User gives the operation confirmation", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// mock os.Stdin
		answer := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(answer)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "y")()

		// create the empty dags folder
		err = os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			s.Equal(http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			s.NoError(err, "Error parsing multipart form")
			s.NotNil(r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", &server.URL, false, description, Options{})
		s.NoError(err)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was created
		destFilePath = "./dags.tar.gz"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading gZipped file")
		defer os.Remove(destFilePath)
	})

	// --yes answers the empty-folder question without asking; the Deployment
	// picked is the one returned; the upload's notes go to Progress.
	s.Run("Yes answers the empty-folder question, and the picked Deployment is returned", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return "picked-id", nil, nil
		}
		appConfig := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType},
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", "picked-id").Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		s.Require().NoError(os.Mkdir("dags", os.ModePerm))
		defer os.RemoveAll("dags")
		defer os.Remove("./dags.tar")
		defer os.Remove("./dags.tar.gz")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		defer server.Close()
		// Under a guard that refuses every question, so asking would fail.
		defer input.SetGuard(func() string { return "with --output json it cannot" })()

		progress := new(bytes.Buffer)
		got, err := DagsOnlyDeploy(s.houstonMock, wsID, "", ".", &server.URL, false, description, Options{Progress: progress, Yes: true})
		s.NoError(err)
		s.Equal("picked-id", got)
		s.Contains(progress.String(), "upload successful")
	})

	s.Run("without Yes, the empty-folder question refuses under a guard, naming --yes", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		appConfig := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType},
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		s.Require().NoError(os.Mkdir("dags", os.ModePerm))
		defer os.RemoveAll("dags")
		defer input.SetGuard(func() string { return "with --output json it cannot" })()

		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", nil, false, description, Options{})
		s.ErrorContains(err, "--yes")
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. Tar creation throws an error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// mock os.Stdin
		answer := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(answer)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "y")()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, "./dags", nil, false, description, Options{})
		s.EqualError(err, "open dags/dags.tar: no such file or directory")

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		s.True(os.IsNotExist(err))
		_, err = os.Stat("./dags.tar.gz")
		s.True(os.IsNotExist(err))
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. Tar is successfully created. But gzip creation throws an error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		gzipMockError := errors.New("some gzip error")

		// mock the gzip creation to throw an error
		gzipFile = func(srcFilePath, destFilePath string) error {
			return gzipMockError
		}

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", nil, false, description, Options{})
		s.ErrorIs(err, gzipMockError)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was not created
		destFilePath = "./dags.tar.gz"
		_, err = os.Stat(destFilePath)
		s.True(os.IsNotExist(err))

		gzipFile = fileutil.GzipFile
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. No need of User confirmation", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			s.Equal(http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			s.NoError(err, "Error parsing multipart form")
			s.NotNil(r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", &server.URL, false, description, Options{})
		s.NoError(err)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was created
		destFilePath = "./dags.tar.gz"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading gZipped file")
		defer os.Remove(destFilePath)
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. No need of User confirmation. Files should be auto-cleaned", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			s.Equal(http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			s.NoError(err, "Error parsing multipart form")
			s.NotNil(r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", &server.URL, true, description, Options{})
		s.NoError(err)

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		s.True(os.IsNotExist(err))
		_, err = os.Stat("./dags.tar.gz")
		s.True(os.IsNotExist(err))
	})
}

func (s *Suite) TestUpdateDeploymentImage() {
	deploymentID := "test-deployment-id"
	wsID := "test-workspace-id"
	runtimeVersion := "12.1.1"
	imageName := "imageName"
	releaseName := "releaseName"

	s.Run("When runtimeVersion is empty", func() {
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, "", imageName, Options{})
		s.ErrorIs(err, ErrRuntimeVersionNotPassedForRemoteImage)
		s.Equal(returnedDeploymentID, "")
	})

	s.Run("When getDeploymentIDForCurrentCommandVar gives an error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, errDeploymentNotFound
		}
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName, Options{})
		s.ErrorIs(err, errDeploymentNotFound)
		s.Equal(returnedDeploymentID, "")
	})

	s.Run("When an error occurs in the GetDeployment api call", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(nil, errMockHouston).Once()

		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName, Options{})
		s.ErrorContains(err, "failed to get deployment info: some houston error")
		s.Equal(returnedDeploymentID, "")
	})

	s.Run("Houston API call throws error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		deployment := &houston.Deployment{
			ReleaseName: releaseName,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("UpdateDeploymentImage", mock.Anything).Return(nil, errMockHouston).Once()
		var returnedDeploymentID string
		var err error
		printed := stdoutOf(s, func() {
			returnedDeploymentID, err = UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName, Options{})
		})
		s.ErrorContains(err, "some houston error")
		s.Equal(returnedDeploymentID, deploymentID)
		s.NotContains(printed, "Image successfully updated", "a failed update is not reported as a success")
	})

	s.Run("Successful API call", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		updateDeploymentImageResp := &houston.UpdateDeploymentImageResp{
			ReleaseName:    releaseName,
			RuntimeVersion: runtimeVersion,
		}
		deployment := &houston.Deployment{
			ReleaseName: releaseName,
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		s.houstonMock.On("UpdateDeploymentImage", mock.Anything).Return(updateDeploymentImageResp, nil).Once()
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName, Options{})
		s.ErrorIs(err, nil)
		s.Equal(returnedDeploymentID, deploymentID)
	})
}

// stdoutOf runs fn and returns what it printed on os.Stdout.
func stdoutOf(s *Suite, fn func()) string {
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	prev := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = prev
	s.Require().NoError(w.Close())
	return <-done
}
