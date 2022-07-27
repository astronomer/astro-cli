package deploy

import (
	"bytes"
	"errors"
	"os"
	// "io"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	// "github.com/astronomer/astro-cli/pkg/azure"
	azure_mocks "github.com/astronomer/astro-cli/pkg/azure/mocks"
	"github.com/spf13/afero"
)

var errMock = errors.New("mock error")

// var mockBlobClient *azblob.BlockBlobClient

// // This helps in assigning mock at the runtime instead of compile time
// var azureUploadMock func(dagFile io.Reader) (string, error)

// type azureClientMock struct{}

// func (a azureClientMock) azureUpload(dagFile io.Reader) (string, error) {
// 	return azureUploadMock(dagFile)
// }

func TestDeploySuccess(t *testing.T) {
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything).Return(mockDeplyResp, nil).Times(3)
	mockClient.On("ListPublicRuntimeReleases").Return([]astro.RuntimeRelease{{Version: "4.2.5", AirflowVersion: "2.2.5"}}, nil).Times(3)
	mockClient.On("CreateImage", mock.Anything).Return(&astro.Image{}, nil).Times(3)
	mockClient.On("DeployImage", mock.Anything).Return(&astro.Image{}, nil).Times(3)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("", nil)
		mockImageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string, isPyTestCompose bool) (airflow.ContainerHandler, error) {
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

	err = Deploy("./testfiles/", "", "test-ws-id", "parse", "", "", true, false, mockClient)
	assert.NoError(t, err)

	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "", false, false, mockClient)
	assert.NoError(t, err)

	// test custom image
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "custom-image", false, false, mockClient)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestDeployFailure(t *testing.T) {
	// no context set failure
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	err := config.ResetCurrentContext()
	assert.NoError(t, err)
	err = Deploy("./testfiles/", "test-id", "test-ws-id", "pytest", "", "", true, false, nil)
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
	mockClient.On("ListDeployments", mock.Anything).Return(mockDeplyResp, nil).Once()
	mockClient.On("ListPublicRuntimeReleases").Return([]astro.RuntimeRelease{{Version: "4.2.5", AirflowVersion: "2.2.5"}}, nil).Once()

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	mockContainerHandler := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string, isPyTestCompose bool) (airflow.ContainerHandler, error) {
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

	err = Deploy("./testfiles/", "", "test-ws-id", "parse", "", "", true, false, mockClient)
	assert.ErrorIs(t, err, errDagsParseFailed)

	mockClient.AssertExpectations(t)
	mockImageHandler.AssertExpectations(t)
	mockContainerHandler.AssertExpectations(t)
}

