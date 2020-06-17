package version

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestIsValidVersionInValid(t *testing.T) {
	assert.False(t, isValidVersion(""))
}

func TestIsValidVersionValid(t *testing.T) {
	assert.True(t, isValidVersion("0.0.1"))
}

func TestPrintVersionError(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.19.1",
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
	err := PrintVersion(api, output)
	if assert.Error(t, err, "An error was expected") {
		assert.EqualError(t, err, messages.ERROR_INVALID_CLI_VERSION)
	}
}

func TestCheckForUpdate(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.0",
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
	CurrVersion = "0.13.0"
	houston.NewHoustonClient(client)
	githubClient := github.NewGithubClient(client)
	output := new(bytes.Buffer)
	CheckForUpdate(githubClient, output)
	expected := "Astro CLI Version:   (0001.01.01)\nAstro CLI Latest:   (0001.01.01)\nYou are running the latest version.\n"
	actual := output.Bytes()
	actualOut := ""

	// TODO: Clean up this for loop
	for a := range actual {
		o, err := output.ReadString(actual[a])
		if err != nil {
			assert.Fail(t, err.Error())
		}
		actualOut += o
	}

	assert.Equal(t, expected, actualOut)
}
