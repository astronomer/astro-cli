package fromfile

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

var errTest = errors.New("test error")

func TestCreate(t *testing.T) {
	var (
		err                           error
		filePath, data, orgID         string
		existingClusters              []astro.Cluster
		existingWorkspaces            []astro.Workspace
		mockWorkerQueueDefaultOptions astro.WorkerQueueDefaultOptions
		emails                        []string
		mockAlertEmailResponse        astro.DeploymentAlerts
		createdDeployment             astro.Deployment
	)

	t.Run("reads the yaml file and creates a deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						ID:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				},
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		mockEnvVarResponse := []astro.EnvironmentVariablesObject{
			{
				IsSecret:  false,
				Key:       "foo",
				Value:     "bar",
				UpdatedAt: "NOW",
			},
			{
				IsSecret:  true,
				Key:       "bar",
				Value:     "baz",
				UpdatedAt: "NOW+1",
			},
		}
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		emails = []string{"test1@test.com", "test2@test.com"}
		mockAlertEmailResponse = astro.DeploymentAlerts{AlertEmails: emails}
		createdDeployment = astro.Deployment{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
		mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{createdDeployment}, nil)
		err = Create("deployment.yaml", mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "configuration:\n        name: "+createdDeployment.Label)
		assert.Contains(t, out.String(), "metadata:\n        deployment_id: "+createdDeployment.ID)
		mockClient.AssertExpectations(t)
	})
	t.Run("reads the json file and creates a deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
            "dag_deploy_enabled": true,
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-workspace"
        },
        "worker_queues": [
            {
                "name": "default",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 180,
                "worker_type": "test-worker-1"
            },
            {
                "name": "test-queue-1",
                "is_default": false,
                "max_worker_count": 175,
                "min_worker_count": 8,
                "worker_concurrency": 176,
                "worker_type": "test-worker-2"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						ID:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				},
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		mockEnvVarResponse := []astro.EnvironmentVariablesObject{
			{
				IsSecret:  false,
				Key:       "foo",
				Value:     "bar",
				UpdatedAt: "NOW",
			},
			{
				IsSecret:  true,
				Key:       "bar",
				Value:     "baz",
				UpdatedAt: "NOW+1",
			},
		}
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		emails = []string{"test1@test.com", "test2@test.com"}
		mockAlertEmailResponse = astro.DeploymentAlerts{AlertEmails: emails}
		createdDeployment = astro.Deployment{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
		mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{createdDeployment}, nil)
		err = Create("deployment.yaml", mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "\"configuration\": {\n            \"name\": \""+createdDeployment.Label+"\"")
		assert.Contains(t, out.String(), "\"metadata\": {\n            \"deployment_id\": \""+createdDeployment.ID+"\"")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if file does not exist", func(t *testing.T) {
		err = Create("deployment.yaml", nil, nil)
		assert.ErrorContains(t, err, "open deployment.yaml: no such file or directory")
	})
	t.Run("returns an error if file exists but user provides incorrect path", func(t *testing.T) {
		filePath = "./2/deployment.yaml"
		data = "test"
		err = fileutil.WriteStringToFile(filePath, data)
		assert.NoError(t, err)
		defer afero.NewOsFs().RemoveAll("./2")
		err = Create("1/deployment.yaml", nil, nil)
		assert.ErrorContains(t, err, "open 1/deployment.yaml: no such file or directory")
	})
	t.Run("returns an error if file is empty", func(t *testing.T) {
		filePath = "./deployment.yaml"
		data = ""
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", nil, nil)
		assert.ErrorIs(t, err, errEmptyFile)
		assert.ErrorContains(t, err, "deployment.yaml has no content")
	})
	t.Run("returns an error if unmarshalling fails", func(t *testing.T) {
		filePath = "./deployment.yaml"
		data = "test"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", nil, nil)
		assert.ErrorContains(t, err, "error unmarshaling JSON:")
	})
	t.Run("returns an error if required fields are missing", func(t *testing.T) {
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
    name:
    description: description
    runtime_version: 6.0.0
    dag_deploy_enabled: true
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
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", nil, nil)
		assert.ErrorContains(t, err, "missing required field: deployment.configuration.name")
	})
	t.Run("returns an error if getting context fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`

		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorContains(t, err, "no context set")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if workspace does not exist", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, nil)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errNotFound)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, errTest)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster does not exist", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: cluster-name
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errNotFound)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing cluster fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		orgID = "test-org-id"
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errTest)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, errTest)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if deployment already exists", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		existingDeployments := []astro.Deployment{
			{
				Label:       "test-deployment-label",
				Description: "deployment-1",
			},
			{
				Label:       "d-2",
				Description: "deployment-2",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
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
    dag_deploy_enabled: true
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return(existingDeployments, nil)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorContains(t, err, "deployment: test-deployment-label already exists: use deployment update --from-file deployment.yaml instead")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if creating deployment input fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
            "dag_deploy_enabled": true,
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-workspace"
        },
        "worker_queues": [
            {
                "name": "default",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 180,
                "worker_type": "test-worker-1"
            },
            {
                "name": "test-queue-1",
                "is_default": false,
                "max_worker_count": 130,
                "min_worker_count": 8,
                "worker_concurrency": 150,
                "worker_type": "test-worker-2"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
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
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						ID:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				},
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		err = Create("deployment.yaml", mockClient, nil)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "worker queue option is invalid: worker concurrency")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error from the api if create deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
            "dag_deploy_enabled": true,
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-workspace"
        },
        "worker_queues": [
            {
                "name": "default",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 180,
                "worker_type": "test-worker-1"
            },
            {
                "name": "test-queue-1",
                "is_default": false,
                "max_worker_count": 130,
                "min_worker_count": 8,
                "worker_concurrency": 176,
                "worker_type": "test-worker-2"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
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
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						ID:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				},
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, errTest)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errCreateFailed)
		assert.ErrorContains(t, err, "test error: failed to create deployment with input")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error from the api if creating environment variables fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
            "dag_deploy_enabled": true,
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-workspace"
        },
        "worker_queues": [
            {
                "name": "default",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 180,
                "worker_type": "test-worker-1"
            },
            {
                "name": "test-queue-1",
                "is_default": false,
                "max_worker_count": 130,
                "min_worker_count": 8,
                "worker_concurrency": 176,
                "worker_type": "test-worker-2"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						ID:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				},
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, errTest)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error from the api if creating alert emails fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
            "dag_deploy_enabled": true,
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-workspace"
        },
        "worker_queues": [
            {
                "name": "default",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 180,
                "worker_type": "test-worker-1"
            },
            {
                "name": "test-queue-1",
                "is_default": false,
                "max_worker_count": 130,
                "min_worker_count": 8,
                "worker_concurrency": 176,
                "worker_type": "test-worker-2"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/analytics",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
		existingClusters = []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						ID:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				},
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		existingWorkspaces = []astro.Workspace{
			{
				ID:    "test-workspace-id",
				Label: "test-workspace",
			},
			{
				ID:    "test-workspace-id-1",
				Label: "test-workspace-1",
			},
		}
		orgID = "test-org-id"
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
		mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, errTest)
		err = Create("deployment.yaml", mockClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockClient.AssertExpectations(t)
	})
}

