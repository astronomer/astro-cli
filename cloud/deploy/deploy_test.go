package deploy

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var (
	errMock                  = errors.New("mock error")
	org                      = "test-org-id"
	ws                       = "test-ws-id"
	initiatedDagDeploymentID = "test-dag-deployment-id"
	runtimeID                = "test-id"
	dagURL                   = "http://fake-url.windows.core.net"
)

type Suite struct {
	suite.Suite
}

func TestCloudDeploySuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestDeployWithoutDagsDeploySuccess() {
	mockDeplyResp := astro.Deployment{
		ID:             "test-id",
		ReleaseName:    "test-name",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		Workspace:      astro.Workspace{ID: ws},
		DeploymentSpec: astro.DeploymentSpec{
			Webserver: astro.Webserver{URL: "test-url"},
		},
		CreatedAt:        time.Now(),
		DagDeployEnabled: false,
	}
	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           false,
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("GetDeployment", mock.Anything).Return(mockDeplyResp, nil).Times(3)
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", Workspace: astro.Workspace{ID: ws}}}, nil).Once()
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Times(4)
	mockClient.On("CreateImage", mock.Anything).Return(&astro.Image{}, nil).Times(4)
	mockClient.On("DeployImage", mock.Anything).Return(&astro.Image{}, nil).Times(4)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	ctx, err := config.GetCurrentContext()
	s.NoError(err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	s.NoError(err)

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
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.RuntimeID = "test-id"
	deployInput.Pytest = "pytest"
	deployInput.Prompt = false
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	// test custom image
	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.ImageName = "custom-image"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.DeploymentName = "test-name"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	mockClient.AssertExpectations(s.T())
	mockImageHandler.AssertExpectations(s.T())
	mockContainerHandler.AssertExpectations(s.T())
}

func (s *Suite) TestDeployWithDagsDeploySuccess() {
	os.Mkdir("./testfiles/dags", os.ModePerm)
	path := "./testfiles/dags/test.py"
	fileutil.WriteStringToFile(path, "testing")

	mockDeplyResp := astro.Deployment{
		ID:             "test-id",
		ReleaseName:    "test-name",
		Workspace:      astro.Workspace{ID: ws},
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{
			Webserver: astro.Webserver{URL: "test-url"},
		},
		CreatedAt:        time.Now(),
		DagDeployEnabled: true,
	}
	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           false,
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("GetDeployment", mock.Anything).Return(mockDeplyResp, nil).Times(5)
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", Workspace: astro.Workspace{ID: ws}, DagDeployEnabled: true}}, nil).Times(2)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Times(7)
	mockClient.On("CreateImage", mock.Anything).Return(&astro.Image{}, nil).Times(7)
	mockClient.On("DeployImage", mock.Anything).Return(&astro.Image{}, nil).Times(7)
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Times(6)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		RuntimeID:                runtimeID,
		Action:                   "UPLOAD",
		VersionID:                "version-id",
		Status:                   "SUCCEEDED",
		Message:                  "DAGs uploaded successfully",
	}
	mockClient.On("ReportDagDeploymentStatus", reportDagDeploymentStatusInput).Return(astro.DagDeploymentStatus{}, nil).Times(6)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	ctx, err := config.GetCurrentContext()
	s.NoError(err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	s.NoError(err)

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
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.RuntimeID = "test-id"
	deployInput.Pytest = "pytest"
	deployInput.Prompt = false
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.DeploymentName = "test-name"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = parseAndPytest
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	// test custom image with dag deploy enabled
	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.ImageName = "custom-image"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	os.Mkdir("./testfiles1/", os.ModePerm)
	fileutil.WriteStringToFile("./testfiles1/Dockerfile", "FROM quay.io/astronomer/astro-runtime:4.2.5")
	fileutil.WriteStringToFile("./testfiles1/.env", "")
	fileutil.WriteStringToFile("./testfiles1/.dockerignore", "")

	deployInput = InputDeploy{
		Path:           "./testfiles1/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           false,
	}
	defer testUtil.MockUserInput(s.T(), "y")()
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer os.RemoveAll("./testfiles1/")
	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(s.T())
	mockImageHandler.AssertExpectations(s.T())
	mockContainerHandler.AssertExpectations(s.T())
}

func (s *Suite) TestDagsDeploySuccess() {
	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt:        time.Now(),
			DagDeployEnabled: true,
		},
		{
			ID:             "test-id-2",
			ReleaseName:    "test-name-2",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt:        time.Now(),
			DagDeployEnabled: true,
		},
	}

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "test-id",
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           true,
		DagsPath:       "./testfiles/dags",
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Times(3)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockDeplyResp, nil).Times(4)
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Times(4)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		RuntimeID:                runtimeID,
		Action:                   "UPLOAD",
		VersionID:                "version-id",
		Status:                   "SUCCEEDED",
		Message:                  "DAGs uploaded successfully",
	}
	mockClient.On("ReportDagDeploymentStatus", reportDagDeploymentStatusInput).Return(astro.DagDeploymentStatus{}, nil).Times(4)

	defer testUtil.MockUserInput(s.T(), "y")()
	err := Deploy(deployInput, mockClient)
	s.NoError(err)

	// Test pytest with dags deploy
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = allTests
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = parseAndPytest
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(s.T())
}

