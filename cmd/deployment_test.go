package cmd

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

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deployment")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment")
}

func TestDeploymentCreateCommandNfsMountDisabled(t *testing.T) {
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "", expectedError: "unknown flag: --dag-deployment-type"},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandTriggererDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"triggererEnabled": false},
    "createDeployment": {
      "airflowVersion": "2.2.0",
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
      "desiredAirflowVersion": "2.2.0",
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}, expectedOutput: "", expectedError: "unknown flag: --triggerer-replicas"},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"triggererEnabled": true, "featureFlags": { "triggererEnabled": true} },
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--triggerer-replicas=1"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandNfsMountEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"nfsMountDagDeployment": true, "featureFlags": { "nfsMountDagDeployment": true}},
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume, git_sync"},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandGitSyncEnabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"gitSyncDagDeployment": true, "featureFlags": { "gitSyncDagDeployment": true}},
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=local", "--dag-deployment-type=git_sync", "-u=https://github.com/bote795/public-ariflow-dags-test.git", "-p=dagscopy/", "-b=main", "-s=200"}, expectedOutput: "Successfully created deployment with Local executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:bote795/private-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=dummy"}, expectedOutput: "", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume, git_sync"},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentCreateCommandGitSyncDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"gitSyncDagDeployment": false},
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "", expectedError: "unknown flag: --dag-deployment-type"},
		{cmdArgs: []string{"deployment", "create", "new-deployment-name", "--executor=celery"}, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateTriggererEnabledCommand(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"triggererEnabled": true, "featureFlags": { "triggererEnabled": true}},
    "updateDeployment": {
      "createdAt": "2021-04-23T14:29:28.497Z",
      "dagDeployment": {
        "nfsLocation": "test:/test",
        "type": "volume"
      },
      "description": "",
      "id": "cknuetusw0018yqxto2jzxjqq",
      "label": "test_dima22asdasd",
      "releaseName": "amateur-instrument-9515",
      "status": null,
      "type": "airflow",
      "updatedAt": "2021-04-26T21:42:35.361Z",
      "urls": [
        {
          "type": "airflow",
          "url": "https://deployments.local.astronomer.io/amateur-instrument-9515/airflow"
        },
        {
          "type": "flower",
          "url": "https://deployments.local.astronomer.io/amateur-instrument-9515/flower"
        }
      ],
      "version": "0.15.6",
      "workspace": {
        "id": "ckn4phn1k0104v5xtrer5lpli"
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--triggerer-replicas=1"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateCommand(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"nfsMountDagDeployment": true, "gitSyncDagDeployment": true, "featureFlags": { "nfsMountDagDeployment": true, "gitSyncDagDeployment": true}},
    "updateDeployment": {
      "createdAt": "2021-04-23T14:29:28.497Z",
      "dagDeployment": {
        "nfsLocation": "test:/test",
        "type": "volume"
      },
      "description": "",
      "id": "cknuetusw0018yqxto2jzxjqq",
      "label": "test_dima22asdasd",
      "releaseName": "amateur-instrument-9515",
      "status": null,
      "type": "airflow",
      "updatedAt": "2021-04-26T21:42:35.361Z",
      "urls": [
        {
          "type": "airflow",
          "url": "https://deployments.local.astronomer.io/amateur-instrument-9515/airflow"
        },
        {
          "type": "flower",
          "url": "https://deployments.local.astronomer.io/amateur-instrument-9515/flower"
        }
      ],
      "version": "0.15.6",
      "workspace": {
        "id": "ckn4phn1k0104v5xtrer5lpli"
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "-u=https://github.com/bote795/public-ariflow-dags-test.git", "-p=dagscopy/", "-b=main", "-s=200"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:bote795/private-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=git@github.com:neel-astro/private-airflow-dags-test.git", "--ssh-key=./testfiles/ssh_key", "--known-hosts=./testfiles/known_hosts"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=wrong", "--nfs-location=test:/test"}, expectedOutput: "", expectedError: "please specify the correct DAG deployment type, one of the following: image, volume, git_sync"},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--executor=local"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "--cloud-role=arn:aws:iam::1234567890:role/test_role4c2301381e"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
}

func TestDeploymentUpdateCommandGitSyncDisabled(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
  "data": {
    "appConfig": {"nfsMountDagDeployment": true, "gitSyncDagDeployment": false, "featureFlags": { "nfsMountDagDeployment": true, "gitSyncDagDeployment": false}},
    "updateDeployment": {
      "createdAt": "2021-04-23T14:29:28.497Z",
      "dagDeployment": {
        "nfsLocation": "test:/test",
        "type": "volume"
      },
      "description": "",
      "id": "cknuetusw0018yqxto2jzxjqq",
      "label": "test_dima22asdasd",
      "releaseName": "amateur-instrument-9515",
      "status": null,
      "type": "airflow",
      "updatedAt": "2021-04-26T21:42:35.361Z",
      "urls": [
        {
          "type": "airflow",
          "url": "https://deployments.local.astronomer.io/amateur-instrument-9515/airflow"
        },
        {
          "type": "flower",
          "url": "https://deployments.local.astronomer.io/amateur-instrument-9515/flower"
        }
      ],
      "version": "0.15.6",
      "workspace": {
        "id": "ckn4phn1k0104v5xtrer5lpli"
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

	myTests := []struct {
		cmdArgs        []string
		expectedOutput string
		expectedError  string
	}{
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=volume", "--nfs-location=test:/test"}, expectedOutput: "Successfully updated deployment", expectedError: ""},
		{cmdArgs: []string{"deployment", "update", "cknrml96n02523xr97ygj95n5", "label=test22222", "--dag-deployment-type=git_sync", "--git-repository-url=https://github.com/bote795/public-ariflow-dags-test.git", "--dag-directory-path=dagscopy/", "--git-branch-name=main", "--sync-interval=200"}, expectedOutput: "", expectedError: "unknown flag: --git-repository-url"},
	}
	for _, tt := range myTests {
		output, err := executeCommandC(api, tt.cmdArgs...)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Contains(t, output, tt.expectedOutput)
	}
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

	output, err := executeCommandC(api, "deployment", "service-account", "delete", "q1w2e3r4t5y6u7i8o9p0", "--deployment-id=1234")
	assert.NoError(t, err)
	assert.Contains(t, output, "Service Account my_label (q1w2e3r4t5y6u7i8o9p0) successfully deleted")
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

	output, err := executeCommandC(api, "deployment", "service-account", "create", "--deployment-id=ck1qg6whg001r08691y117hub", "--label=my_label", "--role=viewer")
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

	output, err := executeCommandC(api, "deployment", "user", "add", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "somebody@astronomer.com")
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

	output, err := executeCommandC(api, "deployment", "user", "delete", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "somebody@astronomer.com")
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

	output, err := executeCommandC(api, "deployment", "user", "update", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "--role=DEPLOYMENT_ADMIN", "somebody@astronomer.com")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.`

	okResponse := `{
  "data": {
    "appConfig": {"nfsMountDagDeployment": false},
    "deployment": {
	  "id": "ckggzqj5f4157qtc9lescmehm",
	  "airflowVersion": "1.10.5",
	  "desiredAirflowVersion": "1.10.10"
	  },
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

	output, err := executeCommandC(api, "deployment", "airflow", "upgrade", "--deployment-id=ckggvxkw112212kc9ebv8vu6p", "--desired-airflow-version=1.10.10")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentAirflowUpgradeCancelCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Airflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 1.10.5.`

	okResponse := `{
  "data": {
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

	output, err := executeCommandC(api, "deployment", "airflow", "upgrade", "--cancel", "--deployment-id=ckggvxkw112212kc9ebv8vu6p")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentSAGetCommand(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
	okResponse := `
{
  "data": {
    "appConfig": {"nfsMountDagDeployment": false},
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
			Body:       ioutil.NopCloser(strings.NewReader(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)

	output, err := executeCommandC(api, "deployment", "sa", "get", "--deployment-id=ckqvf9spa1189rn9hbh5h439u")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Successfully deleted deployment`
	okResponse := `{
		"data": {
		  "appConfig": {"nfsMountDagDeployment": false},
		  "deleteDeployment": {
		    "id": "ckqh2dmzc43548h9hxzspysyi",
		    "type": "airflow",
		    "label": "test2",
		    "description": "",
		    "releaseName": "combusting-radiant-1610",
		    "version": "0.17.1",
		    "workspace": {
		      "id": "ckqh2d9zh40758h9h650gf8dc"
		    },
		    "createdAt": "2021-06-28T20:19:03.193Z",
		    "updatedAt": "2021-07-07T18:16:52.118Z"
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

	output, err := executeCommandC(api, "deployment", "delete", "ckqh2dmzc43548h9hxzspysyi")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}

func TestDeploymentDeleteHardResponseNo(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
		  "appConfig": {
			"version": "0.15.1",
			"baseDomain": "local.astronomer.io",
			"smtpConfigured": true,
			"manualReleaseNames": false,
			"hardDeleteDeployment": true,
			"nfsMountDagDeployment": false,
			"featureFlags": {
				"hardDeleteDeployment": true
			}
		},
		  "deleteDeployment": {
		    "id": "ckqh2dmzc43548h9hxzspysyi",
		    "type": "airflow",
		    "label": "test2",
		    "description": "",
		    "releaseName": "combusting-radiant-1610",
		    "version": "0.17.1",
		    "workspace": {
		      "id": "ckqh2d9zh40758h9h650gf8dc"
		    },
		    "createdAt": "2021-06-28T20:19:03.193Z",
		    "updatedAt": "2021-07-07T18:16:52.118Z"
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

	// mock os.Stdin
	input := []byte("n")
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

	_, err = executeCommandC(api, "deployment", "delete", "--hard", "ckqh2dmzc43548h9hxzspysyi")
	assert.Nil(t, err)
}

func TestDeploymentDeleteHardResponseYes(t *testing.T) {
	testUtil.InitTestConfig()
	expectedOut := `Successfully deleted deployment`
	okResponse := `{
		"data": {
		  "appConfig": {
			"version": "0.15.1",
			"baseDomain": "local.astronomer.io",
			"smtpConfigured": true,
			"manualReleaseNames": false,
			"hardDeleteDeployment": true,
			"nfsMountDagDeployment": false,
			"featureFlags": {
			   "hardDeleteDeployment": true
			}
		},
		  "deleteDeployment": {
		    "id": "ckqh2dmzc43548h9hxzspysyi",
		    "type": "airflow",
		    "label": "test2",
		    "description": "",
		    "releaseName": "combusting-radiant-1610",
		    "version": "0.17.1",
		    "workspace": {
		      "id": "ckqh2d9zh40758h9h650gf8dc"
		    },
		    "createdAt": "2021-06-28T20:19:03.193Z",
		    "updatedAt": "2021-07-07T18:16:52.118Z"
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

	// mock os.Stdin
	input := []byte("y")
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

	output, err := executeCommandC(api, "deployment", "delete", "--hard", "ckqh2dmzc43548h9hxzspysyi")
	assert.NoError(t, err)
	assert.Contains(t, output, expectedOut)
}
