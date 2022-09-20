package azure

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("test error")

func TestUpload(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		azureUploader = func(sasLink string, file io.Reader) (string, error) {
			return "version-id", nil
		}

		resp, err := azureUpload("test-url", io.Reader(strings.NewReader("abcde")))
		assert.NoError(t, err)
		assert.Equal(t, "version-id", resp)
	})
	t.Run("error path", func(t *testing.T) {
		azureUploader = func(sasLink string, file io.Reader) (string, error) {
			return "", errMock
		}

		_, err := azureUpload("test-url", io.Reader(strings.NewReader("abcde")))
		assert.ErrorIs(t, err, errMock)
	})
}
