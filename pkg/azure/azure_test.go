package azure

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

var errMock = errors.New("test error")

type Suite struct {
	suite.Suite
}

func TestAzure(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestUpload() {
	s.Run("happy path", func() {
		azureUploader = func(sasLink string, file io.Reader) (string, error) {
			return "version-id", nil
		}

		resp, err := azureUpload("test-url", io.Reader(strings.NewReader("abcde")))
		s.NoError(err)
		s.Equal("version-id", resp)
	})
	s.Run("error path", func() {
		azureUploader = func(sasLink string, file io.Reader) (string, error) {
			return "", errMock
		}

		_, err := azureUpload("test-url", io.Reader(strings.NewReader("abcde")))
		s.ErrorIs(err, errMock)
	})
}
