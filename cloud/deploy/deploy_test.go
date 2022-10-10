package deploy

import (
	"bytes"
	"errors"
	"fmt"
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
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock                  = errors.New("mock error")
	org                      = "test-org-id"
	ws                       = "test-ws-id"
	initiatedDagDeploymentID = "test-dag-deployment-id"
	runtimeID                = "test-id"
	dagURL                   = "http://fake-url.windows.core.net"
)

func TestDeployWithoutDagsDeploySuccess(t *testing.T) {
	mockDeplyResp := astro.Deployment{
		ID:             "test-id",
		ReleaseName:    "test-name",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{
			Webserver: astro.Webserver{URL: "test-url"},
		},
		CreatedAt:        time.Now(),
		DagDeployEnabled: false,
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("GetDeployment", mock.Anything).Return(mockDeplyResp, nil).Times(3)
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
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
	err = Deploy("./testfiles/", "", "test-ws-id", "parse", "", "", "", "", true, false, mockClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "", "", "", false, false, mockClient)
	assert.NoError(t, err)

	// test custom image
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "custom-image", "", "", false, false, mockClient)
	assert.NoError(t, err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "custom-image", "test-name", "", false, false, mockClient)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestEnableDagsDeploySuccess(t *testing.T) {
	deploymentUpdateInput := astro.UpdateDeploymentInput{
		ID:               "test-id",
		ClusterID:        "",
		DagDeployEnabled: true,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: "CeleryExecutor",
		},
	}

	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: false}}, nil).Once()
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: true}}, nil).Once()
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: false}}, nil).Once()
	mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id", DagDeployEnabled: true}, nil).Once()
	mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{}, errMock).Once()
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()
	mockClient.On("CreateImage", mock.Anything).Return(&astro.Image{}, nil).Once()
	mockClient.On("DeployImage", mock.Anything).Return(&astro.Image{}, nil).Once()
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Once()

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		RuntimeID:                runtimeID,
		Action:                   "UPLOAD",
		VersionID:                "version-id",
		Status:                   "SUCCEEDED",
		Message:                  "Dags uploaded successfully",
	}
	mockClient.On("ReportDagDeploymentStatus", reportDagDeploymentStatusInput).Return(astro.DagDeploymentStatus{}, nil).Once()

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(nil)
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
	err = Deploy("./testfiles/", "test-id", ws, "parse", "", "", "", "enable", true, false, mockClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", ws, "parse", "", "", "", "enable", true, false, mockClient)
	assert.Contains(t, err.Error(), errMock.Error())

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", ws, "parse", "", "", "", "enable", true, false, mockClient)
	assert.Contains(t, err.Error(), errMock.Error())

	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestDisableDagsDeploySuccess(t *testing.T) {
	deploymentUpdateInput := astro.UpdateDeploymentInput{
		ID:               "test-id",
		ClusterID:        "",
		DagDeployEnabled: false,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: "CeleryExecutor",
		},
	}

	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: true}}, nil).Once()
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: false}}, nil).Once()
	mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id", DagDeployEnabled: false}, nil).Once()
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()
	mockClient.On("CreateImage", mock.Anything).Return(&astro.Image{}, nil).Once()
	mockClient.On("DeployImage", mock.Anything).Return(&astro.Image{}, nil).Once()

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
		mockContainerHandler.On("Parse", mock.Anything, mock.Anything).Return(nil)
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
	err = Deploy("./testfiles/", "test-id", ws, "parse", "", "", "", "disable", true, false, mockClient)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestCancelDisableDagsDeploySuccess(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("true")
	defer testUtil.MockUserInput(t, "n")()
	mockClient := new(astro_mocks.Client)

	os.Mkdir("./testfiles/dags", 0o755)
	fileutil.WriteStringToFile("./testfiles/dags/dags.py", "test")
	defer os.RemoveAll("./testfiles/dags/")

	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: true}}, nil).Once()

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

	err = Deploy("./testfiles/", "test-id", ws, "parse", "", "", "", "disable", true, false, mockClient)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestDeployWithDagsDeploySuccess(t *testing.T) {
	mockDeplyResp := astro.Deployment{
		ID:             "test-id",
		ReleaseName:    "test-name",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{
			Webserver: astro.Webserver{URL: "test-url"},
		},
		CreatedAt:        time.Now(),
		DagDeployEnabled: true,
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("GetDeployment", mock.Anything).Return(mockDeplyResp, nil).Times(3)
	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", DagDeployEnabled: true}}, nil).Times(1)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Times(4)
	mockClient.On("CreateImage", mock.Anything).Return(&astro.Image{}, nil).Times(4)
	mockClient.On("DeployImage", mock.Anything).Return(&astro.Image{}, nil).Times(4)
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Times(2)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		RuntimeID:                runtimeID,
		Action:                   "UPLOAD",
		VersionID:                "version-id",
		Status:                   "SUCCEEDED",
		Message:                  "Dags uploaded successfully",
	}
	mockClient.On("ReportDagDeploymentStatus", reportDagDeploymentStatusInput).Return(astro.DagDeploymentStatus{}, nil).Times(2)

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
	err = Deploy("./testfiles/", "", "test-ws-id", "parse", "", "", "", "", true, false, mockClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "", "", "", false, false, mockClient)
	assert.NoError(t, err)

	// test custom image
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "custom-image", "", "", false, false, mockClient)
	assert.NoError(t, err)

	config.CFG.ProjectDeployment.SetProjectString("test-id")
	// test both deploymentID and name used
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "custom-image", "test-name", "", false, false, mockClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestDagsDeploySuccess(t *testing.T) {
	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt: time.Now(),
		},
		{
			ID:             "test-id-2",
			ReleaseName:    "test-name-2",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt: time.Now(),
		},
	}

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Times(1)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockDeplyResp, nil).Times(2)
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Times(2)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		RuntimeID:                runtimeID,
		Action:                   "UPLOAD",
		VersionID:                "version-id",
		Status:                   "SUCCEEDED",
		Message:                  "Dags uploaded successfully",
	}
	mockClient.On("ReportDagDeploymentStatus", reportDagDeploymentStatusInput).Return(astro.DagDeploymentStatus{}, nil).Times(2)

	defer testUtil.MockUserInput(t, "y")()
	err := Deploy("./testfiles/", "test-id", "test-ws-id", "", "", "", "", "", true, true, mockClient)
	assert.NoError(t, err)

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

	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "all-tests", "", "", "", "", true, true, mockClient)
	assert.NoError(t, err)

	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(t)
}