func TestGetCreateInput(t *testing.T) {
	var (
		expectedDeploymentInput, actual astro.CreateDeploymentInput
		deploymentFromFile              inspect.FormattedDeployment
		qList                           []inspect.Workerq
		existingPools                   []astro.NodePool
		expectedQList                   []astro.WorkerQueue
		clusterID, workspaceID          string
		err                             error
		mockWorkerQueueDefaultOptions   astro.WorkerQueueDefaultOptions
	)
	t.Run("transforms formattedDeployment to CreateDeploymentInput if no queues were requested", func(t *testing.T) {
		clusterID = "test-cluster-id"
		workspaceID = "test-workspace-id"
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2

		expectedDeploymentInput = astro.CreateDeploymentInput{
			WorkspaceID:           workspaceID,
			ClusterID:             clusterID,
			Label:                 deploymentFromFile.Deployment.Configuration.Name,
			Description:           deploymentFromFile.Deployment.Configuration.Description,
			RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
			DagDeployEnabled:      deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
			},
			WorkerQueues: nil,
		}
		mockClient := new(astro_mocks.Client)
		actual, err = getCreateInput(&deploymentFromFile, clusterID, workspaceID, nil, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeploymentInput, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns correct deployment input when multiple queues are requested", func(t *testing.T) {
		clusterID = "test-cluster-id"
		workspaceID = "test-workspace-id"
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		qList = []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		expectedQList = []astro.WorkerQueue{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				NodePoolID:        "test-pool-id",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				NodePoolID:        "test-pool-id-2",
			},
		}
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}

		expectedDeploymentInput = astro.CreateDeploymentInput{
			WorkspaceID:           workspaceID,
			ClusterID:             clusterID,
			Label:                 deploymentFromFile.Deployment.Configuration.Name,
			Description:           deploymentFromFile.Deployment.Configuration.Description,
			RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
			DagDeployEnabled:      deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
			},
			WorkerQueues: expectedQList,
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		actual, err = getCreateInput(&deploymentFromFile, clusterID, workspaceID, existingPools, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeploymentInput, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("sets default queue options if none were requested", func(t *testing.T) {
		clusterID = "test-cluster-id"
		workspaceID = "test-workspace-id"
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		qList = []inspect.Workerq{
			{
				Name:       "default",
				IsDefault:  true,
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				IsDefault:  false,
				WorkerType: "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		expectedQList = []astro.WorkerQueue{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    125,
				MinWorkerCount:    5,
				WorkerConcurrency: 180,
				NodePoolID:        "test-pool-id",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    125,
				MinWorkerCount:    5,
				WorkerConcurrency: 180,
				NodePoolID:        "test-pool-id-2",
			},
		}
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}

		expectedDeploymentInput = astro.CreateDeploymentInput{
			WorkspaceID:           workspaceID,
			ClusterID:             clusterID,
			Label:                 deploymentFromFile.Deployment.Configuration.Name,
			Description:           deploymentFromFile.Deployment.Configuration.Description,
			RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
			DagDeployEnabled:      deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
					Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
				},
			},
			WorkerQueues: expectedQList,
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		actual, err = getCreateInput(&deploymentFromFile, clusterID, workspaceID, existingPools, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeploymentInput, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns error if worker type does not match existing pools", func(t *testing.T) {
		clusterID = "test-cluster-id"
		workspaceID = "test-workspace-id"
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		qList = []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-8",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}

		expectedDeploymentInput = astro.CreateDeploymentInput{}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		actual, err = getCreateInput(&deploymentFromFile, clusterID, workspaceID, existingPools, mockClient)
		assert.ErrorContains(t, err, "worker_type: test-worker-8 does not exist in cluster: test-cluster")
		assert.Equal(t, expectedDeploymentInput, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns error if queue options are invalid", func(t *testing.T) {
		clusterID = "test-cluster-id"
		workspaceID = "test-workspace-id"
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		qList = []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    30,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		mockWorkerQueueDefaultOptions = astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}

		expectedDeploymentInput = astro.CreateDeploymentInput{}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		actual, err = getCreateInput(&deploymentFromFile, clusterID, workspaceID, existingPools, mockClient)
		assert.ErrorContains(t, err, "worker queue option is invalid: min worker count")
		assert.Equal(t, expectedDeploymentInput, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns error if getting worker queue options fails", func(t *testing.T) {
		clusterID = "test-cluster-id"
		workspaceID = "test-workspace-id"
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		qList = []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    30,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		expectedDeploymentInput = astro.CreateDeploymentInput{}
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errTest).Once()
		actual, err = getCreateInput(&deploymentFromFile, clusterID, workspaceID, existingPools, mockClient)
		assert.ErrorContains(t, err, "failed to get worker queue default options")
		assert.Equal(t, expectedDeploymentInput, actual)
		mockClient.AssertExpectations(t)
	})
}

