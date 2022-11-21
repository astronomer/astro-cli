package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPypiVersionInvalidHostFailure(t *testing.T) {
	_, err := GetPypiVersion("http://abcd")
	expectedErrContains := "error getting latest release version for project url http://abcd,  Get \"http://abcd\": dial tcp: lookup abcd"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetBaseDockerImageURIInvalidURLFailure(t *testing.T) {
	_, err := GetBaseDockerImageURI("http://efgh")
	expectedErrContains := "error retrieving the latest configuration http://efgh,  Get \"http://efgh\": dial tcp: lookup efgh: no such host. Using the default"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetBaseDockerImageURIInvalidJSONFailure(t *testing.T) {
	_, err := GetBaseDockerImageURI("https://github.com/astronomer/astro-sdk/blob/main/README.md")
	expectedErrContains := "error parsing the base docker image from the configuration file: invalid character '<' looking for beginning of value"
	assert.ErrorContains(t, err, expectedErrContains)
}
