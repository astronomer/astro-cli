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
	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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

func (s *Suite) TestValidAirflowImageRepo() {
	s.True(validAirflowImageRepo("quay.io/astronomer/ap-airflow"))
	s.True(validAirflowImageRepo("astronomerinc/ap-airflow"))
	s.False(validAirflowImageRepo("personal-repo/ap-airflow"))
}

func (s *Suite) TestValidRuntimeImageRepo() {
	s.False(validRuntimeImageRepo("quay.io/astronomer/ap-airflow"))
	s.True(validRuntimeImageRepo("quay.io/astronomer/astro-runtime"))
	s.False(validRuntimeImageRepo("personal-repo/ap-airflow"))
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
	s.houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)

	err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "")
	s.NoError(err)
}

func (s *Suite) TestBuildPushDockerImageSuccessWithImageRepoWarning() {
	config.InitConfig(s.fsForDockerConfig)
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
	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil)
	s.houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)

	err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "")
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
	s.houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io:test-test", AirflowVersion: "1.10.12", RuntimeVersion: ""}).Return(nil, nil)

	err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true, description, "")
	s.NoError(err)

	expectedLabel := deployRevisionDescriptionLabel + "=" + description
	assert.Contains(s.T(), capturedBuildConfig.Labels, expectedLabel)
	s.mockImageHandler.AssertExpectations(s.T())
	s.houstonMock.AssertExpectations(s.T())

	// Case when SHA is used as tag
	s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io@image_sha", AirflowVersion: "1.10.12", RuntimeVersion: ""}).Return(nil, nil)
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
	err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true, description, "")
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
	s.houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "test", Image: "test.registry.io:latest", AirflowVersion: "1.10.12", RuntimeVersion: "12.2.0"}).Return(nil, nil)

	err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "test.registry.io", false, true, description, customImageName)
	s.NoError(err)
}

func (s *Suite) TestBuildPushDockerImageFailure() {
	// invalid dockerfile test
	dockerfile = "Dockerfile.invalid"
	err := buildPushDockerImage(nil, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "")
	s.EqualError(err, "failed to parse dockerfile: testfiles/Dockerfile.invalid: when using JSON array syntax, arrays must be comprised of strings only")
	dockerfile = "Dockerfile"

	config.InitConfig(s.fsForDockerConfig)
	mockedDeploymentConfig := &houston.DeploymentConfig{
		AirflowImages: mockAirflowImageList,
	}
	s.houstonMock.On("GetDeploymentConfig", nil).Return(nil, errMockHouston).Once()
	s.houstonMock.On("GetRuntimeReleases", "").Return(houston.RuntimeReleases{}, nil)
	// houston GetDeploymentConfig call failure
	err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "")
	s.Error(err, errMockHouston)

	s.houstonMock.On("GetDeploymentConfig", nil).Return(mockedDeploymentConfig, nil).Twice()

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return s.mockImageHandler
	}

	// build error test case
	err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "")
	s.Error(err, errSomeContainerIssue.Error())
	s.mockImageHandler.AssertExpectations(s.T())

	s.mockImageHandler = new(mocks.ImageHandler)
	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", errSomeContainerIssue)
		return s.mockImageHandler
	}

	// push error test case
	err = buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "test", "./testfiles/", "test", "test", "", false, false, description, "")
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

func (s *Suite) TestGetAirflowUILinkFailure() {
	actualResult := getAirflowUILink("", []houston.DeploymentURL{})
	s.Equal(actualResult, "")

	config.InitConfig(s.fsForLocalConfig)

	actualResult = getAirflowUILink("testDeploymentID", []houston.DeploymentURL{})
	s.Equal(actualResult, "")
}

func (s *Suite) TestAirflowFailure() {
	// No workspace ID test case
	_, err := Airflow(nil, "", "", "", "", false, false, false, description, false, "")
	s.ErrorIs(err, ErrNoWorkspaceID)

	// houston GetWorkspace failure case
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(nil, errMockHouston).Once()

	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", "", false, false, false, description, false, "")
	s.ErrorIs(err, errMockHouston)
	s.houstonMock.AssertExpectations(s.T())

	// houston ListDeployments failure case
	s.houstonMock.On("GetWorkspace", mock.Anything).Return(&houston.Workspace{}, nil)
	s.houstonMock.On("ListDeployments", mock.Anything).Return(nil, errMockHouston).Once()

	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", "", false, false, false, description, false, "")
	s.ErrorIs(err, errMockHouston)
	s.houstonMock.AssertExpectations(s.T())

	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil).Times(3)

	config.InitConfig(s.fsForLocalConfig)

	// config GetCurrentContext failure case
	config.ResetCurrentContext()

	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", "", false, false, false, description, false, "")
	s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	context.Switch("localhost")

	// Invalid deployment name case
	_, err = Airflow(s.houstonMock, "", "test-deployment-id", "test-workspace-id", "", false, false, false, description, false, "")
	s.ErrorIs(err, errInvalidDeploymentID)

	// No deployment in the current workspace case
	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", "", false, false, false, description, false, "")
	s.ErrorIs(err, errDeploymentNotFound)
	s.houstonMock.AssertExpectations(s.T())

	// Invalid deployment selection case
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	_, err = Airflow(s.houstonMock, "", "", "test-workspace-id", "", false, false, false, description, false, "")
	s.ErrorIs(err, errInvalidDeploymentSelected)

	// return error When houston get deployment throws an error
	s.houstonMock.On("ListDeployments", mock.Anything).Return([]houston.Deployment{{ID: "test-deployment-id"}}, nil)
	s.houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()
	_, err = Airflow(s.houstonMock, "", "test-deployment-id", "test-workspace-id", "", false, false, false, description, false, "")
	s.Equal(err.Error(), "failed to get deployment info: "+errMockHouston.Error())

	// buildPushDockerImage failure case
	s.houstonMock.On("GetDeployment", "test-deployment-id").Return(&houston.Deployment{}, nil)
	dockerfile = "Dockerfile.invalid"
	_, err = Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false, description, false, "")
	dockerfile = "Dockerfile"
	s.Error(err)
	s.Contains(err.Error(), "failed to parse dockerfile")
}