func (s *Suite) TestNoDagsDeploy() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("true")
	mockClient := new(astro_mocks.Client)

	ctx, err := config.GetCurrentContext()
	s.NoError(err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	s.NoError(err)

	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt:        time.Now(),
			DagDeployEnabled: true,
		},
		{
			ID:             "test-id-2",
			ReleaseName:    "test-name-2",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt:        time.Now(),
			DagDeployEnabled: true,
		},
	}

	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockDeplyResp, nil).Times(1)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "test-id",
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           true,
	}
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	mockClient.AssertExpectations(s.T())
}

func (s *Suite) TestDagsDeployFailed() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt:        time.Now(),
			DagDeployEnabled: false,
		},
		{
			ID:             "test-id-2",
			ReleaseName:    "test-name-2",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt:        time.Now(),
			DagDeployEnabled: true,
		},
	}

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "test-id",
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           true,
	}
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockDeplyResp, nil).Times(3)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Times(2)

	defer testUtil.MockUserInput(s.T(), "y")()
	err := Deploy(deployInput, mockClient)
	s.Equal(err.Error(), "DAG-only deploys are not enabled for this Deployment. Run 'astro deployment update test-id --dag-deploy enable' to enable DAG-only deploys.")

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(errMock)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything).Return("", errMock)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockClient)
	s.Error(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.Pytest = allTests
	err = Deploy(deployInput, mockClient)
	s.Error(err)

	mockClient.AssertExpectations(s.T())
}

func (s *Suite) TestDeployFailure() {
	os.Mkdir("./testfiles/dags", os.ModePerm)
	path := "./testfiles/dags/test.py"
	fileutil.WriteStringToFile(path, "testing")

	defer os.RemoveAll("./testfiles/dags/")

	// no context set failure
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	err := config.ResetCurrentContext()
	s.NoError(err)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "test-id",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           false,
	}

	defer testUtil.MockUserInput(s.T(), "y")()
	err = Deploy(deployInput, nil)
	s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	// airflow parse failure
	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			Workspace:      astro.Workspace{ID: ws},
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
		},
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", org, ws).Return(mockDeplyResp, nil).Times(2)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(errMock)
		return mockContainerHandler, nil
	}

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
	deployInput.RuntimeID = ""
	err = Deploy(deployInput, mockClient)
	s.ErrorIs(err, errDagsParseFailed)

	mockClient.On("ListDeployments", org, "invalid-workspace").Return(mockDeplyResp, nil).Once()
	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.RuntimeID = "test-id"
	deployInput.WsID = "invalid-workspace"
	err = Deploy(deployInput, mockClient)
	s.NoError(err)

	defer testUtil.MockUserInput(s.T(), "y")()
	deployInput.WsID = ws
	deployInput.EnvFile = "invalid-path"
	err = Deploy(deployInput, mockClient)
	s.ErrorIs(err, envFileMissing)

	mockClient.AssertExpectations(s.T())
	mockImageHandler.AssertExpectations(s.T())
	mockContainerHandler.AssertExpectations(s.T())
}

