package fromfile

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/stretchr/testify/mock"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

var errTest = errors.New("test error")

func TestCreate(t *testing.T) {
	var (
		err            error
		filePath, data string
	)

	t.Run("reads the yaml file and creates a deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		filePath = "./deployment.yaml"
		data = `
deployment:
  environment_variables:
    - is_secret: false
      key: foo
      updated_at: NOW
      value: bar
    - is_secret: true
      key: bar
      updated_at: NOW+1
      value: baz
  configuration:
    name: test-deployment-label
    description: description
    runtime_version: 6.0.0
    scheduler_au: 5
    scheduler_count: 3
    cluster_id: cluster-id
  worker_queues:
    - name: default
      id: test-wq-id
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 110
      node_pool_id: test-pool-id
    - name: test-queue-1
      id: test-wq-id-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 150
      node_pool_id: test-pool-id-1
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    dag_deploy_enabled: true
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
`

		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
		err = Create("deployment.yaml", mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("reads the json file and creates a deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		filePath = "./deployment.yaml"
		data = `{
    "deployment": {
        "environment_variables": [
            {
                "is_secret": false,
                "key": "foo",
                "updated_at": "NOW",
                "value": "bar"
            },
            {
                "is_secret": true,
                "key": "bar",
                "updated_at": "NOW+1",
                "value": "baz"
            }
        ],
        "configuration": {
            "name": "test-deployment-label",
            "description": "description",
            "runtime_version": "6.0.0",
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_id": "cluster-id"
        },
        "worker_queues": [
            {
                "name": "default",
                "id": "test-wq-id",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 110,
                "node_pool_id": "test-pool-id"
            },
            {
                "name": "test-queue-1",
                "id": "test-wq-id-1",
                "is_default": false,
                "max_worker_count": 175,
                "min_worker_count": 8,
                "worker_concurrency": 150,
                "node_pool_id": "test-pool-id-1"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "dag_deploy_enabled": true,
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1",
            "email2"
        ]
    }
}`

		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
		err = Create("deployment.yaml", mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if file does not exist", func(t *testing.T) {
		err = Create("deployment.yaml", nil)
		assert.ErrorContains(t, err, "open deployment.yaml: no such file or directory")
	})
	t.Run("returns an error if file exists but no perms to read it", func(t *testing.T) {
		filePath = "deployment.yaml"
		data = "test"
		err = fileutil.WriteStringToFile(filePath, data)
		assert.NoError(t, err)
		mode := fs.FileMode(0o000)
		afero.NewOsFs().Chmod(filePath, mode)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", nil)
		assert.ErrorContains(t, err, "permission denied")
	})
	t.Run("returns an error if file exists but user provides incorrect path", func(t *testing.T) {
		filePath = "./2/deployment.yaml"
		data = "test"
		err = fileutil.WriteStringToFile(filePath, data)
		assert.NoError(t, err)
		defer afero.NewOsFs().RemoveAll("./2")
		err = Create("1/deployment.yaml", nil)
		assert.ErrorContains(t, err, "open 1/deployment.yaml: no such file or directory")
	})
	t.Run("returns an error if file is empty", func(t *testing.T) {
		filePath = "./deployment.yaml"
		data = ""
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", nil)
		assert.ErrorIs(t, err, errEmptyFile)
		assert.ErrorContains(t, err, "deployment.yaml has no content")
	})
	t.Run("returns an error if unmarshalling fails", func(t *testing.T) {
		filePath = "./deployment.yaml"
		data = "test"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", nil)
		assert.ErrorContains(t, err, "error unmarshaling JSON:")
	})
	t.Run("returns an error from the api if create deployment fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		filePath = "./deployment.yaml"
		data = `{
    "deployment": {
        "environment_variables": [
            {
                "is_secret": false,
                "key": "foo",
                "updated_at": "NOW",
                "value": "bar"
            },
            {
                "is_secret": true,
                "key": "bar",
                "updated_at": "NOW+1",
                "value": "baz"
            }
        ],
        "configuration": {
            "name": "test-deployment-label",
            "description": "description",
            "runtime_version": "6.0.0",
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_id": "cluster-id"
        },
        "worker_queues": [
            {
                "name": "default",
                "id": "test-wq-id",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 110,
                "node_pool_id": "test-pool-id"
            },
            {
                "name": "test-queue-1",
                "id": "test-wq-id-1",
                "is_default": false,
                "max_worker_count": 175,
                "min_worker_count": 8,
                "worker_concurrency": 150,
                "node_pool_id": "test-pool-id-1"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "dag_deploy_enabled": true,
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1",
            "email2"
        ]
    }
}`

		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, errTest)
		err = Create("deployment.yaml", mockClient)
		assert.ErrorIs(t, err, errCreateFailed)
		assert.ErrorContains(t, err, "test error: failed to create deployment with input")
		mockClient.AssertExpectations(t)
	})
}

func TestGetCreateInput(t *testing.T) {
	t.Run("transforms formattedDeployment to CreateDeploymentInput", func(t *testing.T) {
		var (
			expectedDeploymentInput, actual astro.CreateDeploymentInput
			deploymentFromFile              inspect.FormattedDeployment
		)
		deploymentFromFile.Deployment.Configuration.ClusterID = "test-cluster-id"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2

		expectedDeploymentInput = astro.CreateDeploymentInput{
			WorkspaceID:           "",
			ClusterID:             deploymentFromFile.Deployment.Configuration.ClusterID,
			Label:                 deploymentFromFile.Deployment.Configuration.Name,
			Description:           deploymentFromFile.Deployment.Configuration.Description,
			RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
			DagDeployEnabled:      false, // should come from configuration
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
			},
		}
		actual = getCreateInput(&deploymentFromFile)
		assert.Equal(t, expectedDeploymentInput, actual)
	})
}