func TestCheckRequiredFields(t *testing.T) {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	input.Deployment.Configuration.Description = "test-description"
	t.Run("returns an error if name is missing", func(t *testing.T) {
		err = checkRequiredFields(&input)
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.configuration.name")
	})
	t.Run("returns an error if cluster_name is missing", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		err = checkRequiredFields(&input)
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.configuration.cluster_name")
	})
	t.Run("if queues were requested, it returns an error if queue name is missing", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		qList := []inspect.Workerq{
			{
				Name:       "",
				IsDefault:  true,
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				IsDefault:  false,
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input)
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.worker_queues[0].name")
	})
	t.Run("if queues were requested, it returns an error if default queue is not named default", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		qList := []inspect.Workerq{
			{
				Name:       "test-q-1",
				IsDefault:  false,
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				IsDefault:  true,
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input)
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.worker_queues[1].name = default")
	})
	t.Run("if queues were requested, it returns an error if worker type is missing", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		qList := []inspect.Workerq{
			{
				Name:      "test-q-1",
				IsDefault: false,
			},
			{
				Name:       "default",
				IsDefault:  true,
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input)
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.worker_queues[0].worker_type")
	})
	t.Run("returns nil if there are no missing fields", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		qList := []inspect.Workerq{
			{
				Name:       "default",
				IsDefault:  true,
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				IsDefault:  false,
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input)
		assert.NoError(t, err)
	})
}

