package deploy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock                    = errors.New("mock error")
	ws                         = "test-ws-id"
	dagTarballVersionTest      = "test-version"
	dagsUploadTestURL          = "test-url"
	deploymentID               = "test-deployment-id"
	tarballVersion             = "test-version"
	hybridType                 = astroplatformcore.DeploymentTypeHYBRID
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:     deploymentID,
			Name:   "test-deployment",
			Status: "HEALTHY",
			Type:   &hybridType,
		},
	}
	mockCoreDeploymentResponseCICD = []astroplatformcore.Deployment{
		{
			Id:             deploymentID,
			Status:         "HEALTHY",
			IsCicdEnforced: true,
			Type:           &hybridType,
		},
	}
	mockListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	mockListDeploymentsResponseCICD = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponseCICD,
		},
	}
	createDeployResponse = astroplatformcore.CreateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deploy{
			Id:                "test-id",
			DagTarballVersion: &dagTarballVersionTest,
			ImageRepository:   "test-repository",
			DagsUploadUrl:     &dagsUploadTestURL,
		},
	}
	finalizeDeployResponse = astroplatformcore.FinalizeDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deploy{
			Id:                "test-id",
			DagTarballVersion: &dagTarballVersionTest,
			ImageTag:          "test-tag",
		},
	}
	getDeploymentOptionsResponse = astroplatformcore.GetDeploymentOptionsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentOptions{
			RuntimeReleases: []astroplatformcore.RuntimeRelease{
				{Version: "12.0.0"},
				{Version: "4.2.6"},
				{Version: "4.2.5"},
				{Version: "3.1-1"},
				{Version: "3.0-3"},
				{Version: "3.0-1"},
			},
		},
	}
	deploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                 deploymentID,
			RuntimeVersion:     "12.0.0",
			Namespace:          "test-name",
			WorkspaceId:        ws,
			WebServerUrl:       "test-url",
			IsDagDeployEnabled: false,
			Type:               &hybridType,
		},
	}
	deploymentResponseCICD = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                 deploymentID,
			RuntimeVersion:     "12.0.0",
			Namespace:          "test-name",
			WorkspaceId:        ws,
			WebServerUrl:       "test-url",
			IsDagDeployEnabled: false,
			IsCicdEnforced:     true,
			Type:               &hybridType,
			Name:               "test-deployment",
		},
	}
	deploymentResponseRemoteExecution = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                 deploymentID,
			RuntimeVersion:     "3.0-1",
			Namespace:          "test-name",
			WorkspaceId:        ws,
			WebServerUrl:       "test-url",
			IsDagDeployEnabled: false,
			Type:               &hybridType,
			RemoteExecution: &astroplatformcore.DeploymentRemoteExecution{
				Enabled: true,
			},
		},
	}
	deploymentResponseDags = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                       deploymentID,
			RuntimeVersion:           "12.0.0",
			Namespace:                "test-name",
			WorkspaceId:              ws,
			WebServerUrl:             "test-url",
			IsDagDeployEnabled:       true,
			IsCicdEnforced:           false,
			Type:                     &hybridType,
			DesiredDagTarballVersion: &tarballVersion,
		},
	}
)

