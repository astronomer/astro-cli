package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArgNotSetError(t *testing.T) {
	errorMessage := ArgNotSetError("sample_argument")
	expectedErrorMessage := "argument not set:sample_argument"
	assert.EqualError(t, errorMessage, expectedErrorMessage)
}

func TestDockerNonZeroExitCodeError(t *testing.T) {
	errorMessage := DockerNonZeroExitCodeError(1)
	expectedErrorMessage := "docker command has returned a non-zero exit code:1"
	assert.EqualError(t, errorMessage, expectedErrorMessage)
}
