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
      "config": {
        "executor": "CeleryExecutor"
      },
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
	deploymentConfig := make(map[string]string)
	deploymentConfig["executor"] = "CeleryExecutor"

	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, deploymentConfig, api, buf)
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
	deploymentConfig := make(map[string]string)
	deploymentConfig["executor"] = "CeleryExecutor"

	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, deploymentConfig, api, buf)
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
        "config": {
          "executor": "CeleryExecutor"
        }
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
	expected :=` NAME     DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG     
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
      "config": {
        "executor": "CeleryExecutor"
      },
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
	expected :=` NAME        DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG      
 test123     burning-terrestrial-5940     0.0.0     ckbv801t300qh0760pck7ea0c     %!s(MISSING)

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