func TestDeployWithoutDagsDeploySuccess(t *testing.T) {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           false,
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(5)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(5)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(5)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	assert.NoError(t, err)

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

	defer testUtil.MockUserInput(t, "1")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.RuntimeID = "test-id"
	deployInput.Pytest = "pytest"
	deployInput.Prompt = false
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// test custom image
	defer testUtil.MockUserInput(t, "y")()
	deployInput.ImageName = "custom-image"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(t, "y")()
	deployInput.DeploymentName = "test-name"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = ""
	deployInput.WaitForStatus = true
	sleepTime = 1
	timeoutNum = 1
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.ErrorIs(t, err, deployment.ErrTimedOut)

	mockCoreClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeployOnRemoteExecutionDeployment(t *testing.T) {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           false,
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseRemoteExecution, nil).Times(6)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(5)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(5)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(5)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("3.0-1", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	assert.NoError(t, err)

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

	defer testUtil.MockUserInput(t, "1")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.RuntimeID = "test-id"
	deployInput.Pytest = "pytest"
	deployInput.Prompt = false
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// test custom image
	defer testUtil.MockUserInput(t, "y")()
	deployInput.ImageName = "custom-image"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(t, "y")()
	deployInput.DeploymentName = "test-name"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = ""
	deployInput.WaitForStatus = true
	sleepTime = 1
	timeoutNum = 1
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.ErrorIs(t, err, deployment.ErrTimedOut)

	mockCoreClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeployOnCiCdEnforcedDeployment(t *testing.T) {
	os.Mkdir("./testfiles/dags", os.ModePerm)
	path := "./testfiles/dags/test.py"
	fileutil.WriteStringToFile(path, "testing")
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           false,
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	canCiCdDeploy = func(astroAPIToken string) bool {
		return false
	}

	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseCICD, nil).Twice()

	defer testUtil.MockUserInput(t, "1")()
	err := Deploy(deployInput, mockPlatformCoreClient, nil)
	assert.Contains(t, err.Error(), "cannot deploy since ci/cd enforcement is enabled for the deployment test-deployment. Please use API Tokens instead")

	defer os.RemoveAll("./testfiles/dags/")

	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeployWithDagsDeploySuccess(t *testing.T) {
	os.Mkdir("./testfiles/dags", os.ModePerm)
	path := "./testfiles/dags/test.py"
	fileutil.WriteStringToFile(path, "testing")
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           false,
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")

	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(9)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(7)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(7)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(7)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	assert.NoError(t, err)

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

	defer testUtil.MockUserInput(t, "1")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.RuntimeID = "test-id"
	deployInput.Pytest = "pytest"
	deployInput.Prompt = false
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(t, "y")()
	deployInput.DeploymentName = "test-name"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = parseAndPytest
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// test custom image with dag deploy enabled
	defer testUtil.MockUserInput(t, "y")()
	deployInput.ImageName = "custom-image"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

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
		WaitForStatus:  false,
		Dags:           false,
	}
	defer testUtil.MockUserInput(t, "1")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles1/")
	defer os.RemoveAll("./testfiles/dags/")

	mockCoreClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDagsDeploySuccess(t *testing.T) {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           true,
		WaitForStatus:  false,
		DagsPath:       "./testfiles/dags",
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(12)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(4)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(6)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(6)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	defer testUtil.MockUserInput(t, "1")()
	err := Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// Test pytest with dags deploy
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(t, "1")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "1")()
	deployInput.Pytest = allTests
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "1")()
	deployInput.Pytest = parseAndPytest
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)
	// image deploy
	defer testUtil.MockUserInput(t, "1")()
	deployInput.Image = true

	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "1")()
	deployInput.Pytest = ""
	deployInput.WaitForStatus = true
	dagOnlyDeploySleepTime = 1
	timeoutNum = 1
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.ErrorIs(t, err, deployment.ErrTimedOut)

	defer os.RemoveAll("./testfiles/dags/")

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestImageOnlyDeploySuccess(t *testing.T) {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		Dags:           false,
		Image:          true,
		WaitForStatus:  false,
		DagsPath:       "./testfiles/dags",
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(2)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(1)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(1)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(1)

	// Test pytest with dags deploy
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(t, "1")()
	err := Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles/dags/")

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestNoDagsDeploy(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("true")
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.Token = "test testing"
	err = ctx.SetContext()
	assert.NoError(t, err)

	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(2)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(1)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      "",
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           true,
	}
	defer testUtil.MockUserInput(t, "1")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDagsDeployFailed(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      deploymentID,
		WsID:           ws,
		Pytest:         "",
		EnvFile:        "",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           true,
	}
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(2)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(3)

	defer testUtil.MockUserInput(t, "y")()
	err := Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.Equal(t, err.Error(), "DAG-only deploys are not enabled for this Deployment. Run 'astro deployment update test-deployment-id --dag-deploy enable' to enable DAG-only deploys")

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", errMock)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.Error(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = allTests
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.Error(t, err)

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeployFailure(t *testing.T) {
	os.Mkdir("./testfiles/dags", os.ModePerm)
	path := "./testfiles/dags/test.py"
	fileutil.WriteStringToFile(path, "testing")

	defer os.RemoveAll("./testfiles/dags/")

	// no context set failure
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := config.ResetCurrentContext()
	assert.NoError(t, err)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      deploymentID,
		WsID:           ws,
		Pytest:         "parse",
		EnvFile:        "./testfiles/.env",
		ImageName:      "",
		DeploymentName: "",
		Prompt:         true,
		WaitForStatus:  false,
		Dags:           false,
	}

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astro Private Cloud? Run astro login and try again")

	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(1)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(2)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockContainerHandler, nil
	}

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

	defer testUtil.MockUserInput(t, "1")()
	deployInput.RuntimeID = ""
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.ErrorIs(t, err, errDagsParseFailed)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.RuntimeID = deploymentID
	deployInput.WsID = "invalid-workspace"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.WsID = ws
	deployInput.EnvFile = "invalid-path"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.ErrorIs(t, err, envFileMissing)

	mockCoreClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)

	mockContainerHandler.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeployMonitoringDAGNonHosted(t *testing.T) {
	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      deploymentID,
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
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.OrganizationProduct = "HYBRID"
	err = ctx.SetContext()
	assert.NoError(t, err)

	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(3)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(8)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(4)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(4)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		_, err = os.Stat("./testfiles/dags/astronomer_monitoring_dag.py")
		assert.NoError(t, err)
		return "version-id", nil
	}

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// Test pytest with dags deploy
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = allTests
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = parseAndPytest
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles/dags/")

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeployNoMonitoringDAGHosted(t *testing.T) {
	dagsDir := "./testfiles/dags"
	err := os.MkdirAll(dagsDir, os.ModePerm)
	assert.NoError(t, err)
	defer os.RemoveAll(dagsDir)

	deployInput := InputDeploy{
		Path:           "./testfiles/",
		RuntimeID:      deploymentID,
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
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.OrganizationProduct = "HOSTED"
	err = ctx.SetContext()
	assert.NoError(t, err)

	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(3)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(8)
	mockPlatformCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(4)
	mockPlatformCoreClient.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&finalizeDeployResponse, nil).Times(4)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		_, err = os.Stat("./testfiles/dags/astronomer_monitoring_dag.py")
		assert.ErrorIs(t, err, os.ErrNotExist)
		return "version-id", nil
	}

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// Test pytest with dags deploy
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return("", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
		return mockContainerHandler, nil
	}

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = "parse"
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = allTests
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = parseAndPytest
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles/dags/")

	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestBuildImageFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockImageHandler := new(mocks.ImageHandler)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	// image build failure
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(errMock).Once()
		return mockImageHandler
	}
	_, err := buildImage("./testfiles/", "4.2.5", "", "", "", "", false, false, false, mockPlatformCoreClient)
	assert.ErrorIs(t, err, errMock)

	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("12.0.0", nil)
		return mockImageHandler
	}

	// dockerfile parsing error
	dockerfile = "Dockerfile.invalid"
	_, err = buildImage("./testfiles/", "4.2.5", "", "", "", "", false, false, false, mockPlatformCoreClient)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse dockerfile")

	// failed to get runtime releases
	dockerfile = "Dockerfile"
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, errMock).Once()
	_, err = buildImage("./testfiles/", "4.2.5", "", "", "", "", false, false, false, mockPlatformCoreClient)
	assert.ErrorIs(t, err, errMock)
	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
}

