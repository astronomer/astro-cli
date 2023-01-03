package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerNonZeroExitCodeError(t *testing.T) {
	errorMessage := DockerNonZeroExitCodeError(1)
	expectedErrorMessage := "docker command has returned a non-zero exit code:1"
	assert.EqualError(t, errorMessage, expectedErrorMessage)
}
