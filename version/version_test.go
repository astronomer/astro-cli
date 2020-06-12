package version

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
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
