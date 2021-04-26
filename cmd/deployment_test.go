package cmd

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
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

func TestDeploymentCreateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"nfsMountDagDeployment": false},
    "createDeployment": {
      "airflowVersion": "2.0.0",
      "config": {
        "dagDeployment": {
          "nfsLocation": "",
          "type": "image"
        },
        "executor": "CeleryExecutor"
      },
      "createdAt": "2021-04-26T20:03:36.262Z",
      "dagDeployment": {
        "nfsLocation": "",
        "type": "image"
      },
      "description": "",
      "desiredAirflowVersion": "2.0.0",
      "id": "cknz133ra49758zr9w34b87ua",
      "label": "test",
      "properties": {
        "alert_emails": [
          "andrii@astronomer.io"
        ],
        "component_version": "2.0.0"
      },
      "releaseName": "accurate-radioactivity-8677",
      "status": null,
      "type": "airflow",
      "updatedAt": "2021-04-26T20:03:36.262Z",
      "urls": [
        {
          "type": "airflow",
          "url": "https://deployments.local.astronomer.io/accurate-radioactivity-8677/airflow"
        },
        {
          "type": "flower",
          "url": "https://deployments.local.astronomer.io/accurate-radioactivity-8677/flower"
        }
      ],
      "version": "0.15.6",
      "workspace": {
        "id": "ckn4phn1k0104v5xtrer5lpli",
        "label": "w1"
      }
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

	_, output, err := executeCommandC(api, "deployment", "create", "new-deployment-name", "--executor=celery")
	assert.NoError(t, err)
	assert.Contains(t, output, "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
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
    "appConfig": {"nfsMountDagDeployment": false},
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
    "appConfig": {"nfsMountDagDeployment": false},
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

	_, output, err := executeCommandC(api, "deployment", "service-account", "get", "--deployment-id=q1w2e3r4t5y6u7i8o9p0")
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
    "appConfig": {"nfsMountDagDeployment": false},
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
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "deployment", "service-account", "create", "--deployment-id=ck1qg6whg001r08691y117hub", "--label=my_label", "--role=viewer")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentUserAddCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` DEPLOYMENT NAME              DEPLOYMENT ID                 USER                        ROLE                  
 prehistoric-gravity-9229     ckggvxkw112212kc9ebv8vu6p     somebody@astronomer.com     DEPLOYMENT_VIEWER     

 Successfully added somebody@astronomer.com as a DEPLOYMENT_VIEWER
`
	okResponse := `{
		"data": {
		        "appConfig": {"nfsMountDagDeployment": false},
			"deploymentAddUserRole": {
				"id": "ckggzqj5f4157qtc9lescmehm",
				"user": {
					"username": "somebody@astronomer.com"
				},
				"role": "DEPLOYMENT_VIEWER",
				"deployment": {
					"id": "ckggvxkw112212kc9ebv8vu6p",
					"releaseName": "prehistoric-gravity-9229"
				}
			}
		}
	}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "deployment", "user", "add", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "somebody@astronomer.com")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentUserDeleteCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` DEPLOYMENT ID                 USER                        ROLE                  
 ckggvxkw112212kc9ebv8vu6p     somebody@astronomer.com     DEPLOYMENT_VIEWER     

 Successfully removed the DEPLOYMENT_VIEWER role for somebody@astronomer.com from deployment ckggvxkw112212kc9ebv8vu6p
`
	okResponse := `{
		"data": {
		        "appConfig": {"nfsMountDagDeployment": false},
			"deploymentRemoveUserRole": {
				"id": "ckggzqj5f4157qtc9lescmehm",
				"user": {
					"username": "somebody@astronomer.com"
				},
				"role": "DEPLOYMENT_VIEWER",
				"deployment": {
					"id": "ckggvxkw112212kc9ebv8vu6p",
					"releaseName": "prehistoric-gravity-9229"
				}
			}
		}
	}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "deployment", "user", "delete", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "somebody@astronomer.com")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}

func TestDeploymentUserUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Successfully updated somebody@astronomer.com to a DEPLOYMENT_ADMIN`
	okResponse := `{
		"data": {
			"appConfig": {"nfsMountDagDeployment": false},
			"deploymentUpdateUserRole": {
				"id": "ckggzqj5f4157qtc9lescmehm",
				"user": {
					"username": "somebody@astronomer.com"
				},
				"role": "DEPLOYMENT_ADMIN",
				"deployment": {
					"id": "ckggvxkw112212kc9ebv8vu6p",
					"releaseName": "prehistoric-gravity-9229"
				}
			}
		}
	}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "deployment", "user", "update", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "--role=DEPLOYMENT_ADMIN", "somebody@astronomer.com")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.`

	okResponse := `{"data": {
					"appConfig": {"nfsMountDagDeployment": false},
					"updateDeploymentAirflow": {
						"id": "ckggzqj5f4157qtc9lescmehm",
						"label": "test",
						"airflowVersion": "1.10.5",
						"desiredAirflowVersion": "1.10.10"
					}
				}
			}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "deployment", "airflow", "upgrade", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "--desired-airflow-version=1.10.10")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCancelCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Airflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 1.10.5.`

	okResponse := `{"data": {
                                        "appConfig": {"nfsMountDagDeployment": false},
					"deployment": {
						"id": "ckggzqj5f4157qtc9lescmehm",
						"label": "test",
						"airflowVersion": "1.10.5",
						"desiredAirflowVersion": "1.10.10"
					}
				}
			}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, output, err := executeCommandC(api, "deployment", "airflow", "upgrade", "--cancel", "--deployment-id=ckggvxkw112212kc9ebv8vu6p")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}
