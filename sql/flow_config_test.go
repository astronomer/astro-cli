package sql

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPypiVersionInvalidHostFailure(t *testing.T) {
	_, err := GetPypiVersion("http://abcd", "0.1")
	expectedErrContains := "error getting latest release version for project url http://abcd,  Get \"http://abcd\": dial tcp: lookup abcd"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPypiVersionInvalidJSONFailure(t *testing.T) {
	_, err := GetPypiVersion("https://pypi.org", "0.1")
	expectedErrContains := "error parsing response for project version invalid character '<' looking for beginning of value"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPypiVersionInvalidHTTPRequestFailure(t *testing.T) {
	_, err := GetPypiVersion("gs://pyconuk-workshop/", "0.1")
	expectedErrContains := "error getting latest release version for project url gs://pyconuk-workshop/"
	assert.ErrorContains(t, err, expectedErrContains)
}

func TestGetPypiVersionInvalidAstroCliVersionExactMatchSuccess(t *testing.T) {
	_, err := GetPypiVersion("https://raw.githubusercontent.com/astronomer/astro-sdk/1673-minor-version-match/sql-cli/config/astro-cli.json", "1.9.0")
	assert.NoError(t, err)
}

func TestGetPypiVersionInvalidAstroCliVersionMinorVersionMatchSuccess(t *testing.T) {
	_, err := GetPypiVersion("https://raw.githubusercontent.com/astronomer/astro-sdk/1673-minor-version-match/sql-cli/config/astro-cli.json", "1.10.1")
	assert.NoError(t, err)
}

func TestGetPypiVersionInvalidAstroCliVersionDefaultMatchSuccess(t *testing.T) {
	_, err := GetPypiVersion("https://raw.githubusercontent.com/astronomer/astro-sdk/1673-minor-version-match/sql-cli/config/astro-cli.json", "x.y.z")
	assert.NoError(t, err)
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

func TestGetPythonSDKCompatabilitySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"baseDockerImage": "quay.io/astronomer/astro-runtime:6.0.4-base",
			"compatibility": {
			  "0.1": {
				"astroRuntimeVersion": ">=8.0.0, <8.1.0",
				"astroSDKPythonVersion": ">=1.3.0, <1.5"
			  }
			}
		}`))
	}))
	astroRuntime, astroSdkPython, err := GetPythonSDKComptability(server.URL, "0.1")
	assert.NoError(t, err)
	assert.Equal(t, ">=8.0.0, <8.1.0", astroRuntime)
	assert.Equal(t, ">=1.3.0, <1.5", astroSdkPython)
}
