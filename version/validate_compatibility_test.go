package version

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestValidateCompatibilityVersionsMatched(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.15.1"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	// check that there is no output because version matched
	assert.Equal(t, output, &bytes.Buffer{})

	output = new(bytes.Buffer)
	cliVer = ""
	err = ValidateCompatibility(api, output, cliVer, false)

	assert.NoError(t, err)
	// check that there is no output because version matched
	assert.Equal(t, output, &bytes.Buffer{})
}

func TestValidateCompatibilityMissingCliVersion(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := ""
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	// check that there is no output because cli version is missing
	assert.Equal(t, output, &bytes.Buffer{})
}

func TestValidateCompatibilityVersionsCliDowngrade(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.1"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	expected := "Your Astro CLI Version (0.17.1) is ahead of the server version (0.15.1).\nConsider downgrading your Astro CLI to match. See https://www.astronomer.io/docs/cli-quickstart for more information.\n"
	// check that user can see correct message
	assert.Equal(t, expected, output.String())
}

func TestValidateCompatibilityVersionsCliUpgrade(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "1.0.0",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.1"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.Error(t, err)
	expected := "There is an update for Astro CLI. You're using version 0.17.1, but 1.0.0 is the server version.\nPlease upgrade to the matching version before continuing. See https://www.astronomer.io/docs/cli-quickstart for more information.\nTo skip this check use the --skip-version-check flag.\n"
	// check that user can see correct message
	assert.EqualError(t, err, expected)
}

func TestValidateCompatibilityVersionBypass(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "1.0.0",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.1"
	err := ValidateCompatibility(api, output, cliVer, true)
	expected := ""

	assert.NoError(t, err)
	// check that user can bypass major version check
	assert.Equal(t, expected, output.String())
}

func TestValidateCompatibilityVersionsMinorWarning(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.18.0",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.0"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	expected := "A new minor version of Astro CLI is available. Your version is 0.17.0 and 0.18.0 is the latest.\nSee https://www.astronomer.io/docs/cli-quickstart for more information.\n"
	// check that user can see correct warning message
	assert.Equal(t, expected, output.String())
}

func TestValidateCompatibilityVersionsPatchWarning(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.17.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.0"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	expected := "A new patch for Astro CLI is available. Your version is 0.17.0 and 0.17.1 is the latest.\nSee https://www.astronomer.io/docs/cli-quickstart for more information.\n"
	// check that user can see correct warning message
	assert.Equal(t, expected, output.String())
}

func TestCompareVersionsInvalidServerVer(t *testing.T) {
	output := new(bytes.Buffer)
	err := compareVersions("INVALID VERSION", "0.17.1", output)
	assert.Error(t, err)
}

func TestCompareVersionsInvalidCliVer(t *testing.T) {
	output := new(bytes.Buffer)
	err := compareVersions("0.17.1", "INVALID VERSION", output)
	assert.Error(t, err)
}

func TestParseVersion(t *testing.T) {
	ver, err := parseVersion("0.17.1")
	assert.NoError(t, err)
	if assert.NotNil(t, ver) {
		assert.Equal(t, uint64(0), ver.Major())
		assert.Equal(t, uint64(17), ver.Minor())
		assert.Equal(t, uint64(1), ver.Patch())
	}
}
