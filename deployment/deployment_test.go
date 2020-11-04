package deployment

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestAppConfig(t *testing.T) {
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

	config, err := AppConfig(api)
	assert.NoError(t, err)
	assert.Equal(t, config.ManualReleaseNames, false)
	assert.Equal(t, config.SmtpConfigured, true)
	assert.Equal(t, config.BaseDomain, "local.astronomer.io")
}

func TestAppConfigError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	_, err := AppConfig(api)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestCheckManualReleaseNamesTrue(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"manualReleaseNames": true
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

	assert.True(t, checkManualReleaseNames(api))
}

func TestCheckManualReleaseNamesFalse(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
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

	assert.False(t, checkManualReleaseNames(api))
}

func TestCheckManualReleaseNamesError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	assert.False(t, checkManualReleaseNames(api))
}

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "createDeployment": {
			"id": "ckbv818oa00r107606ywhoqtw",
			"executor": "CeleryExecutor",
			"urls": [
        {
          "type": "airflow",
          "url": "https://deployments.local.astronomer.io/boreal-penumbra-1102/airflow"
        },
        {
          "type": "flower",
          "url": "https://deployments.local.astronomer.io/boreal-penumbra-1102/flower"
        }
      ],
      "properties": {
        "component_version": "0.0.0",
        "alert_emails": []
      },
      "description": "",
      "label": "test2",
      "releaseName": "boreal-penumbra-1102",
      "status": null,
      "type": "airflow",
      "version": "0.0.0",
      "workspace": {
        "id": "ckbv7zvb100pe0760xp98qnh9",
        "label": "w1"
      },
      "createdAt": "2020-06-25T20:10:33.898Z",
      "updatedAt": "2020-06-25T20:10:33.898Z"
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
	label := "label"
	ws := "ck1qg6whg001r08691y117hub"
	releaseName := ""
	role := "test-role"
	executor := "CeleryExecutor"
	airflowVersion := "1.10.5"
	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, executor, airflowVersion, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
}

func TestCreateHoustonError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	label := "label"
	ws := "ck1qg6whg001r08691y117hub"
	releaseName := ""
	role := "test-role"
	executor := "CeleryExecutor"
	airflowVersion := "1.10.5"
	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, executor, airflowVersion, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data":{"deleteDeployment":{"id":"ckbv818oa00r107606ywhoqtw"}}}
`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	deploymentId := "ckbv818oa00r107606ywhoqtw"

	buf := new(bytes.Buffer)
	err := Delete(deploymentId, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully deleted deployment")
}

func TestList(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "workspaceDeployments": [
      {
        "id": "ckbv801t300qh0760pck7ea0c",
        "label": "test",
        "deployInfo": {
          "current": null
        },
        "releaseName": "burning-terrestrial-5940",
        "workspace": {
          "id": "ckbv7zvb100pe0760xp98qnh9",
          "label": "w1"
				},
				"executor": "CeleryExecutor"
      }
    ]
  }
}
`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	ws := "ckbv818oa00r107606ywhoqtw"

	buf := new(bytes.Buffer)
	err := List(ws, false, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG     AIRFLOW VERSION     
 test     burning-terrestrial-5940     v         ckbv801t300qh0760pck7ea0c     ?                           
`
	assert.Equal(t, buf.String(), expected)
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "updateDeployment": {
			"id": "ckbv801t300qh0760pck7ea0c",
			"executor": "CeleryExecutor",
			"urls": [
        {
          "type": "airflow",
          "url": "https://deployments.local.astronomer.io/burning-terrestrial-5940/airflow"
        },
        {
          "type": "flower",
          "url": "https://deployments.local.astronomer.io/burning-terrestrial-5940/flower"
        }
      ],
      "properties": {
        "component_version": "0.0.0",
        "alert_emails": []
      },
      "description": "",
      "label": "test123",
      "releaseName": "burning-terrestrial-5940",
      "status": null,
      "type": "airflow",
      "version": "0.0.0",
      "workspace": {
        "id": "ckbv7zvb100pe0760xp98qnh9"
      },
      "createdAt": "2020-06-25T20:09:38.341Z",
      "updatedAt": "2020-06-25T20:54:15.592Z"
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
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	deploymentConfig := make(map[string]string)
	deploymentConfig["executor"] = "CeleryExecutor"

	buf := new(bytes.Buffer)
	err := Update(id, role, deploymentConfig, api, buf)
	assert.NoError(t, err)
	expected := ` NAME        DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG     AIRFLOW VERSION     
 test123     burning-terrestrial-5940     0.0.0     ckbv801t300qh0760pck7ea0c             %!s(MISSING)

 Successfully updated deployment
`
	assert.Equal(t, buf.String(), expected)
}

func TestUpdateError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	id := "ck1qg6whg001r08691y117hub"
	role := "test-role"
	deploymentConfig := make(map[string]string)
	deploymentConfig["executor"] = "CeleryExecutor"

	buf := new(bytes.Buffer)
	err := Update(id, role, deploymentConfig, api, buf)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestAirflowUpgrade(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data": {
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
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	deploymentId := "ckbv818oa00r107606ywhoqtw"
	desiredAirflowVersion := "1.10.10"

	buf := new(bytes.Buffer)
	err := AirflowUpgrade(deploymentId, desiredAirflowVersion, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     DEPLOYMENT NAME     ASTRO     DEPLOYMENT ID                 AIRFLOW VERSION     
 test                         v         ckggzqj5f4157qtc9lescmehm     1.10.5              

The upgrade from Airflow 1.10.5 to 1.10.10 has been started.To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment airflow upgrade --cancel

`

	assert.Equal(t, buf.String(), expected)
}

