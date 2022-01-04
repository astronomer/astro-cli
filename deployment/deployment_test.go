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

func TestAppVersion(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io"
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

	config, err := AppVersion(api)
	assert.NoError(t, err)
	assert.Equal(t, "0.15.1", config.Version)
	assert.Equal(t, "local.astronomer.io", config.BaseDomain)
}

func TestAppConfig(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false,
				"hardDeleteDeployment": false
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
	assert.Equal(t, false, config.ManualReleaseNames)
	assert.Equal(t, true, config.SMTPConfigured)
	assert.Equal(t, "local.astronomer.io", config.BaseDomain)
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
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false,
				"hardDeleteDeployment": true,
				"manualNamespaceNames": false
			},
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
	dagDeploymentType := "image"
	nfsLocation := ""
	triggerReplicas := 0
	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
}

func TestCreateTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false,
				"hardDeleteDeployment": true,
				"manualNamespaceNames": false,
				"triggererEnabled": true,
				"featureFlags": { "nfsMountDagDeployment": true}
			},
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
	dagDeploymentType := "image"
	nfsLocation := ""
	triggerReplicas := 1
	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
}

func TestCreateWithNFSLocation(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false,
				"hardDeleteDeployment": true,
				"manualNamespaceNames": false
			},
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
	dagDeploymentType := "volume"
	nfsLocation := "test:/test"
	triggerReplicas := 0
	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
}

func TestCreateWithGitSync(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false,
				"hardDeleteDeployment": true,
				"manualNamespaceNames": false
			},
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
	dagDeploymentType := "git_sync"
	triggerReplicas := 0
	myTests := []struct {
		repoURL              string
		revision             string
		dagDirectoryLocation string
		branchName           string
		syncInterval         int
		sshKey               string
		knownHosts           string
		expectedOutput       string
		expectedError        string
	}{
		{repoURL: "https://github.com/bote795/public-ariflow-dags-test.git", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../cmd/testfiles/ssh_key", knownHosts: "../cmd/testfiles/known_hosts", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../cmd/testfiles/ssh_key", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../cmd/testfiles/wrong_ssh_key", knownHosts: "../cmd/testfiles/known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "wrong path specified, no file exists for ssh key"},
		{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../cmd/testfiles/ssh_key", knownHosts: "../cmd/testfiles/wrong_known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "wrong path specified, no file exists for known hosts"},
		{repoURL: "https://gitlab.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../cmd/testfiles/ssh_key", knownHosts: "../cmd/testfiles/known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "git repository host not present in known hosts file"},
	}
	for _, tt := range myTests {
		buf := new(bytes.Buffer)
		err := Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, "", tt.repoURL, tt.revision, tt.branchName, tt.dagDirectoryLocation, tt.sshKey, tt.knownHosts, tt.syncInterval, triggerReplicas, api, buf)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, buf.String(), tt.expectedOutput)
	}
}

func TestCreateWithPreCreateNamespaceDeployment(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true,
      "manualNamespaceNames": true
    },
    "availableNamespaces": [
      {
        "name": "test1"
      },
      {
        "name": "test2"
      }
    ],
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
	dagDeploymentType := "volume"
	nfsLocation := "test:/test"
	triggerReplicas := 0
	buf := new(bytes.Buffer)

	// mock os.Stdin
	input := []byte("1")
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

	err = Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
}

func TestCreateWithPreCreateNamespaceDeploymentError(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true,
      "manualNamespaceNames": true,
      "featureFlags": {
        "manualReleaseNames": false,
        "configureDagDeployment": false,
        "nfsMountDagDeployment": false,
        "hardDeleteDeployment": false,
        "manualNamespaceNames": true,
        "sysAdminScalabilityImprovementsEnabled": false
      }
    },
    "availableNamespaces": [
      {
        "name": "test1"
      },
      {
        "name": "test2"
      }
    ],
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
	dagDeploymentType := "volume"
	nfsLocation := "test:/test"
	triggerReplicas := 0
	buf := new(bytes.Buffer)

	// mock os.Stdin
	input := []byte("5")
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

	err = Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, api, buf)
	assert.EqualError(t, err, "number is out of available range")
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
	dagDeploymentType := "image"
	nfsLocation := ""
	triggerReplicas := 0
	buf := new(bytes.Buffer)
	err := Create(label, ws, releaseName, role, executor, airflowVersion, dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, api, buf)
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
	deploymentID := "ckbv818oa00r107606ywhoqtw"

	buf := new(bytes.Buffer)
	err := Delete(deploymentID, false, api, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully deleted deployment")
}