func TestDeploymentExists(t *testing.T) {
	var (
		existingDeployments []astro.Deployment
		deploymentToCreate  string
		actual              bool
	)
	existingDeployments = []astro.Deployment{
		{
			ID:          "test-d-1",
			Label:       "test-deployment-1",
			Description: "deployment 1",
		},
		{
			ID:          "test-d-2",
			Label:       "test-deployment-2",
			Description: "deployment 2",
		},
	}
	deploymentToCreate = "test-deployment-2"
	t.Run("returns true if deployment already exists", func(t *testing.T) {
		actual = deploymentExists(existingDeployments, deploymentToCreate)
		assert.True(t, actual)
	})
	t.Run("returns false if deployment does not exist", func(t *testing.T) {
		deploymentToCreate = "test-d-2"
		actual = deploymentExists(existingDeployments, deploymentToCreate)
		assert.False(t, actual)
	})
}

func TestGetClusterFromName(t *testing.T) {
	var (
		clusterName, expectedClusterID, actualClusterID, orgID string
		existingPools, actualNodePools                         []astro.NodePool
		err                                                    error
	)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedClusterID = "test-cluster-id"
	clusterName = "test-cluster"
	existingPools = []astro.NodePool{
		{
			ID:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-1",
		},
		{
			ID:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-2",
		},
	}
	orgID = "test-org-id"
	t.Run("returns a cluster id if cluster exists in organization", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		existingClusters := []astro.Cluster{
			{
				ID:        "test-cluster-id",
				Name:      "test-cluster",
				NodePools: existingPools,
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, orgID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedClusterID, actualClusterID)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns error from api if listing cluster fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errTest)
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, orgID, mockClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, "", actualClusterID)
		assert.Equal(t, []astro.NodePool(nil), actualNodePools)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster does not exist in organization", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, nil)
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, orgID, mockClient)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "cluster_name: test-cluster does not exist in organization: test-org-id")
		assert.Equal(t, "", actualClusterID)
		assert.Equal(t, []astro.NodePool(nil), actualNodePools)
		mockClient.AssertExpectations(t)
	})
}

func TestGetWorkspaceIDFromName(t *testing.T) {
	var (
		workspaceName, expectedWorkspaceID, actualWorkspaceID, orgID string
		existingWorkspaces                                           []astro.Workspace
		err                                                          error
	)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedWorkspaceID = "test-workspace-id"
	workspaceName = "test-workspace"
	orgID = "test-org-id"
	existingWorkspaces = []astro.Workspace{
		{
			ID:    "test-workspace-id",
			Label: "test-workspace",
		},
		{
			ID:    "test-workspace-id-1",
			Label: "test-workspace-1",
		},
	}
	t.Run("returns a workspace id if workspace exists in organization", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedWorkspaceID, actualWorkspaceID)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns error from api if listing workspace fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, errTest)
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, "", actualWorkspaceID)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if workspace does not exist in organization", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, nil)
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockClient)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "workspace_name: test-workspace does not exist in organization: test-org-id")
		assert.Equal(t, "", actualWorkspaceID)
		mockClient.AssertExpectations(t)
	})
}