func TestAirflowUpgradeError(t *testing.T) {
	testUtil.InitTestConfig()
	response := ``
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(response)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	deploymentId := "ckbv818oa00r107606ywhoqtw"
	desiredAirflowVersion := "1.10.10"

	buf := new(bytes.Buffer)
	err := AirflowUpgrade(deploymentId, desiredAirflowVersion, api, buf)
	assert.Error(t, err, "API error (500):")
}

func TestAirflowUpgradeCancel(t *testing.T) {
	testUtil.InitTestConfig()
	deploymentId := "ckggzqj5f4157qtc9lescmehm"

	okResponse := `{"data": {
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
	buf := new(bytes.Buffer)
	err := AirflowUpgradeCancel(deploymentId, api, buf)
	assert.NoError(t, err)
	expected := `
Airflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 1.10.5.
`
	assert.Equal(t, buf.String(), expected)
}

func TestAirflowUpgradeCancelError(t *testing.T) {
	testUtil.InitTestConfig()
	deploymentId := "ckggzqj5f4157qtc9lescmehm"

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(strings.NewReader(``)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	buf := new(bytes.Buffer)
	err := AirflowUpgradeCancel(deploymentId, api, buf)
	assert.Error(t, err, "API error (500):")
}
func Test_getDeployment(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{"data": {
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
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	deploymentId := "ckbv818oa00r107606ywhoqtw"

	deployment, err := getDeployment(deploymentId, api)
	assert.NoError(t, err)
	assert.Equal(t, deployment, &houston.Deployment{Id: "ckggzqj5f4157qtc9lescmehm", Label: "test", AirflowVersion: "1.10.5", DesiredAirflowVersion: "1.10.10"})
}

func Test_getDeploymentError(t *testing.T) {
	testUtil.InitTestConfig()
	response := ``
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(response)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	deploymentId := "ckbv818oa00r107606ywhoqtw"

	_, err := getDeployment(deploymentId, api)
	assert.Error(t, err, "tes")
}

func Test_getAirflowVersionSelection(t *testing.T) {
	deploymentId := "ckggzqj5f4157qtc9lescmehm"
	testUtil.InitTestConfig()
	okResponse := `{"data": {
					"deployment": {
						"id": "ckggzqj5f4157qtc9lescmehm",
						"label": "test",
						"airflowVersion": "1.10.7",
						"desiredAirflowVersion": "1.10.10"
					},
					"deploymentConfig": {
							"airflowVersions": [
							  "1.10.7",
							  "1.10.10",
							  "1.10.12"
							]
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
	buf := new(bytes.Buffer)

	// mock os.Stdin
	input := []byte("3")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	airflowVersion, err := getAirflowVersionSelection(deploymentId, api, buf)
	assert.NoError(t, err)
	assert.Equal(t, airflowVersion, "1.10.12")
}

func Test_getAirflowVersionSelectionError(t *testing.T) {
	deploymentId := "ckggzqj5f4157qtc9lescmehm"
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(``)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	buf := new(bytes.Buffer)
	airflowVersion, err := getAirflowVersionSelection(deploymentId, api, buf)
	assert.Error(t, err, "API error (500):")
	assert.Equal(t, airflowVersion, "")
}