func (s *Suite) TestAirflowSuccess() {
	config.InitConfig(s.fsForLocalConfig)

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
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
	s.houstonMock.On("GetDeployment", mock.Anything).Return(&houston.Deployment{}, nil).Once()
	s.houstonMock.On("GetRuntimeReleases", "").Return(mockRuntimeReleases, nil)

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false, description, false, "")
	s.NoError(err)
}

func (s *Suite) TestAirflowSuccessForImageOnly() {
	config.InitConfig(s.fsForLocalConfig)

	imageHandlerInit = func(image string) airflow.ImageHandler {
		s.mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
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
	}

	s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
	s.houstonMock.On("GetRuntimeReleases", "").Return(mockRuntimeReleases, nil)

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false, description, true, "")
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
	}

	s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
	s.houstonMock.On("GetRuntimeReleases", "").Return(mockRuntimeReleases, nil)

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false, description, true, customImageName)
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
	}

	s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false, description, true, customImageName)
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
	}

	s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

	_, err := Airflow(s.houstonMock, "./testfiles/", "test-deployment-id", "test-workspace-id", "", false, false, false, description, true, "")
	s.Error(err, ErrDeploymentTypeIncorrectForImageOnly)
}

func (s *Suite) TestDeployDagsOnlyFailure() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "test-deployment-id"
	wsID := "test-workspace-id"

	s.Run("When config flag is set to false", func() {
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}

		err := DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false, description)
		s.ErrorIs(err, ErrDagOnlyDeployDisabledInConfig)
	})

	s.Run("When getDeploymentIDForCurrentCommandVar gives an error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, errDeploymentNotFound
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}

		err := DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false, description)
		s.ErrorIs(err, errDeploymentNotFound)
	})

	s.Run("When config flag is set to true but an error occurs in the GetDeployment api call", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()

		err := DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false, description)
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		err := DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false, description)
		s.ErrorIs(err, ErrDagOnlyDeployNotEnabledForDeployment)
	})

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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		config.ResetCurrentContext()

		err := DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false, description)
		s.EqualError(err, "could not get current context! Error: no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		err := DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, config.WorkingPath, nil, false, description)
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
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

		err = DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, ".", nil, false, description)
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
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

		err = DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, ".", &server.URL, false, description)
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		defer testUtil.MockUserInput(s.T(), "y")()

		err = DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, "./dags", nil, false, description)
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
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		gzipMockError := errors.New("some gzip error") //nolint

		// mock the gzip creation to throw an error
		gzipFile = func(srcFilePath, destFilePath string) error {
			return gzipMockError
		}

		err = DagsOnlyDeploy(s.houstonMock, appConfig, deploymentID, wsID, ".", nil, false, description)
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

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

		err = DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, ".", &server.URL, false, description)
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
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()

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

		err = DagsOnlyDeploy(s.houstonMock, appConfig, wsID, deploymentID, ".", &server.URL, true, description)
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
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, "", imageName)
		s.ErrorIs(err, ErrRuntimeVersionNotPassedForRemoteImage)
		s.Equal(returnedDeploymentID, "")
	})

	s.Run("When getDeploymentIDForCurrentCommandVar gives an error", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, errDeploymentNotFound
		}
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName)
		s.ErrorIs(err, errDeploymentNotFound)
		s.Equal(returnedDeploymentID, "")
	})

	s.Run("When an error occurs in the GetDeployment api call", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(nil, errMockHouston).Once()

		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName)
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
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		s.houstonMock.On("UpdateDeploymentImage", mock.Anything).Return(nil, errMockHouston).Once()
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName)
		s.ErrorContains(err, "some houston error")
		s.Equal(returnedDeploymentID, deploymentID)
	})

	s.Run("Successful API call", func() {
		getDeploymentIDForCurrentCommandVar = func(houstonClient houston.ClientInterface, wsID, deploymentID string, prompt bool) (string, []houston.Deployment, error) {
			return deploymentID, nil, nil
		}
		updateDeploymentImageResp := &houston.UpdateDeploymentImageResp{
			ReleaseName:    releaseName,
			AirflowVersion: "",
			RuntimeVersion: runtimeVersion,
		}
		deployment := &houston.Deployment{
			ReleaseName: releaseName,
		}
		s.houstonMock.On("GetDeployment", mock.Anything).Return(deployment, nil).Once()
		s.houstonMock.On("UpdateDeploymentImage", mock.Anything).Return(updateDeploymentImageResp, nil).Once()
		returnedDeploymentID, err := UpdateDeploymentImage(s.houstonMock, deploymentID, wsID, runtimeVersion, imageName)
		s.ErrorIs(err, nil)
		s.Equal(returnedDeploymentID, deploymentID)
	})
}