func TestGetNodePoolIDFromName(t *testing.T) {
	var (
		workerType, expectedPoolID, actualPoolID, clusterID string
		existingPools                                       []astro.NodePool
		err                                                 error
	)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedPoolID = "test-pool-id"
	workerType = "worker-1"
	clusterID = "test-cluster-id"
	existingPools = []astro.NodePool{
		{
			ID:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-1",
		},
		{
			ID:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-2",
		},
	}
	t.Run("returns a nodepool id from cluster for pool with matching worker type", func(t *testing.T) {
		actualPoolID, err = getNodePoolIDFromWorkerType(workerType, clusterID, existingPools)
		assert.NoError(t, err)
		assert.Equal(t, expectedPoolID, actualPoolID)
	})
	t.Run("returns an error if no pool with matching worker type exists in the cluster", func(t *testing.T) {
		workerType = "worker-3"
		actualPoolID, err = getNodePoolIDFromWorkerType(workerType, clusterID, existingPools)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "worker_type: worker-3 does not exist in cluster: test-cluster")
		assert.Equal(t, "", actualPoolID)
	})
}

func TestHasEnvVars(t *testing.T) {
	t.Run("returns true if there are env vars in the deployment", func(t *testing.T) {
		var deploymentFromFile inspect.FormattedDeployment
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
				Value:     "val-1",
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     "val-2",
			},
		}
		deploymentFromFile.Deployment.EnvVars = list
		actual := hasEnvVars(&deploymentFromFile)
		assert.True(t, actual)
	})
	t.Run("returns false if there are no env vars in the deployment", func(t *testing.T) {
		var deploymentFromFile inspect.FormattedDeployment
		actual := hasEnvVars(&deploymentFromFile)
		assert.False(t, actual)
	})
}

func TestCreateEnvVars(t *testing.T) {
	var (
		expectedEnvVarsInput astro.EnvironmentVariablesInput
		actualEnvVars        []astro.EnvironmentVariablesObject
		deploymentFromFile   inspect.FormattedDeployment
		err                  error
	)
	t.Run("creates env vars if they were requested in a formatted deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		deploymentFromFile = inspect.FormattedDeployment{}
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
				Value:     "val-1",
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     "val-2",
			},
		}
		expectedList := []astro.EnvironmentVariable{
			{
				IsSecret: false,
				Key:      "key-1",
				Value:    "val-1",
			},
			{
				IsSecret: true,
				Key:      "key-2",
				Value:    "val-2",
			},
		}
		mockResponse := []astro.EnvironmentVariablesObject{
			{
				IsSecret:  false,
				Key:       "key-1",
				Value:     "val-1",
				UpdatedAt: "now",
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				Value:     "val-2",
				UpdatedAt: "now",
			},
		}
		deploymentFromFile.Deployment.EnvVars = list
		expectedEnvVarsInput = astro.EnvironmentVariablesInput{
			DeploymentID:         "test-deployment-id",
			EnvironmentVariables: expectedList,
		}
		mockClient.On("ModifyDeploymentVariable", expectedEnvVarsInput).Return(mockResponse, nil)
		actualEnvVars, err = createEnvVars(&deploymentFromFile, "test-deployment-id", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, actualEnvVars)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns api error if modifyDeploymentVariable fails", func(t *testing.T) {
		var mockResponse []astro.EnvironmentVariablesObject
		mockClient := new(astro_mocks.Client)
		deploymentFromFile = inspect.FormattedDeployment{}
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
				Value:     "val-1",
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     "val-2",
			},
		}
		expectedList := []astro.EnvironmentVariable{
			{
				IsSecret: false,
				Key:      "key-1",
				Value:    "val-1",
			},
			{
				IsSecret: true,
				Key:      "key-2",
				Value:    "val-2",
			},
		}
		deploymentFromFile.Deployment.EnvVars = list
		expectedEnvVarsInput = astro.EnvironmentVariablesInput{
			DeploymentID:         "test-deployment-id",
			EnvironmentVariables: expectedList,
		}
		mockClient.On("ModifyDeploymentVariable", expectedEnvVarsInput).Return(mockResponse, errTest)
		actualEnvVars, err = createEnvVars(&deploymentFromFile, "test-deployment-id", mockClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, mockResponse, actualEnvVars)
		mockClient.AssertExpectations(t)
	})
}

func TestHasQueues(t *testing.T) {
	t.Run("returns true if there are worker queues in the deployment", func(t *testing.T) {
		var deploymentFromFile inspect.FormattedDeployment
		qList := []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		actual := hasQueues(&deploymentFromFile)
		assert.True(t, actual)
	})
	t.Run("returns false if there are no worker queues in the deployment", func(t *testing.T) {
		var deploymentFromFile inspect.FormattedDeployment
		actual := hasQueues(&deploymentFromFile)
		assert.False(t, actual)
	})
}

