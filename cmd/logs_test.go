package cmd

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestDeploymentLogsRootCommandTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
		  "appConfig": {"triggererEnabled": true, "featureFlags": { "triggererEnabled": true}}
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
	output, err := executeCommandC(api, "deployment", "logs")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment logs")
	assert.Contains(t, output, "triggerer")
}

func TestDeploymentLogsRootCommandTriggererDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
		  "appConfig": {"triggererEnabled": false, "featureFlags": { "triggererEnabled": false}}
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
	output, err := executeCommandC(api, "deployment", "logs")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment logs")
	assert.NotContains(t, output, "triggerer")
}
