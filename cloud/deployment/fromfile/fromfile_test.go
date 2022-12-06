package fromfile

import (
	"errors"
	"io/fs"
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
		err                   error
		filePath, data, orgID string
		existingClusters      []astro.Cluster
		existingWorkspaces    []astro.Workspace
	)

	t.Run("reads the yaml file and creates a deployment", func(t *testing.T) {
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
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
		err = Create("deployment.yaml", mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("reads the json file and creates a deployment", func(t *testing.T) {
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
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
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
		err = Create("deployment.yaml", nil)
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
    cluster_name: cluster-name
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
		err = Create("deployment.yaml", mockClient)
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
    cluster_name: cluster-name
    workspace_name: test-workspace
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
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, nil)
		err = Create("deployment.yaml", mockClient)
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
    cluster_name: cluster-name
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
		orgID = "test-org-id"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, errTest)
		err = Create("deployment.yaml", mockClient)
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
		err = Create("deployment.yaml", mockClient)
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
    cluster_name: cluster-name
    workspace_name: test-workspace
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
		err = Create("deployment.yaml", mockClient)
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
		err = Create("deployment.yaml", mockClient)
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
		err = Create("deployment.yaml", mockClient)
		assert.ErrorContains(t, err, "deployment: test-deployment-label already exists: use deployment update --from-file deployment.yaml instead")
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
		mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{}, nil)
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
			clusterID, workspaceID          string
		)
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
		}
		actual = getCreateInput(&deploymentFromFile, clusterID, workspaceID)
		assert.Equal(t, expectedDeploymentInput, actual)
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
	t.Run("returns nil if there are no missing fields", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
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

func TestGetClusterIDFromName(t *testing.T) {
	var (
		clusterName, expectedClusterID, actualClusterID, orgID string
		err                                                    error
	)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedClusterID = "test-cluster-id"
	clusterName = "test-cluster"
	orgID = "test-org-id"
	t.Run("returns a cluster id if cluster exists in organization", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		existingClusters := []astro.Cluster{
			{
				ID:   "test-cluster-id",
				Name: "test-cluster",
			},
			{
				ID:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		}
		mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
		actualClusterID, err = getClusterIDFromName(clusterName, orgID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedClusterID, actualClusterID)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns error from api if listing cluster fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errTest)
		actualClusterID, err = getClusterIDFromName(clusterName, orgID, mockClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, "", actualClusterID)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster does not exist in organization", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, nil)
		actualClusterID, err = getClusterIDFromName(clusterName, orgID, mockClient)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "cluster_name: test-cluster does not exist in organization: test-org-id")
		assert.Equal(t, "", actualClusterID)
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