func TestValidRuntimeVersion(t *testing.T) {
	testCases := []struct {
		name              string
		currentVersion    string
		newVersion        string
		deploymentOptions []string
		forceUpgradeToAF3 bool
		expected          bool
		expectedError     string
	}{
		// Empty current version cases
		{
			name:              "empty current version allows any upgrade",
			currentVersion:    "",
			newVersion:        "4.2.6",
			deploymentOptions: []string{"4.2.6"},
			expected:          true,
		},

		// AF2 version upgrade cases
		{
			name:              "newer AF2 version is valid upgrade",
			currentVersion:    "4.2.5",
			newVersion:        "4.2.6",
			deploymentOptions: []string{"4.2.6"},
			expected:          true,
		},
		{
			name:              "older AF2 version is invalid upgrade",
			currentVersion:    "4.2.6",
			newVersion:        "4.2.5",
			deploymentOptions: []string{"4.2.5"},
			expected:          false,
			expectedError:     "Cannot deploy a downgraded Astro Runtime version",
		},

		// AF2 to AF3 upgrade cases
		{
			name:              "AF2 >= 12.0.0 to AF3 version with force flag is valid upgrade",
			currentVersion:    "12.0.0",
			newVersion:        "3.0-1",
			deploymentOptions: []string{"3.0-1"},
			forceUpgradeToAF3: true,
			expected:          true,
		},
		{
			name:              "AF2 < 12.0.0 to AF3 version with force flag is invalid upgrade",
			currentVersion:    "4.2.5",
			newVersion:        "3.0-1",
			deploymentOptions: []string{"3.0-1"},
			forceUpgradeToAF3: true,
			expected:          false,
			expectedError:     "Can only upgrade deployment from Airflow 2 to Airflow 3 with deployment at Astro Runtime 12.0.0 or higher",
		},
		{
			name:              "AF2 >= 12.0.0 to AF3 version without force flag is invalid upgrade",
			currentVersion:    "12.0.0",
			newVersion:        "3.0-1",
			deploymentOptions: []string{"3.0-1"},
			forceUpgradeToAF3: false,
			expected:          false,
			expectedError:     "Can only upgrade deployment from Airflow 2 to Airflow 3 with the --force-upgrade-to-af3 flag",
		},

		// AF3 version cases
		{
			name:              "newer AF3 version is valid upgrade",
			currentVersion:    "3.0-1",
			newVersion:        "3.1-1",
			deploymentOptions: []string{"3.1-1"},
			expected:          true,
		},
		{
			name:              "older AF3 version is invalid upgrade",
			currentVersion:    "3.1-1",
			newVersion:        "3.0-3",
			deploymentOptions: []string{"3.0-3"},
			expected:          false,
			expectedError:     "Cannot deploy a downgraded Astro Runtime version",
		},
		{
			name:              "AF3 to AF2 is invalid upgrade",
			currentVersion:    "3.0-1",
			newVersion:        "4.2.6",
			deploymentOptions: []string{"4.2.6"},
			expected:          false,
			expectedError:     "Cannot deploy a downgraded Astro Runtime version",
		},

		// Deployment options validation cases
		{
			name:              "version not in deployment options is invalid",
			currentVersion:    "4.2.5",
			newVersion:        "4.2.6",
			deploymentOptions: []string{"4.2.5", "4.2.4"},
			expected:          false,
			expectedError:     "Cannot deploy an unsupported Astro Runtime version",
		},
		{
			name:              "version in deployment options is valid",
			currentVersion:    "4.2.5",
			newVersion:        "4.2.6",
			deploymentOptions: []string{"4.2.6", "4.2.5"},
			expected:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout to check error messages
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			outC := make(chan string)
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outC <- buf.String()
			}()

			result := ValidRuntimeVersion(tc.currentVersion, tc.newVersion, tc.deploymentOptions, tc.forceUpgradeToAF3)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Check result
			assert.Equal(t, tc.expected, result)

			// Check error message if expected
			if tc.expectedError != "" {
				output := <-outC
				assert.Contains(t, output, tc.expectedError)
			}
		})
	}
}