func TestDeleteHard(t *testing.T) {
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
	deploymentID := "ckbv818oa00r107606ywhoqtw"

	buf := new(bytes.Buffer)
	err := Delete(deploymentID, true, api, buf)
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
	assert.Equal(t, expected, buf.String())
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
  "appConfig": {
    "version": "0.15.1",
     "baseDomain": "local.astronomer.io",
     "smtpConfigured": true, 
     "manualReleaseNames": false,
     "hardDeleteDeployment": true,
     "manualNamespaceNames": true,
     "triggererEnabled": false
	},
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
	  "airflowVersion": "2.2.2",
      "workspace": {
        "id": "ckbv7zvb100pe0760xp98qnh9"
      },
	  "deployInfo":{
		"current":"2.2.2-1"
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

	expected := ` NAME        DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG         AIRFLOW VERSION     
 test123     burning-terrestrial-5940     0.0.0     ckbv801t300qh0760pck7ea0c     2.2.2-1     2.2.2               

 Successfully updated deployment
`
	myTests := []struct {
		deploymentConfig  map[string]string
		dagDeploymentType string
		expectedOutput    string
	}{
		{deploymentConfig: map[string]string{"executor": "CeleryExecutor"}, dagDeploymentType: "", expectedOutput: expected},
		{deploymentConfig: map[string]string{"executor": "CeleryExecutor"}, dagDeploymentType: "image", expectedOutput: expected},
	}
	for _, tt := range myTests {
		buf := new(bytes.Buffer)
		err := Update(id, role, tt.deploymentConfig, tt.dagDeploymentType, "", "", "", "", "", "", "", "", 1, 0, api, buf)
		assert.NoError(t, err)
		assert.Equal(t, expected, buf.String())
	}
}

func TestUpdateTriggerer(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
  "appConfig": {
    "version": "0.15.1",
     "baseDomain": "local.astronomer.io",
     "smtpConfigured": true, 
     "manualReleaseNames": false,
     "hardDeleteDeployment": true,
     "manualNamespaceNames": true,
     "triggererEnabled": true
	},
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
	  "airflowVersion": "2.2.2",
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

	expected := ` NAME        DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG     AIRFLOW VERSION     
 test123     burning-terrestrial-5940     0.0.0     ckbv801t300qh0760pck7ea0c     ?       2.2.2               

 Successfully updated deployment
`
	myTests := []struct {
		deploymentConfig  map[string]string
		dagDeploymentType string
		expectedOutput    string
	}{
		{deploymentConfig: map[string]string{"executor": "CeleryExecutor"}, dagDeploymentType: "", expectedOutput: expected},
		{deploymentConfig: map[string]string{"executor": "CeleryExecutor"}, dagDeploymentType: "image", expectedOutput: expected},
	}
	for _, tt := range myTests {
		buf := new(bytes.Buffer)
		err := Update(id, role, tt.deploymentConfig, tt.dagDeploymentType, "", "", "", "", "", "", "", "", 1, 1, api, buf)
		assert.NoError(t, err)
		assert.Equal(t, expected, buf.String())
	}
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
	err := Update(id, role, deploymentConfig, "", "", "", "", "", "", "", "", "", 1, 0, api, buf)

	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestAirflowUpgrade(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "deployment": {
      "id": "ckbv818oa00r107606ywhoqtw",
	  "airflowVersion": "1.10.5",
	  "desiredAirflowVersion": "1.10.10"
    },
    "updateDeploymentAirflow": {
	  "id": "ckbv818oa00r107606ywhoqtw",
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
	deploymentID := "ckbv818oa00r107606ywhoqtw"
	desiredAirflowVersion := "1.10.10"

	buf := new(bytes.Buffer)
	err := AirflowUpgrade(deploymentID, desiredAirflowVersion, api, buf)
	assert.NoError(t, err)
	expected := ` NAME     DEPLOYMENT NAME     ASTRO     DEPLOYMENT ID                 AIRFLOW VERSION     
 test                         v         ckbv818oa00r107606ywhoqtw     1.10.5              

The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment airflow upgrade --cancel

`

	assert.Equal(t, expected, buf.String())
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
	deploymentID := "ckbv818oa00r107606ywhoqtw"
	desiredAirflowVersion := "1.10.10"

	buf := new(bytes.Buffer)
	err := AirflowUpgrade(deploymentID, desiredAirflowVersion, api, buf)
	assert.Error(t, err, "API error (500):")
}

func TestAirflowUpgradeCancel(t *testing.T) {
	testUtil.InitTestConfig()
	deploymentID := "ckggzqj5f4157qtc9lescmehm"

	okResponse := `{
  "data": {
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
	err := AirflowUpgradeCancel(deploymentID, api, buf)
	assert.NoError(t, err)
	expected := `
Airflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 1.10.5.
`
	assert.Equal(t, expected, buf.String())
}

func TestAirflowUpgradeCancelError(t *testing.T) {
	testUtil.InitTestConfig()
	deploymentID := "ckggzqj5f4157qtc9lescmehm"

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(strings.NewReader(``)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	buf := new(bytes.Buffer)
	err := AirflowUpgradeCancel(deploymentID, api, buf)
	assert.Error(t, err, "API error (500):")
}

func TestAirflowUpgradeEmptyDesiredVersion(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "deployment": {
	  "id": "ckbv818oa00r107606ywhoqtw",
	  "airflowVersion": "1.10.5",
	  "desiredAirflowVersion": "1.10.10"
	  },
	  "updateDeploymentAirflow": {
	  "id": "ckbv818oa00r107606ywhoqtw",
	  "label": "test",
	  "airflowVersion": "1.10.5",
	  "desiredAirflowVersion": "1.10.10"
	  },
	  "deploymentConfig": {
	  "airflowVersions": [
	  "1.10.7",
	  "1.10.10",
	  "1.10.12"
	  ]}
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
	deploymentID := "ckggzqj5f4157qtc9lescmehm"
	desiredAirflowVersion := ""

	// mock os.Stdin for when prompted by getAirflowVersionSelection()
	input := []byte("2")
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

	buf := new(bytes.Buffer)
	err = AirflowUpgrade(deploymentID, desiredAirflowVersion, api, buf)
	t.Log(buf.String()) // Log the buffer so that this test is recognized by go test

	assert.NoError(t, err)
	expected := `#     AIRFLOW VERSION     
1     1.10.7              
2     1.10.10             
3     1.10.12             
 NAME     DEPLOYMENT NAME     ASTRO     DEPLOYMENT ID                 AIRFLOW VERSION     
 test                         v         ckbv818oa00r107606ywhoqtw     1.10.5              

The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment airflow upgrade --cancel

`
	assert.Equal(t, expected, buf.String())
}

func Test_getDeployment(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
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
	deploymentID := "ckbv818oa00r107606ywhoqtw"

	deployment, err := getDeployment(deploymentID, api)
	assert.NoError(t, err)
	assert.Equal(t, deployment, &houston.Deployment{ID: "ckggzqj5f4157qtc9lescmehm", Label: "test", AirflowVersion: "1.10.5", DesiredAirflowVersion: "1.10.10"})
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
	deploymentID := "ckbv818oa00r107606ywhoqtw"

	_, err := getDeployment(deploymentID, api)
	assert.Error(t, err, "test")
}

func Test_getAirflowVersionSelection(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
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
        ]}
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
	input := []byte("2")
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

	airflowVersion, err := getAirflowVersionSelection("1.10.7", api, buf)
	t.Log(buf.String()) // Log the buffer so that this test is recognized by go test
	assert.NoError(t, err)
	assert.Equal(t, "1.10.12", airflowVersion)
}

func Test_getAirflowVersionSelectionError(t *testing.T) {
	deploymentID := "ckggzqj5f4157qtc9lescmehm"
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
	airflowVersion, err := getAirflowVersionSelection(deploymentID, api, buf)
	assert.Error(t, err, "API error (500):")
	assert.Equal(t, "", airflowVersion)
}

func Test_meetsAirflowUpgradeReqs(t *testing.T) {
	airflowVersion := "1.10.12"
	desiredAirflowVersion := "2.0.0"
	err := meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to 1.10.14 "+
		"first and make sure your DAGs and configs are 2.0 compatible")

	airflowVersion = "2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Error: You tried to set --desired-airflow-version to 2.0.0, but this Airflow Deployment "+
		"is already running 2.0.0. Please indicate a higher version of Airflow and try again.")

	airflowVersion = "1.10.14"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.NoError(t, err)

	airflowVersion = "1.10.7"
	desiredAirflowVersion = "1.10.10"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.NoError(t, err)

	airflowVersion = "-1.10.12"
	desiredAirflowVersion = "2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Invalid Semantic Version")

	airflowVersion = "1.10.12"
	desiredAirflowVersion = "-2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Invalid Semantic Version")
}

func TestCheckNFSMountDagDeploymentError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(``)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	assert.Equal(t, false, CheckNFSMountDagDeployment(api))
}

func TestCheckNFSMountDagDeploymentSuccess(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
	  "version": "0.15.1",
	  "baseDomain": "local.astronomer.io",
	  "smtpConfigured": true,
	  "manualReleaseNames": false,
	  "nfsMountDagDeployment": true,
	  "featureFlags": {
		"manualReleaseNames": false,
		"configureDagDeployment": false,
		"nfsMountDagDeployment": true,
		"hardDeleteDeployment": false,
		"manualNamespaceNames": false,
		"sysAdminScalabilityImprovementsEnabled": false
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
	assert.Equal(t, true, CheckNFSMountDagDeployment(api))
}

func TestCheckHardDeleteDeployment(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true,
      "featureFlags": {
        "manualReleaseNames": false,
        "configureDagDeployment": false,
        "nfsMountDagDeployment": false,
        "hardDeleteDeployment": true,
        "manualNamespaceNames": false,
        "sysAdminScalabilityImprovementsEnabled": false
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

	hardDelete := CheckHardDeleteDeployment(api)
	assert.Equal(t, true, hardDelete)
}

func TestCheckHardDeleteDeploymentError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(``)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	assert.Equal(t, false, CheckHardDeleteDeployment(api))
}

func TestCheckTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true,
      "triggererEnabled": true,
      "featureFlags": {
        "manualReleaseNames": false,
        "configureDagDeployment": false,
        "nfsMountDagDeployment": false,
        "hardDeleteDeployment": false,
        "manualNamespaceNames": false,
        "triggererEnabled": true,
        "sysAdminScalabilityImprovementsEnabled": false
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

	triggererEnabled := CheckTriggererEnabled(api)
	assert.Equal(t, true, triggererEnabled)
}

func TestCheckTriggererEnabledError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(``)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	assert.Equal(t, false, CheckTriggererEnabled(api))
}

func TestGetDeploymentSelectionNamespaces(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true
    },
    "availableNamespaces": [ { "name": "test1" }, { "name": "test2" } ]
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
	input := []byte("1")
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

	name, err := getDeploymentSelectionNamespaces(api, buf)
	assert.NoError(t, err)
	expected := `#     AVAILABLE KUBERNETES NAMESPACES     
1     test1                               
2     test2                               
`
	assert.Equal(t, expected, buf.String())
	assert.Equal(t, "test1", name)
}

func TestGetDeploymentSelectionNamespacesNoNamespaces(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true
    },
    "availableNamespaces" : []
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

	buf := new(bytes.Buffer)
	name, err := getDeploymentSelectionNamespaces(api, buf)
	expected := ``
	assert.Equal(t, expected, name)
	assert.EqualError(t, err, "no kubernetes namespaces are available")
}

func TestGetDeploymentSelectionNamespacesParseError(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true
    },
    "availableNamespaces": [ { "name": "test1" }, { "name": "test2" } ]
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
	input := []byte("test")
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

	name, err := getDeploymentSelectionNamespaces(api, buf)
	assert.Equal(t, "", name)
	assert.EqualError(t, err, "cannot parse test to int")
}

func TestGetDeploymentSelectionNamespacesError(t *testing.T) {
	testUtil.InitTestConfig()
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewBufferString(`Internal Server Error`)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	buf := new(bytes.Buffer)
	name, err := getDeploymentSelectionNamespaces(api, buf)
	assert.Equal(t, "", name)
	assert.EqualError(t, err, "API error (500): Internal Server Error")
}

func TestCheckPreCreateNamespacesDeployment(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {
      "version": "0.15.1",
      "baseDomain": "local.astronomer.io",
      "smtpConfigured": true,
      "manualReleaseNames": false,
      "hardDeleteDeployment": true,
      "manualNamespaceNames": true,
      "featureFlags": {
        "manualReleaseNames": false,
        "configureDagDeployment": false,
        "nfsMountDagDeployment": false,
        "hardDeleteDeployment": false,
        "manualNamespaceNames": true,
        "sysAdminScalabilityImprovementsEnabled": false
      }
    }
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

	usesPreCreateNamespace := CheckPreCreateNamespaceDeployment(api)
	assert.Equal(t, true, usesPreCreateNamespace)
}
