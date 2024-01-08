package deploy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
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
	createDeployResponse = astrocore.CreateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Deploy{
			Id:                "test-id",
			DagTarballVersion: &dagTarballVersionTest,
			ImageRepository:   "test-repository",
			DagsUploadUrl:     &dagsUploadTestURL,
		},
	}
	updateDeployResponse = astrocore.UpdateDeployResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Deploy{
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
				{Version: "4.0.0"},
				{Version: "5.0.0"},
			},
		},
	}
	deploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:               deploymentID,
			RuntimeVersion:   "4.2.5",
			Namespace:        "test-name",
			WorkspaceId:      ws,
			WebServerUrl:     "test-url",
			DagDeployEnabled: false,
			Type:             &hybridType,
		},
	}
	deploymentResponseCICD = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:               deploymentID,
			RuntimeVersion:   "4.2.5",
			Namespace:        "test-name",
			WorkspaceId:      ws,
			WebServerUrl:     "test-url",
			DagDeployEnabled: false,
			IsCicdEnforced:   true,
			Type:             &hybridType,
		},
	}
	deploymentResponseDags = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                deploymentID,
			RuntimeVersion:    "4.2.5",
			Namespace:         "test-name",
			WorkspaceId:       ws,
			WebServerUrl:      "test-url",
			DagDeployEnabled:  true,
			IsCicdEnforced:    false,
			Type:              &hybridType,
			DagTarballVersion: &tarballVersion,
		},
	}
	mockCoreDeploymentResponse = []astrocore.Deployment{
		{
			Id:     deploymentID,
			Status: "HEALTHY",
		},
	}
	mockListDeploymentsResponse = astrocore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
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
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(5)
	mockCoreClient.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&updateDeployResponse, nil).Times(5)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockContainerHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", nil)
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

	defer testUtil.MockUserInput(t, "y")()
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
	assert.ErrorContains(t, err, "timed out waiting for the deployment to become healthy")

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

	err := Deploy(deployInput, mockPlatformCoreClient, nil)
	assert.ErrorIs(t, err, errCiCdEnforcementUpdate)

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
	mockClient := new(astro_mocks.Client)

	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(9)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(7)
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(7)
	mockCoreClient.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&updateDeployResponse, nil).Times(7)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("", nil)
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

	defer testUtil.MockUserInput(t, "y")()
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
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles1/")
	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(t)
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
	mockClient := new(astro_mocks.Client)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponseDags, nil).Times(12)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(4)
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(6)
	mockCoreClient.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&updateDeployResponse, nil).Times(6)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	defer testUtil.MockUserInput(t, "y")()
	err := Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	// Test pytest with dags deploy
	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("", nil)
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
	// image deploy
	defer testUtil.MockUserInput(t, "y")()
	deployInput.Image = true

	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	deployInput.Pytest = ""
	deployInput.WaitForStatus = true
	dagOnlyDeploySleepTime = 1
	timeoutNum = 1
	err = Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.ErrorContains(t, err, "timed out waiting for the deployment to become healthy")

	defer os.RemoveAll("./testfiles/dags/")

	mockCoreClient.AssertExpectations(t)
	mockClient.AssertExpectations(t)
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
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(1)

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
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(3)

	defer testUtil.MockUserInput(t, "y")()
	err := Deploy(deployInput, mockPlatformCoreClient, mockCoreClient)
	assert.Equal(t, err.Error(), "DAG-only deploys are not enabled for this Deployment. Run 'astro deployment update test-deployment-id --dag-deploy enable' to enable DAG-only deploys.")

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("4.2.5", nil)
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
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, nil).Times(1)
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(2)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("4.2.5", nil)
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

	defer testUtil.MockUserInput(t, "y")()
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
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(4)
	mockCoreClient.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&updateDeployResponse, nil).Times(4)

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
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("", nil)
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
	mockCoreClient.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&createDeployResponse, nil).Times(4)
	mockCoreClient.On("UpdateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&updateDeployResponse, nil).Times(4)

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
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("", nil)
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
	_, err := buildImage("./testfiles/", "4.2.5", "", "", "", "", false, nil)
	assert.ErrorIs(t, err, errMock)

	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", mock.Anything, runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	// dockerfile parsing error
	dockerfile = "Dockerfile.invalid"
	_, err = buildImage("./testfiles/", "4.2.5", "", "", "", "", false, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse dockerfile")

	// failed to get runtime releases
	dockerfile = "Dockerfile"
	mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentOptionsResponse, errMock).Once()
	_, err = buildImage("./testfiles/", "4.2.5", "", "", "", "", false, mockPlatformCoreClient)
	assert.ErrorIs(t, err, errMock)
	mockCoreClient.AssertExpectations(t)
	mockPlatformCoreClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
}

func TestIsValidUpgrade(t *testing.T) {
	resp := IsValidUpgrade("4.2.5", "4.2.6")
	assert.True(t, resp)

	resp = IsValidUpgrade("4.2.6", "4.2.5")
	assert.False(t, resp)

	resp = IsValidUpgrade("", "4.2.6")
	assert.True(t, resp)
}

func TestIsValidTag(t *testing.T) {
	resp := IsValidTag([]string{"4.2.5", "4.2.6"}, "4.2.7")
	assert.False(t, resp)

	resp = IsValidTag([]string{"4.2.5", "4.2.6"}, "4.2.6")
	assert.True(t, resp)
}

func TestCheckVersion(t *testing.T) {
	httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), false)
	latestRuntimeVersion, _ := airflowversions.GetDefaultImageTag(httpClient, "")

	// version that is older than newest
	buf := new(bytes.Buffer)
	CheckVersion("1.0.0", buf)
	assert.Contains(t, buf.String(), "WARNING! You are currently running Astro Runtime Version")

	// version that is latest
	CheckVersion(latestRuntimeVersion, buf)
	assert.Contains(t, buf.String(), "Runtime Version: "+latestRuntimeVersion)
}

func TestCheckVersionBeta(t *testing.T) {
	// version that newer than latest
	buf := new(bytes.Buffer)
	defer testUtil.MockUserInput(t, "y")()
	CheckVersion("10.0.0", buf)
	assert.Contains(t, buf.String(), "")
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
