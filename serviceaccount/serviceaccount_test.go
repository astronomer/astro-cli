package deployment

import (
	"bytes"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestCreateUsingDeploymentUUID(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `
{
  "data": {
    "createDeploymentServiceAccount": {
      "id": "ckbvcbqs1014t0760u4bszmcs",
      "label": "test",
      "apiKey": "60f2f4f3fa006e3e135dbe99b1391d84",
      "entityType": "DEPLOYMENT",
      "deploymentUuid": null,
      "category": "test",
      "active": true,
      "lastUsedAt": null,
      "createdAt": "2020-06-25T22:10:42.385Z",
      "updatedAt": "2020-06-25T22:10:42.385Z"
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

	deploymentUuid := "ck1qg6whg001r08691y117hub"
	label, category, role := "test", "test", "test"
	buf := new(bytes.Buffer)
	err := CreateUsingDeploymentUUID(deploymentUuid, label, category, role, api, buf)
	assert.NoError(t, err)
	expectedOut := ` NAME     CATEGORY     ID                            APIKEY
 test     test         ckbvcbqs1014t0760u4bszmcs     60f2f4f3fa006e3e135dbe99b1391d84

 Service account successfully created.
`
	assert.Equal(t, buf.String(), expectedOut)
}

func TestCreateUsingWorkspaceUUID(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `
{
  "data": {
    "createDeploymentServiceAccount": {
      "id": "ckbvcbqs1014t0760u4bszmcs",
      "label": "test",
      "apiKey": "60f2f4f3fa006e3e135dbe99b1391d84",
      "entityType": "DEPLOYMENT",
      "deploymentUuid": null,
      "category": "test",
      "active": true,
      "lastUsedAt": null,
      "createdAt": "2020-06-25T22:10:42.385Z",
      "updatedAt": "2020-06-25T22:10:42.385Z"
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

	workspaceUuid := "ck1qg6whg001r08691y117hub"
	label, category, role := "test", "test", "test"
	buf := new(bytes.Buffer)
	err := CreateUsingDeploymentUUID(workspaceUuid, label, category, role, api, buf)
	assert.NoError(t, err)
	expectedOut := ` NAME     CATEGORY     ID                            APIKEY
 test     test         ckbvcbqs1014t0760u4bszmcs     60f2f4f3fa006e3e135dbe99b1391d84

 Service account successfully created.
`
	assert.Equal(t, buf.String(), expectedOut)
}

func TestDeleteUsingWorkspaceUUID(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `
{
  "data": {
    "deleteWorkspaceServiceAccount": {
      "id": "ckbvcbqs1014t0760u4bszmcs"
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
	serviceAccountId := "ckbvcbqs1014t0760u4bszmcs"
	workspaceUuid := "ck1qg6whg001r08691y117hub"
	buf := new(bytes.Buffer)
	err := DeleteUsingWorkspaceUUID(serviceAccountId, workspaceUuid, api, buf)
	assert.NoError(t, err)
	expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
	assert.Equal(t, buf.String(), expectedOut)
}

func TestDeleteUsingDeploymentUUID(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `
{
  "data": {
    "deleteDeploymentServiceAccount": {
      "id": "ckbvcbqs1014t0760u4bszmcs"
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
	serviceAccountId := "ckbvcbqs1014t0760u4bszmcs"
	deploymentUuid := "ck1qg6whg001r08691y117hub"
	buf := new(bytes.Buffer)
	err := DeleteUsingDeploymentUUID(serviceAccountId, deploymentUuid, api, buf)
	assert.NoError(t, err)
	expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
	assert.Equal(t, buf.String(), expectedOut)
}