func TestGetQueues(t *testing.T) {
	var (
		deploymentFromFile inspect.FormattedDeployment
		actualWQList       []astro.WorkerQueue
		existingPools      []astro.NodePool
		err                error
	)
	t.Run("returns list of queues for the requested deployment", func(t *testing.T) {
		expectedWQList := []astro.WorkerQueue{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				NodePoolID:        "test-pool-id",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				NodePoolID:        "test-pool-id-2",
			},
		}
		qList := []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-2",
			},
		}
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        true,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.WorkerQs = qList
		actualWQList, err = getQueues(&deploymentFromFile, existingPools)
		assert.NoError(t, err)
		assert.Equal(t, expectedWQList, actualWQList)
	})
	t.Run("returns an error if unable to determine nodepool id", func(t *testing.T) {
		qList := []inspect.Workerq{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-4",
			},
		}
		existingPools = []astro.NodePool{
			{
				ID:               "test-pool-id",
				IsDefault:        true,
				NodeInstanceType: "test-worker-1",
			},
			{
				ID:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.WorkerQs = qList
		actualWQList, err = getQueues(&deploymentFromFile, existingPools)
		assert.ErrorContains(t, err, "worker_type: test-worker-4 does not exist in cluster: test-cluster")
		assert.Equal(t, []astro.WorkerQueue(nil), actualWQList)
	})
}

func TestHasAlertEmails(t *testing.T) {
	t.Run("returns true if there are env vars in the deployment", func(t *testing.T) {
		var deploymentFromFile inspect.FormattedDeployment
		list := []string{"test@test.com", "testing@testing.com"}
		deploymentFromFile.Deployment.AlertEmails = list
		actual := hasAlertEmails(&deploymentFromFile)
		assert.True(t, actual)
	})
	t.Run("returns false if there are no env vars in the deployment", func(t *testing.T) {
		var deploymentFromFile inspect.FormattedDeployment
		actual := hasAlertEmails(&deploymentFromFile)
		assert.False(t, actual)
	})
}

func TestCreateAlertEmails(t *testing.T) {
	var (
		deploymentFromFile inspect.FormattedDeployment
		expectedInput      astro.UpdateDeploymentAlertsInput
		expected, actual   astro.DeploymentAlerts
		emails             []string
		deploymentID       string
		err                error
	)
	t.Run("updates alert emails for a deployment", func(t *testing.T) {
		emails = []string{"test1@email.com", "test2@email.com"}
		deploymentFromFile.Deployment.AlertEmails = emails
		expected = astro.DeploymentAlerts{AlertEmails: emails}
		deploymentID = "test-deployment-id"
		expectedInput = astro.UpdateDeploymentAlertsInput{
			DeploymentID: deploymentID,
			AlertEmails:  emails,
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("UpdateAlertEmails", expectedInput).Return(expected, nil)
		actual, err = createAlertEmails(&deploymentFromFile, deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns api error if updating deployment alert email fails", func(t *testing.T) {
		emails = []string{"test1@email.com", "test2@meail.com"}
		deploymentFromFile.Deployment.AlertEmails = emails
		expected = astro.DeploymentAlerts{}
		deploymentID = "test-deployment-id"
		expectedInput = astro.UpdateDeploymentAlertsInput{
			DeploymentID: deploymentID,
			AlertEmails:  emails,
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("UpdateAlertEmails", expectedInput).Return(expected, errTest)
		actual, err = createAlertEmails(&deploymentFromFile, deploymentID, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expected, actual)
		mockClient.AssertExpectations(t)
	})
}

func TestIsJSON(t *testing.T) {
	var (
		valid, invalid string
		actual         bool
	)
	t.Run("returns true for valid json", func(t *testing.T) {
		valid = `{"test":"yay"}`
		actual = isJSON([]byte(valid))
		assert.True(t, actual)
	})
	t.Run("returns false for invalid json", func(t *testing.T) {
		invalid = `-{"test":"yay",{}`
		actual = isJSON([]byte(invalid))
		assert.False(t, actual)
	})
}
