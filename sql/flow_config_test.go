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

func TestGetPypiVersionInvalidJSONFailure(t *testing.T) {
	_, err := GetPypiVersion("https://pypi.org")
	expectedErrContains := "error parsing response for project version invalid character '<' looking for beginning of value"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPypiVersionInvalidHTTPRequestFailure(t *testing.T) {
	_, err := GetPypiVersion("gs://pyconuk-workshop/")
	expectedErrContains := "error getting latest release version for project url gs://pyconuk-workshop/"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetBaseDockerImageURIInvalidURLFailure(t *testing.T) {
	_, err := GetBaseDockerImageURI("http://efgh")
	expectedErrContains := "error retrieving the latest configuration http://efgh,  Get \"http://efgh\": dial tcp: lookup efgh"
	assert.ErrorContains(t, err, expectedErrContains)
	expectedErrContains = "no such host. Using the default"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetBaseDockerImageURIInvalidJSONFailure(t *testing.T) {
	_, err := GetBaseDockerImageURI("https://github.com/astronomer/astro-sdk/blob/main/README.md")
	expectedErrContains := "error parsing the base docker image from the configuration file: invalid character '<' looking for beginning of value"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetBaseDockerImageURIInvalidHTTPRequestFailure(t *testing.T) {
	_, err := GetBaseDockerImageURI("gs://pyconuk-workshop/")
	expectedErrContains := "error retrieving the latest configuration gs://pyconuk-workshop/"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPythonSDKComptabilityInvalidURLFailure(t *testing.T) {
	_, _, err := GetPythonSDKComptability("http://efgh", "")
	expectedErrContains := "error retrieving the latest configuration http://efgh,  Get \"http://efgh\": dial tcp: lookup efgh"
	assert.ErrorContains(t, err, expectedErrContains)
	expectedErrContains = "no such host. Using the default"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPythonSDKComptabilityInvalidJSONFailure(t *testing.T) {
	_, _, err := GetPythonSDKComptability("https://github.com/astronomer/astro-sdk/blob/main/README.md", "")
	expectedErrContains := "error parsing the compatibility versions from the configuration file: invalid character '<' looking for beginning of value"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPythonSDKComptabilityHTTPRequestFailure(t *testing.T) {
	_, _, err := GetPythonSDKComptability("gs://pyconuk-workshop/", "")
	expectedErrContains := "error retrieving the latest configuration gs://pyconuk-workshop/"
	assert.ErrorContains(t, err, expectedErrContains)
}