func TestDagsDeployFailed(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt: time.Now(),
		},
		{
			ID:             "test-id-2",
			ReleaseName:    "test-name-2",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
			CreatedAt: time.Now(),
		},
	}

	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockDeplyResp, nil).Times(1)
	dagDeployErr := errors.New("dag deploy is not enabled for deployment") //nolint: goerr113
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, fmt.Errorf("%w", dagDeployErr)).Times(1)

	defer testUtil.MockUserInput(t, "y")()
	err := Deploy("./testfiles/", "test-id", "test-ws-id", "", "", "", "", "", true, true, mockClient)
	assert.Equal(t, err.Error(), "Dag Deploy is not enabled for deployment. Run 'astro deployment update test-id --dag-deploy enable' to enable dags deploy")

	mockClient.AssertExpectations(t)
}

func TestDagsDeployVR(t *testing.T) {
	runtimeID := "vr-test-id"
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)

	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Times(1)
	mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, errMock).Times(1)

	azureUploader = func(sasLink string, file io.Reader) (string, error) {
		return "version-id", nil
	}

	reportDagDeploymentStatusInput := &astro.ReportDagDeploymentStatusInput{
		InitiatedDagDeploymentID: initiatedDagDeploymentID,
		RuntimeID:                runtimeID,
		Action:                   "UPLOAD",
		VersionID:                "version-id",
		Status:                   "SUCCEEDED",
		Message:                  "Dags uploaded successfully",
	}
	mockClient.On("ReportDagDeploymentStatus", reportDagDeploymentStatusInput).Return(astro.DagDeploymentStatus{}, nil).Times(1)

	defer testUtil.MockUserInput(t, "y")()
	defer testUtil.MockUserInput(t, "y")()
	err := Deploy("./testfiles//", runtimeID, "test-ws-id", "", "", "", "", "", true, true, mockClient)
	assert.NoError(t, err)

	defer testUtil.MockUserInput(t, "y")()
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles//", runtimeID, "test-ws-id", "", "", "", "", "", true, true, mockClient)
	assert.ErrorIs(t, err, errMock)
	defer afero.NewOsFs().Remove("./testfiles/dags.tar")
	defer os.RemoveAll("./testfiles/dags/")

	mockClient.AssertExpectations(t)
}

func TestNoDagsDeploy(t *testing.T) {
	defer testUtil.MockUserInput(t, "n")()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	config.CFG.ShowWarnings.SetHomeString("true")
	mockClient := new(astro_mocks.Client)

	err := Deploy("./testfiles/", "test-id", "test-ws-id", "", "", "", "", "", true, true, mockClient)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestDeployFailure(t *testing.T) {
	// no context set failure
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	err := config.ResetCurrentContext()
	assert.NoError(t, err)
	defer testUtil.MockUserInput(t, "y")()
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "", "", "", true, false, nil)
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")

	// airflow parse failure
	mockDeplyResp := []astro.Deployment{
		{
			ID:             "test-id",
			ReleaseName:    "test-name",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Webserver: astro.Webserver{URL: "test-url"},
			},
		},
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", org, ws).Return(mockDeplyResp, nil).Once()
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
	err = Deploy("./testfiles/", "", "test-ws-id", "parse", "", "", "", "", true, false, mockClient)
	assert.ErrorIs(t, err, errDagsParseFailed)

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestBuildImageFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)

	mockImageHandler := new(mocks.ImageHandler)

	// image build failure
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(errMock).Once()
		return mockImageHandler
	}
	_, err = buildImage(&ctx, "./testfiles/", "4.2.5", "", "", false, nil)
	assert.ErrorIs(t, err, errMock)

	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	// dockerfile parsing error
	dockerfile = "Dockerfile.invalid"
	_, err = buildImage(&ctx, "./testfiles/", "4.2.5", "", "", false, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse dockerfile")

	// failed to get runtime releases
	dockerfile = "Dockerfile"
	mockClient := new(astro_mocks.Client)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()
	_, err = buildImage(&ctx, "./testfiles/", "4.2.5", "", "", false, mockClient)
	assert.ErrorIs(t, err, errMock)
	mockClient.AssertExpectations(t)
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
	mockContainerHandler.On("Pytest", []string{""}, "", mockDeployImage).Return("", errMock).Once()

	// random error on running airflow pytest
	err := checkPytest("", mockDeployImage, mockContainerHandler)
	assert.ErrorIs(t, err, errMock)
	mockContainerHandler.AssertExpectations(t)

	// airflow pytest exited with status code 1
	mockContainerHandler.On("Pytest", []string{""}, "", mockDeployImage).Return("exit code 1", errMock).Once()
	err = checkPytest("", mockDeployImage, mockContainerHandler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 pytest in your tests directory failed. Fix the issues listed or rerun the command without the '--pytest' flag to deploy")
	mockContainerHandler.AssertExpectations(t)
}
