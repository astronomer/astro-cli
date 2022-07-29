package azure_test

import (
	"context"
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

var (
	errMock  = errors.New("test error")
	errMock2 = errors.New("test error 2")
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
		mockAZBlobClient.On("CreateSASDagClient", mock.Anything).Return(clientToBeReturned, errMock)
		_, err := mockAZBlobClient.CreateSASDagClient("test")
		assert.ErrorIs(t, err, errMock)
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
		mockAZBlobClient.On("Upload", clientToBeReturned, mock.Anything).Return(mockResponse, nil).Once()
		testClient, err := mockAZBlobClient.CreateSASDagClient("test")
		assert.NoError(t, err)
		resp, err := mockAZBlobClient.Upload(clientToBeReturned, io.Reader(strings.NewReader("abcde")))
		assert.NoError(t, err)
		assert.NotNil(t, testClient.BlobClient)
		assert.Equal(t, mockResponse, resp)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		clientToBeReturned := azure.DagClient{}
		mockAZBlobClient := new(azure_mocks.Azure)
		mockAZBlobClient.On("CreateSASDagClient", mock.Anything).Return(clientToBeReturned, errMock)
		mockAZBlobClient.On("Upload", clientToBeReturned, mock.Anything).Return("", errMock2)
		_, err := mockAZBlobClient.CreateSASDagClient("test")
		assert.ErrorIs(t, err, errMock)
		_, err = mockAZBlobClient.Upload(clientToBeReturned, io.Reader(strings.NewReader("abcde")))
		assert.ErrorIs(t, err, errMock2)
		mockAZBlobClient.AssertExpectations(t)
	})
}

func TestNewBlockBlobClientWithNoCredential(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.ClientAPI)
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
		mockAZBlobClient := new(azure_mocks.ClientAPI)
		mockAZBlobClient.On("NewBlockBlobClientWithNoCredential", mock.Anything, mock.Anything).Return(&clientToBeReturned, errMock)
		_, err := mockAZBlobClient.NewBlockBlobClientWithNoCredential("test", nil)
		assert.ErrorIs(t, err, errMock)
		mockAZBlobClient.AssertExpectations(t)
	})
}

func TestUploadStream(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.ClientAPI)
		mockResponse := &azblob.BlockBlobCommitBlockListResponse{}
		mockAZBlobClient.On("UploadStream", mock.Anything, mock.Anything, mock.Anything).Return(mockResponse, nil)
		response, err := mockAZBlobClient.UploadStream(context.TODO(), nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, response)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.ClientAPI)
		mockResponse := &azblob.BlockBlobCommitBlockListResponse{}
		mockAZBlobClient.On("UploadStream", mock.Anything, mock.Anything, mock.Anything).Return(mockResponse, errMock)
		response, err := mockAZBlobClient.UploadStream(context.TODO(), nil, nil)
		assert.ErrorIs(t, err, errMock)
		assert.Equal(t, mockResponse, response)
		mockAZBlobClient.AssertExpectations(t)
	})
}