// mockTransport implements http.RoundTripper for mocking HTTP responses
type mockTransport struct {
	response *http.Response
}

func (m *mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return m.response, nil
}

func TestWarnNonLatestVersion(t *testing.T) {
	// Create a mock HTTP client
	mockClient := &httputil.HTTPClient{
		HTTPClient: &http.Client{
			Transport: &mockTransport{
				response: &http.Response{
					StatusCode: 200,
					Body: io.NopCloser(strings.NewReader(`{
						"runtimeVersions": {
							"5.0.1": {
								"metadata": {
									"airflowVersion": "2.2.5",
									"channel": "stable",
									"releaseDate": "2024-01-01"
								},
								"migrations": {
									"airflowDatabase": false
								}
							},
							"5.0.2": {
								"metadata": {
									"airflowVersion": "2.2.6",
									"channel": "stable",
									"releaseDate": "2024-01-02"
								},
								"migrations": {
									"airflowDatabase": false
								}
							}
						}
					}`)),
				},
			},
		},
	}

	t.Run("older version shows warning", func(t *testing.T) {
		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()

		r, w, _ := os.Pipe()
		os.Stdout = w

		WarnIfNonLatestVersion("4.2.5", mockClient)

		w.Close()
		out, _ := io.ReadAll(r)
		assert.Equal(t, "WARNING! You are currently running Astro Runtime Version 4.2.5\nConsider upgrading to the latest version, Astro Runtime 5.0.2\n", string(out))
	})

	t.Run("latest version shows no warning", func(t *testing.T) {
		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()

		r, w, _ := os.Pipe()
		os.Stdout = w

		WarnIfNonLatestVersion("5.0.2", mockClient)

		w.Close()
		out, _ := io.ReadAll(r)
		assert.Equal(t, "", string(out))
	})

	t.Run("newer version shows no warning", func(t *testing.T) {
		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()

		r, w, _ := os.Pipe()
		os.Stdout = w

		WarnIfNonLatestVersion("5.0.3", mockClient)

		w.Close()
		out, _ := io.ReadAll(r)
		assert.Equal(t, "", string(out))
	})
}

