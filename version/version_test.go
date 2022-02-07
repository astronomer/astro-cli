package version

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidVersionInValid(t *testing.T) {
	assert.False(t, isValidVersion(""))
}

func TestIsValidVersionValid(t *testing.T) {
	assert.True(t, isValidVersion("0.0.1"))
}

func TestPrintVersionHoustonError(t *testing.T) {
	testUtil.InitTestConfig()
	badResponse := "Internal Server Error"
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(badResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewClient(client)
	output := new(bytes.Buffer)
	CurrVersion = "0.15.0"
	err := PrintVersion(api, output)
	assert.NoError(t, err)
	assert.Equal(t, output.String(), "Astro CLI Version: 0.15.0, Git Commit: \nAstro Server Version: Please authenticate to a cluster to see server version\n")
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
	api := houston.NewClient(client)
	output := new(bytes.Buffer)
	CurrVersion = ""
	err := PrintVersion(api, output)
	if assert.Error(t, err, "An error was expected") {
		assert.EqualError(t, err, messages.ErrInvalidCLIVersion)
	}
}

func TestCheckForUpdateVersionMatch(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.13.0",
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
	houstonClient := houston.NewClient(client)

	CurrVersion = "0.15.0"
	okGitHubResponse := `{
		"url": "https://api.github.com/repos/astronomer/astro-cli/releases/27102579",
		"tag_name": "v0.15.0",
		"draft": false,
		"prerelease": false,
		"published_at": "2020-06-01T16:27:05Z"
	}`
	gitHubClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okGitHubResponse)),
			Header:     make(http.Header),
		}
	})
	githubClient := github.NewGithubClient(gitHubClient)
	output := new(strings.Builder)
	CheckForUpdate(houstonClient, githubClient, output)
	expected := "Astro CLI Version: v0.15.0 (2020.06.01)\nAstro CLI Latest: v0.15.0 (2020.06.01)\nAstro Server Version: 0.13.0\n"
	actual := output.String()

	assert.Equal(t, expected, actual)
}

func TestPrintServerVersion(t *testing.T) {
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
	houstonClient := houston.NewClient(client)

	output := new(strings.Builder)
	printServerVersion(houstonClient, output)
	expected := "Astro Server Version: 0.18.0\n"
	actual := output.String()

	assert.Equal(t, expected, actual)
}