func (s *Suite) TestBuildImageFailure() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockImageHandler := new(mocks.ImageHandler)

	// image build failure
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything).Return(errMock).Once()
		return mockImageHandler
	}
	_, err := buildImage("./testfiles/", "4.2.5", "", "", false, nil)
	s.ErrorIs(err, errMock)

	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	// dockerfile parsing error
	dockerfile = "Dockerfile.invalid"
	_, err = buildImage("./testfiles/", "4.2.5", "", "", false, nil)
	s.Error(err)
	s.Contains(err.Error(), "failed to parse dockerfile")

	// failed to get runtime releases
	dockerfile = "Dockerfile"
	mockClient := new(astro_mocks.Client)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()
	_, err = buildImage("./testfiles/", "4.2.5", "", "", false, mockClient)
	s.ErrorIs(err, errMock)
	mockClient.AssertExpectations(s.T())
	mockImageHandler.AssertExpectations(s.T())
}

func (s *Suite) TestIsValidUpgrade() {
	resp := IsValidUpgrade("4.2.5", "4.2.6")
	s.True(resp)

	resp = IsValidUpgrade("4.2.6", "4.2.5")
	s.False(resp)

	resp = IsValidUpgrade("", "4.2.6")
	s.True(resp)
}

func (s *Suite) TestIsValidTag() {
	resp := IsValidTag([]string{"4.2.5", "4.2.6"}, "4.2.7")
	s.False(resp)

	resp = IsValidTag([]string{"4.2.5", "4.2.6"}, "4.2.6")
	s.True(resp)
}

func (s *Suite) TestCheckVersion() {
	httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), false)
	latestRuntimeVersion, _ := airflowversions.GetDefaultImageTag(httpClient, "")

	// version that is older than newest
	buf := new(bytes.Buffer)
	CheckVersion("1.0.0", buf)
	s.Contains(buf.String(), "WARNING! You are currently running Astro Runtime Version")

	// version that is latest
	CheckVersion(latestRuntimeVersion, buf)
	s.Contains(buf.String(), "Runtime Version: "+latestRuntimeVersion)
}

func (s *Suite) TestCheckVersionBeta() {
	// version that newer than latest
	buf := new(bytes.Buffer)
	defer testUtil.MockUserInput(s.T(), "y")()
	CheckVersion("10.0.0", buf)
	s.Contains(buf.String(), "")
}

func (s *Suite) TestCheckPyTest() {
	mockDeployImage := "test-image"

	mockContainerHandler := new(mocks.ContainerHandler)
	mockContainerHandler.On("Pytest", []string{""}, "", mockDeployImage).Return("", errMock).Once()

	// random error on running airflow pytest
	err := checkPytest("", mockDeployImage, mockContainerHandler)
	s.ErrorIs(err, errMock)
	mockContainerHandler.AssertExpectations(s.T())

	// airflow pytest exited with status code 1
	mockContainerHandler.On("Pytest", []string{""}, "", mockDeployImage).Return("exit code 1", errMock).Once()
	err = checkPytest("", mockDeployImage, mockContainerHandler)
	s.Error(err)
	s.Contains(err.Error(), "at least 1 pytest in your tests directory failed. Fix the issues listed or rerun the command without the '--pytest' flag to deploy")
	mockContainerHandler.AssertExpectations(s.T())
}
