package azure_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/astronomer/astro-cli/pkg/azure"

	azure_mocks "github.com/astronomer/astro-cli/pkg/azure/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TODO this is scratch to figure out how to test any of the azure mock client's methods
// We may drop this file all together
var (
	testError  = errors.New("test-error")
	testError2 = errors.New("test-error-2")
)

func TestCreateSASDagClient(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.Azure)
		testBClient, err := azblob.NewBlockBlobClient("test-url", nil, nil)
		assert.NoError(t, err)
		clientToBeReturned := azure.DagClient{BlobClient: testBClient}
		mockAZBlobClient.On("CreateSASDagClient", mock.Anything).Return(clientToBeReturned, nil)
		testClient, err := mockAZBlobClient.CreateSASDagClient("test")
		assert.NoError(t, err)
		assert.NotNil(t, testClient.BlobClient)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		clientToBeReturned := azure.DagClient{}
		mockAZBlobClient := new(azure_mocks.Azure)
		mockAZBlobClient.On("CreateSASDagClient", mock.Anything).Return(clientToBeReturned, testError)
		_, err := mockAZBlobClient.CreateSASDagClient("test")
		assert.ErrorIs(t, err, testError)
		mockAZBlobClient.AssertExpectations(t)
	})
}

func TestUpload(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.Azure)
		testBClient, err := azblob.NewBlockBlobClient("test-url", nil, nil)
		assert.NoError(t, err)
		clientToBeReturned := azure.DagClient{BlobClient: testBClient}
		mockResponse := "yay"
		mockAZBlobClient.On("CreateSASDagClient", mock.Anything).Return(clientToBeReturned, nil).Once()
		mockAZBlobClient.On("Upload", mock.Anything).Return(mockResponse, nil).Once()
		testClient, err := mockAZBlobClient.CreateSASDagClient("test")
		resp, err := mockAZBlobClient.Upload(io.Reader(strings.NewReader("abcde")))
		assert.NoError(t, err)
		assert.NotNil(t, testClient.BlobClient)
		assert.Equal(t, mockResponse, resp)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		clientToBeReturned := azure.DagClient{}
		mockAZBlobClient := new(azure_mocks.Azure)
		mockAZBlobClient.On("CreateSASDagClient", mock.Anything).Return(clientToBeReturned, testError)
		mockAZBlobClient.On("Upload", mock.Anything).Return("", testError2)
		_, err := mockAZBlobClient.CreateSASDagClient("test")
		assert.ErrorIs(t, err, testError)
		_, err = mockAZBlobClient.Upload(io.Reader(strings.NewReader("abcde")))
		assert.ErrorIs(t, err, testError2)
		mockAZBlobClient.AssertExpectations(t)
	})
}

func TestNewBlockBlobClientWithNoCredential(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.AzureClientAPI)
		testBClient, err := azblob.NewBlockBlobClient("test-url", nil, nil)
		assert.NoError(t, err)
		clientToBeReturned := azblob.BlockBlobClient{BlobClient: testBClient.BlobClient}
		mockAZBlobClient.On("NewBlockBlobClientWithNoCredential", mock.Anything, mock.Anything).Return(&clientToBeReturned, nil)
		testClient, err := mockAZBlobClient.NewBlockBlobClientWithNoCredential("test", nil)
		assert.NoError(t, err)
		assert.NotNil(t, testClient.BlobClient)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		clientToBeReturned := azblob.BlockBlobClient{}
		mockAZBlobClient := new(azure_mocks.AzureClientAPI)
		mockAZBlobClient.On("NewBlockBlobClientWithNoCredential", mock.Anything, mock.Anything).Return(&clientToBeReturned, testError)
		_, err := mockAZBlobClient.NewBlockBlobClientWithNoCredential("test", nil)
		assert.ErrorIs(t, err, testError)
		mockAZBlobClient.AssertExpectations(t)
	})
}

func TestUploadStream(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.AzureClientAPI)
		mockResponse := &azblob.BlockBlobCommitBlockListResponse{}
		mockAZBlobClient.On("UploadStream", mock.Anything, mock.Anything, mock.Anything).Return(mockResponse, nil)
		response, err := mockAZBlobClient.UploadStream(nil, nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, response)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.AzureClientAPI)
		// mockResponse := &azblob.BlockBlobCommitBlockListResponse{}
		mockResponse := map[string]string{"VersionID": "version-id"}
		mockAZBlobClient.On("UploadStream", mock.Anything, mock.Anything, mock.Anything).Return(mockResponse, testError)
		response, err := mockAZBlobClient.UploadStream(nil, nil, nil)
		assert.ErrorIs(t, err, testError)
		assert.Equal(t, mockResponse, response)
		mockAZBlobClient.AssertExpectations(t)
	})
}
