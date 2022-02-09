package version

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

var (
	errMock       = errors.New("api error") //nolint:goerr113
	mockAppConfig = &houston.AppConfig{
		Version:              "0.19.1",
		BaseDomain:           "local.astronomer.io",
		SMTPConfigured:       true,
		ManualNamespaceNames: false,
	}
)

func TestIsValidVersionInValid(t *testing.T) {
	assert.False(t, isValidVersion(""))
}

func TestIsValidVersionValid(t *testing.T) {
	assert.True(t, isValidVersion("0.0.1"))
}

func TestPrintVersionHoustonError(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(nil, errMock)
	output := new(bytes.Buffer)
	CurrVersion = "0.15.0"
	err := PrintVersion(api, output)
	assert.NoError(t, err)
	assert.Equal(t, output.String(), "Astro CLI Version: 0.15.0, Git Commit: \nAstro Server Version: Please authenticate to a cluster to see server version\n")
	api.AssertExpectations(t)
}

func TestPrintVersionError(t *testing.T) {
	testUtil.InitTestConfig()

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)

	output := new(bytes.Buffer)
	CurrVersion = ""
	err := PrintVersion(api, output)
	if assert.Error(t, err, "An error was expected") {
		assert.EqualError(t, err, messages.ErrInvalidCLIVersion)
	}
}

func TestCheckForUpdateVersionMatch(t *testing.T) {
	testUtil.InitTestConfig()
	houstonClient := new(mocks.ClientInterface)
	houstonClient.On("GetAppConfig").Return(mockAppConfig, nil)

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
	expected := "Astro CLI Version: v0.15.0 (2020.06.01)\nAstro CLI Latest: v0.15.0 (2020.06.01)\nAstro Server Version: 0.19.1\n"
	actual := output.String()

	assert.Equal(t, expected, actual)
	houstonClient.AssertExpectations(t)
}

func TestPrintServerVersion(t *testing.T) {
	testUtil.InitTestConfig()
	houstonClient := new(mocks.ClientInterface)
	houstonClient.On("GetAppConfig").Return(mockAppConfig, nil)

	output := new(strings.Builder)
	printServerVersion(houstonClient, output)
	expected := "Astro Server Version: 0.19.1\n"
	actual := output.String()

	assert.Equal(t, expected, actual)
	houstonClient.AssertExpectations(t)
}
