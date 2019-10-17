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

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deployment")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment")
}

func TestDeploymentSaRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deployment", "service-account")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment service-account")
}

func TestDeploymentSaDeleteWoKeyIdCommand(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("deployment", "service-account", "delete", "--deployment-id=1234")
	assert.Error(t, err)
	assert.EqualError(t, err, "accepts 1 arg(s), received 0")
}

func TestDeploymentSaDeleteWoDeploymentIdCommand(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("deployment", "service-account", "delete", "key-test-id")
	assert.Error(t, err)
	assert.EqualError(t, err, `required flag(s) "deployment-id" not set`)
}

func TestDeploymentSaDeleteRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "deleteDeploymentServiceAccount": {
      "id": "q1w2e3r4t5y6u7i8o9p0",
      "apiKey": "000000000000000000000000",
      "label": "my_label",
      "category": "default",
      "entityType": "DEPLOYMENT",
      "entityUuid": null,
      "active": true,
      "createdAt": "2019-10-16T21:14:22.105Z",
      "updatedAt": "2019-10-16T21:14:22.105Z",
      "lastUsedAt": null
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

	_, output, err := executeCommandC(api, "deployment", "service-account", "delete", "q1w2e3r4t5y6u7i8o9p0", "--deployment-id=1234")
	assert.NoError(t, err)
	assert.Contains(t, output, "Service Account my_label (q1w2e3r4t5y6u7i8o9p0) successfully deleted")
}

func TestDeploymentSaGetCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` NAME         CATEGORY     ID                       APIKEY                       
 my_label     default      q1w2e3r4t5y6u7i8o9p0     000000000000000000000000     
`
	okResponse := `{
  "data": {
    "serviceAccounts": [{
      "id": "q1w2e3r4t5y6u7i8o9p0",
      "apiKey": "000000000000000000000000",
      "label": "my_label",
      "category": "default",
      "entityType": "DEPLOYMENT",
      "entityUuid": null,
      "active": true,
      "createdAt": "2019-10-16T21:14:22.105Z",
      "updatedAt": "2019-10-16T21:14:22.105Z",
      "lastUsedAt": null
    }]
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

	_, output, err := executeCommandC(api, "deployment", "service-account", "get", "q1w2e3r4t5y6u7i8o9p0")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentSaCreateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` NAME         CATEGORY     ID                       APIKEY                       
 my_label     default      q1w2e3r4t5y6u7i8o9p0     000000000000000000000000     

 Service account successfully created.
`
	okResponse := `{
  "data": {
    "createDeploymentServiceAccount": {
      "id": "q1w2e3r4t5y6u7i8o9p0",
      "apiKey": "000000000000000000000000",
      "label": "my_label",
      "category": "default",
      "entityType": "DEPLOYMENT",
      "entityUuid": null,
      "active": true,
      "createdAt": "2019-10-16T21:14:22.105Z",
      "updatedAt": "2019-10-16T21:14:22.105Z",
      "lastUsedAt": null
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

	_, output, err := executeCommandC(api, "deployment", "service-account", "create", "--deployment-id=ck1qg6whg001r08691y117hub", "--label=my_label", "--role=viewer")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}