func TestCheckPyTest(t *testing.T) {
	mockDeployImage := "test-image"

	mockContainerHandler := new(mocks.ContainerHandler)
	mockContainerHandler.On("Pytest", "", "", mockDeployImage, "", "").Return("", errMock).Once()

	// random error on running airflow pytest
	err := checkPytest("", mockDeployImage, "", mockContainerHandler)
	assert.ErrorIs(t, err, errMock)
	mockContainerHandler.AssertExpectations(t)

	// airflow pytest exited with status code 1
	mockContainerHandler.On("Pytest", "", "", mockDeployImage, "", "").Return("exit code 1", errMock).Once()
	err = checkPytest("", mockDeployImage, "", mockContainerHandler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 pytest in your tests directory failed. Fix the issues listed or rerun the command without the '--pytest' flag to deploy")
	mockContainerHandler.AssertExpectations(t)
}

func TestDeployClientImage(t *testing.T) {
	// Store original DockerLogin function to restore after tests
	originalDockerLogin := airflow.DockerLogin
	defer func() {
		airflow.DockerLogin = originalDockerLogin
	}()

	t.Run("successful client deploy", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)
		// Mock DockerLogin
		dockerLoginCalled := false
		var capturedRegistry, capturedUsername, capturedToken string
		airflow.DockerLogin = func(registry, username, token string) error {
			dockerLoginCalled = true
			capturedRegistry = registry
			capturedUsername = username
			capturedToken = token
			return nil
		}

		// Mock image handler
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("Build", "Dockerfile.client", "", mock.AnythingOfType("types.ImageBuildConfig")).Return(nil).Once()
		mockImageHandler.On("Push", mock.AnythingOfType("string"), "", "", false).Return("", nil).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		// Mock config.CFG.RemoteClientRegistry
		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:              "/test/path",
			BuildSecretString: "",
		}

		err = DeployClientImage(deployInput)
		assert.NoError(t, err)
		assert.True(t, dockerLoginCalled, "DockerLogin should have been called")
		assert.Equal(t, "images.astronomer.cloud", capturedRegistry)
		assert.Equal(t, "cli", capturedUsername)
		assert.Equal(t, "test-token", capturedToken)
		mockImageHandler.AssertExpectations(t)
	})

	t.Run("docker login failure", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock DockerLogin to return error
		airflow.DockerLogin = func(registry, username, token string) error {
			return errors.New("login failed")
		}

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:              "/test/path",
			BuildSecretString: "",
		}

		err = DeployClientImage(deployInput)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate with registry images.astronomer.cloud")
	})

	t.Run("missing registry configuration", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock DockerLogin (shouldn't be called)
		dockerLoginCalled := false
		airflow.DockerLogin = func(registry, username, token string) error {
			dockerLoginCalled = true
			return nil
		}

		config.CFG.RemoteClientRegistry.SetHomeString("") // Empty registry

		deployInput := InputClientDeploy{
			Path:              "/test/path",
			BuildSecretString: "",
		}

		err = DeployClientImage(deployInput)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remote client registry is not configured")
		assert.False(t, dockerLoginCalled, "DockerLogin should not have been called")
	})

	t.Run("build failure", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock successful DockerLogin
		airflow.DockerLogin = func(registry, username, token string) error {
			return nil
		}

		// Mock image handler with build failure
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("Build", "Dockerfile.client", "", mock.AnythingOfType("types.ImageBuildConfig")).Return(errors.New("build failed")).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:              "/test/path",
			BuildSecretString: "",
		}

		err = DeployClientImage(deployInput)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build client image")
		mockImageHandler.AssertExpectations(t)
	})

	t.Run("push failure", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock successful DockerLogin
		airflow.DockerLogin = func(registry, username, token string) error {
			return nil
		}

		// Mock image handler with push failure
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("Build", "Dockerfile.client", "", mock.AnythingOfType("types.ImageBuildConfig")).Return(nil).Once()
		mockImageHandler.On("Push", mock.AnythingOfType("string"), "", "", false).Return("", errors.New("push failed")).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:              "/test/path",
			BuildSecretString: "",
		}

		err = DeployClientImage(deployInput)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to push client image")
		mockImageHandler.AssertExpectations(t)
	})

	t.Run("deploy with custom image name", func(t *testing.T) {
		// Set up current context
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Token = "test-token"
		err = ctx.SetContext()
		assert.NoError(t, err)

		// Mock image handler
		mockImageHandler := new(mocks.ImageHandler)
		mockImageHandler.On("TagLocalImage", "custom-image:tag").Return(nil).Once()
		// Remote image will use timestamp tag, not the user-provided tag
		mockImageHandler.On("Push", mock.MatchedBy(func(remoteImage string) bool {
			// Verify it uses timestamp-based tag format, not "tag" from the user input
			return strings.Contains(remoteImage, "test-registry:latest:deploy-") &&
				!strings.Contains(remoteImage, ":tag")
		}), "", "", false).Return("", nil).Once()

		// Override airflowImageHandler
		originalAirflowImageHandler := airflowImageHandler
		airflowImageHandler = func(imageName string) airflow.ImageHandler {
			return mockImageHandler
		}
		defer func() {
			airflowImageHandler = originalAirflowImageHandler
		}()

		config.CFG.RemoteClientRegistry.SetHomeString("test-registry:latest")

		deployInput := InputClientDeploy{
			Path:              "/test/path",
			ImageName:         "custom-image:tag",
			BuildSecretString: "",
		}

		err = DeployClientImage(deployInput)
		assert.NoError(t, err)
		mockImageHandler.AssertExpectations(t)
	})
}
