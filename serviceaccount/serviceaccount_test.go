package deployment

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
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
	api := houston.NewClient(client)

	deploymentUUID := "ck1qg6whg001r08691y117hub"
	label, category, role := "test", "test", "test"
	buf := new(bytes.Buffer)
	err := CreateUsingDeploymentUUID(deploymentUUID, label, category, role, api, buf)
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
	api := houston.NewClient(client)

	workspaceUUID := "ck1qg6whg001r08691y117hub"
	label, category, role := "test", "test", "test"
	buf := new(bytes.Buffer)
	err := CreateUsingDeploymentUUID(workspaceUUID, label, category, role, api, buf)
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
	api := houston.NewClient(client)
	serviceAccountID := "ckbvcbqs1014t0760u4bszmcs"
	workspaceUUID := "ck1qg6whg001r08691y117hub"
	buf := new(bytes.Buffer)
	err := DeleteUsingWorkspaceUUID(serviceAccountID, workspaceUUID, api, buf)
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
	api := houston.NewClient(client)
	serviceAccountID := "ckbvcbqs1014t0760u4bszmcs"
	deploymentUUID := "ck1qg6whg001r08691y117hub"
	buf := new(bytes.Buffer)
	err := DeleteUsingDeploymentUUID(serviceAccountID, deploymentUUID, api, buf)
	assert.NoError(t, err)
	expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
	assert.Equal(t, buf.String(), expectedOut)
}

func TestGetDeploymentServiceAccount(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `
{
  "data": {
    "deploymentServiceAccounts": [
      {
        "id": "ckqvfa2cu1468rn9hnr0bqqfk",
        "apiKey": "658b304f36eaaf19860a6d9eb73f7d8a",
        "label": "yooo can u see me test",
        "category": "",
        "entityType": "DEPLOYMENT",
        "entityUuid": null,
        "active": true,
        "createdAt": "2021-07-08T21:28:57.966Z",
        "updatedAt": "2021-07-08T21:28:57.967Z",
        "lastUsedAt": null
      }
    ]
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
	deploymentUUID := "ckqvf9spa1189rn9hbh5h439u"
	buf := new(bytes.Buffer)
	err := GetDeploymentServiceAccounts(deploymentUUID, api, buf)
	assert.NoError(t, err)
	expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
	assert.Contains(t, buf.String(), expectedOut)
}

func TestGetWorkspaceServiceAccount(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `
{
  "data": {
    "workspaceServiceAccounts": [
      {
        "id": "ckqvfa2cu1468rn9hnr0bqqfk",
        "apiKey": "658b304f36eaaf19860a6d9eb73f7d8a",
        "label": "yooo can u see me test",
        "category": "",
        "entityType": "DEPLOYMENT",
        "entityUuid": null,
        "active": true,
        "createdAt": "2021-07-08T21:28:57.966Z",
        "updatedAt": "2021-07-08T21:28:57.967Z",
        "lastUsedAt": null
      }
    ]
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
	deploymentUUID := "ckqvf9spa1189rn9hbh5h439u"
	buf := new(bytes.Buffer)
	err := GetWorkspaceServiceAccounts(deploymentUUID, api, buf)
	assert.NoError(t, err)
	expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
	assert.Contains(t, buf.String(), expectedOut)
}
