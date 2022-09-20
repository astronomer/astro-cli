package azure_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	azure_mocks "github.com/astronomer/astro-cli/pkg/azure/mocks"
	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("test error")

func TestUpload(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.Azure)
		mockResponse := "yay"
		mockAZBlobClient.On("Upload", "test-url", io.Reader(strings.NewReader("abcde"))).Return(mockResponse, nil).Once()
		resp, err := mockAZBlobClient.Upload("test-url", io.Reader(strings.NewReader("abcde")))
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
		mockAZBlobClient.AssertExpectations(t)
	})
	t.Run("error path", func(t *testing.T) {
		mockAZBlobClient := new(azure_mocks.Azure)
		mockAZBlobClient.On("Upload", "test-url", io.Reader(strings.NewReader("abcde"))).Return("", errMock).Once()
		_, err := mockAZBlobClient.Upload("test-url", io.Reader(strings.NewReader("abcde")))
		assert.ErrorIs(t, err, errMock)
		mockAZBlobClient.AssertExpectations(t)
	})
}