func TestDeployDagsSuccess(t *testing.T) {
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

	mockInitiateDagDeploymentResponse := astro.InitiateDagDeployment{
		ID:     "test-dag-deployment-id",
		DagURL: "http://test-dag-url",
	}

	mockDagDeploymentStatusResponse := astro.DagDeploymentStatus{
		ID:            "test-dag-deployment-status-id",
		DeploymentID:  "test-id",
		Action:        "UPLOAD",
		VersionID:     "version-id",
		Status:        "SUCCESS",
		Message:       "some-message",
		CreatedAt:     "created-date",
		InitiatorID:   "initiator-id",
		InitiatorType: "user",
	}
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	config.CFG.ShowWarnings.SetHomeString("false")
	mockClient := new(astro_mocks.Client)
	// mockBlobClient = new(azblob.BlockBlobClient)
	mockAzure := new(azure_mocks.Azure)
	mockAzureClientAPI := new(azure_mocks.AzureClientAPI)
	// mockDagClient, err := mockAzureClient.CreateSASDagClient("http://test-dag-url")
	// if err != nil {
	//     t.Fatal(err)
	// }
	mockClient.On("ListDeployments", mock.Anything).Return(mockDeplyResp, nil).Once()
	mockClient.On("InitiateDagDeployment", mock.Anything).Return(mockInitiateDagDeploymentResponse, nil).Once()
	// var dagFile io.Reader
	filePath := "test.yaml"
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, filePath, []byte(`test`), 0o777)
	tempFile, _ := os.CreateTemp("", "test.yaml")
	// defer os.Remove(tempFile.Name())
	// dagClient := azure.DagClient{
	// 	"UploadStream": func() (*azblob.BlobContentInfo, error) {
	// 		return &map[string]string{"VersionID": "version-id"}, nil
	// 	},
	// }
	// dagClient := azure.DagClient{
	// 	"UploadStream": func() (*azblob.BlockBlobCommitBlockListResponse, error) {
	// 		return &azblob.BlockBlobCommitBlockListResponse{map[string]interface{}{VersionID: "version-id"}}, nil
	// 	},
	// }
	// dagClient := &azblob.BlockBlobClient{
	// 	"UploadStream": func() (*azblob.BlockBlobCommitBlockListResponse, error) {
	// 		return &azblob.BlockBlobCommitBlockListResponse{map[string]interface{}{"VersionID": "version-id"}}, nil
	// 	},
	// }
	// fmt.Println(*mockAzureClient)
	// mockAzureClient.On("getBlockBlobClientFromSAS", "http://test-dag-url").Return(dagClient, nil).Once()
	// dagClient := &azblob.BlockBlobClient{}
	cli, err := azblob.NewBlockBlobClientWithNoCredential(mockInitiateDagDeploymentResponse.DagURL, nil)
	fmt.Println(cli)
	fmt.Println(err)
	res, err := cli.UploadStream(context.TODO(), tempFile, azblob.UploadStreamOptions{})
	fmt.Println(res)
	fmt.Println(err)
	// {
	// 	NewBlockBlobClientWithNoCredential: func(url string) (*azblob.BlockBlobClient, error) {
	// 		return azblob.BlockBlobClient{}, nil
	// 	},
	// }
	// mockAzureClientAPI.On("NewBlockBlobClientWithNoCredential", mockInitiateDagDeploymentResponse.DagURL, nil).Return(dagClient, nil).Once()
	// mockAzureClientAPI.On("UploadStream", context.TODO(), dagFile, azblob.UploadStreamOptions{}).Return(map[string]string{"VersionID": "version-id"}, nil).Once()
	// mockAzure.On("getBlockBlobClientFromSAS", mockInitiateDagDeploymentResponse.DagURL).Return(dagClient, nil).Once()
	// mockAzure.On("CreateSASDagClient", mockInitiateDagDeploymentResponse.DagURL).Return(dagClient, nil).Once()
	// mockAzure.On("Upload", mock.Anything).Return("version-id", nil).Once()
	// azureClient = azureClientMock{}
	// azureUploadMock = func(dagFile io.Reader) (string, error) {
	// 	return "version-id", nil
	// }
	// _, err = mockDagClient.Upload(dagFile)
	// assert.NoError(t, err)
	// uploadRes, err := mockBlobClient.UploadStream(context.TODO(), tempFile, azblob.UploadStreamOptions{})
	// fmt.Println(uploadRes);
	// assert.NoError(t, err)
	// assert.Equal(t, *uploadRes.VersionID, "version-id")

	mockClient.On("ReportDagDeploymentStatus", mock.Anything).Return(mockDagDeploymentStatusResponse, nil).Once()

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

	err = Deploy("./testfiles/", "", "test-ws-id", "", "", "", true, true, mockClient)
	fmt.Println(err)
	assert.NoError(t, err)

	mockAzureClientAPI.AssertExpectations(t)

	mockAzure.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestBuildImageFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)
	ctx.SetSystemAdmin(true)

	mockImageHandler := new(mocks.ImageHandler)
	airflowImageHandler = func(image string) airflow.ImageHandler {
		mockImageHandler.On("Build", mock.Anything).Return(nil)
		mockImageHandler.On("GetLabel", runtimeImageLabel).Return("4.2.5", nil)
		return mockImageHandler
	}

	// dockerfile parsing error
	dockerfile = "Dockerfile.invalid"
	_, err = buildImage(&ctx, "./testfiles/", "4.2.5", "", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse dockerfile")

	// failed to get runtime releases
	dockerfile = "Dockerfile"
	mockClient := new(astro_mocks.Client)
	mockClient.On("ListInternalRuntimeReleases").Return([]astro.RuntimeRelease{}, errMock).Once()
	_, err = buildImage(&ctx, "./testfiles/", "4.2.5", "", "", mockClient)
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
	mockContainerHandler.On("Pytest", mock.Anything, "", mockDeployImage).Return("", errMock).Once()

	// random error on running airflow pytest
	err := checkPytest("", mockDeployImage, mockContainerHandler)
	assert.ErrorIs(t, err, errMock)
	mockContainerHandler.AssertExpectations(t)

	// airflow pytest exited with status code 1
	mockContainerHandler.On("Pytest", mock.Anything, "", mockDeployImage).Return("exit code 1", errMock).Once()
	err = checkPytest("", mockDeployImage, mockContainerHandler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 pytest in your tests directory failed. Fix the issues listed or rerun the command without the '--pytest' flag to deploy")
	mockContainerHandler.AssertExpectations(t)
}
