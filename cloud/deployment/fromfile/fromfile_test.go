package fromfile

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

var errTest = errors.New("test error")

type Suite struct {
	suite.Suite
}

func TestCloudDeploymentFromFileSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreateOrUpdate() {
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

	s.Run("common across create or update", func() {
		s.Run("returns an error if file does not exist", func() {
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil)
			s.ErrorContains(err, "open deployment.yaml: no such file or directory")
		})
		s.Run("returns an error if file exists but user provides incorrect path", func() {
			filePath = "./2/deployment.yaml"
			data = "test"
			err = fileutil.WriteStringToFile(filePath, data)
			s.NoError(err)
			defer afero.NewOsFs().RemoveAll("./2")
			err = CreateOrUpdate("1/deployment.yaml", "create", nil, nil)
			s.ErrorContains(err, "open 1/deployment.yaml: no such file or directory")
		})
		s.Run("returns an error if file is empty", func() {
			filePath = "./deployment.yaml"
			data = ""
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil)
			s.ErrorIs(err, errEmptyFile)
			s.ErrorContains(err, "deployment.yaml has no content")
		})
		s.Run("returns an error if unmarshalling fails", func() {
			filePath = "./deployment.yaml"
			data = "test"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil)
			s.ErrorContains(err, "error unmarshaling JSON:")
		})
		s.Run("returns an error if required fields are missing", func() {
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
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil)
			s.ErrorContains(err, "missing required field: deployment.configuration.name")
		})
		s.Run("returns an error if getting context fails", func() {
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
    executor: CeleryExecutor
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
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorContains(err, "no context set")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if cluster does not exist", func() {
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
    executor: CeleryExecutor
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
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errNotFound)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if listing cluster fails", func() {
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
    executor: CeleryExecutor
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
			mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errTest)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if listing deployment fails", func() {
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
    executor: CeleryExecutor
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
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errTest)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("does not update environment variables if input is empty", func() {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)
			filePath = "./deployment.yaml"
			data = `{
    "deployment": {
        "environment_variables": [],
        "configuration": {
            "name": "test-deployment-label",
            "description": "description",
            "runtime_version": "6.0.0",
            "dag_deploy_enabled": true,
            "executor": "CeleryExecutor",
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
			createdDeployment = astro.Deployment{
				ID:    "test-deployment-id",
				Label: "test-deployment-label",
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, nil)
			mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, out)
			s.NoError(err)
			s.NotNil(out)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("does not update alert emails if input is empty", func() {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)
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
    executor: CeleryExecutor
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
  alert_emails: []
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
			createdDeployment = astro.Deployment{
				ID:    "test-deployment-id",
				Label: "test-deployment-label",
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
			mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, out)
			s.NoError(err)
			s.NotNil(out)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error from the api if creating environment variables fails", func() {
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
            "executor": "CeleryExecutor",
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errTest)
			s.ErrorContains(err, "\n failed to create alert emails")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error from the api if creating alert emails fails", func() {
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
            "executor": "CeleryExecutor",
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errTest)
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when action is create", func() {
		s.Run("reads the yaml file and creates a deployment", func() {
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
    executor: CeleryExecutor
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), "configuration:\n        name: "+createdDeployment.Label)
			s.Contains(out.String(), "metadata:\n        deployment_id: "+createdDeployment.ID)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("reads the json file and creates a deployment", func() {
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
            "executor": "CeleryExecutor",
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "test-workspace-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), "\"configuration\": {\n            \"name\": \""+createdDeployment.Label+"\"")
			s.Contains(out.String(), "\"metadata\": {\n            \"deployment_id\": \""+createdDeployment.ID+"\"")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if workspace does not exist", func() {
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
    executor: CeleryExecutor
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
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errNotFound)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if listing workspace fails", func() {
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
    executor: CeleryExecutor
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
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errTest)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if deployment already exists", func() {
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
    executor: CeleryExecutor
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
			mockClient.On("ListDeployments", orgID, "").Return(existingDeployments, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorContains(err, "deployment: test-deployment-label already exists: use deployment update --deployment-file deployment.yaml instead")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if creating deployment input fails", func() {
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
            "executor": "CeleryExecutor",
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
            "email1@test.com",
            "email2@test.com"
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.Error(err)
			s.ErrorContains(err, "worker queue option is invalid: worker concurrency")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error from the api if create deployment fails", func() {
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
            "executor": "CeleryExecutor",
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
            "email1@test.com",
            "email2@test.com"
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
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, nil)
			s.ErrorIs(err, errCreateFailed)
			s.ErrorContains(err, "test error: failed to create deployment with input")
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when action is update", func() {
		s.Run("reads the yaml file and updates an existing deployment", func() {
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
    description: description 1
    runtime_version: 6.0.0
    dag_deploy_enabled: true
    executor: CeleryExecutor
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
			existingDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description",
				Cluster: astro.Cluster{
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
			}
			updatedDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description 1",
			}
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(updatedDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), "configuration:\n        name: "+existingDeployment.Label)
			s.Contains(out.String(), "\n        description: "+updatedDeployment.Description)
			s.Contains(out.String(), "metadata:\n        deployment_id: "+existingDeployment.ID)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("reads the json file and updates an existing deployment", func() {
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
            "executor": "CeleryExecutor",
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
			existingDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description",
				Cluster: astro.Cluster{
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
			}
			updatedDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description 1",
			}
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(updatedDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), "\"configuration\": {\n            \"name\": \""+existingDeployment.Label+"\"")
			s.Contains(out.String(), "\n            \"description\": \""+updatedDeployment.Description+"\"")
			s.Contains(out.String(), "\"metadata\": {\n            \"deployment_id\": \""+existingDeployment.ID+"\"")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if deployment does not exist", func() {
			testUtil.InitTestConfig(testUtil.CloudPlatform)
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
    executor: CeleryExecutor
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
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, nil)
			s.ErrorContains(err, "deployment: test-deployment-label does not exist: use deployment create --deployment-file deployment.yaml instead")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if creating update deployment input fails", func() {
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
            "executor": "CeleryExecutor",
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
            "email1@test.com",
            "email2@test.com"
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
			existingDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description",
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
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, nil)
			s.Error(err)
			s.ErrorContains(err, "worker queue option is invalid: worker concurrency")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error from the api if update deployment fails", func() {
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
            "executor": "CeleryExecutor",
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
            "email1@test.com",
            "email2@test.com"
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
			existingDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description",
				Cluster: astro.Cluster{
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
			mockClient.On("ListClusters", orgID).Return(existingClusters, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, nil)
			s.ErrorIs(err, errUpdateFailed)
			s.ErrorContains(err, "test error: failed to update deployment with input")
			mockClient.AssertExpectations(s.T())
		})
	})
}

func (s *Suite) TestGetCreateOrUpdateInput() {
	var (
		expectedDeploymentInput, actualCreateInput       astro.CreateDeploymentInput
		expectedUpdateDeploymentInput, actualUpdateInput astro.UpdateDeploymentInput
		deploymentFromFile                               inspect.FormattedDeployment
		qList                                            []inspect.Workerq
		existingPools                                    []astro.NodePool
		expectedQList                                    []astro.WorkerQueue
		clusterID, workspaceID, deploymentID             string
		err                                              error
		mockWorkerQueueDefaultOptions                    astro.WorkerQueueDefaultOptions
	)
	clusterID = "test-cluster-id"
	workspaceID = "test-workspace-id"
	s.Run("common across create and update", func() {
		s.Run("returns error if worker type does not match existing pools", func() {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			minCount := 3
			qList = []inspect.Workerq{
				{
					Name:              "default",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
					WorkerConcurrency: 200,
					WorkerType:        "test-worker-1",
				},
				{
					Name:              "test-q-2",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
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
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
			s.ErrorContains(err, "worker_type: test-worker-8 does not exist in cluster: test-cluster")
			s.Equal(expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("when executor is Celery", func() {
			s.Run("returns error if queue options are invalid", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
				minCountThirty := 30
				minCountThree := 3
				qList = []inspect.Workerq{
					{
						Name:              "default",
						MaxWorkerCount:    16,
						MinWorkerCount:    &minCountThirty,
						WorkerConcurrency: 200,
						WorkerType:        "test-worker-1",
					},
					{
						Name:              "test-q-2",
						MaxWorkerCount:    16,
						MinWorkerCount:    &minCountThree,
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "worker queue option is invalid: min worker count")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
			s.Run("returns error if getting worker queue options fails", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
				minCountThirty := 30
				minCountThree := 3
				qList = []inspect.Workerq{
					{
						Name:              "default",
						MaxWorkerCount:    16,
						MinWorkerCount:    &minCountThirty,
						WorkerConcurrency: 200,
						WorkerType:        "test-worker-1",
					},
					{
						Name:              "test-q-2",
						MaxWorkerCount:    16,
						MinWorkerCount:    &minCountThree,
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "failed to get worker queue default options")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
			s.Run("sets default queue options if none were requested", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
				minCount := -1
				qList = []inspect.Workerq{
					{
						Name:           "default",
						WorkerType:     "test-worker-1",
						MinWorkerCount: &minCount,
					},
					{
						Name:           "test-q-2",
						WorkerType:     "test-worker-2",
						MinWorkerCount: &minCount,
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
						Floor:   0,
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
						Executor: deployment.CeleryExecutor,
						Scheduler: astro.Scheduler{
							AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
							Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
						},
					},
					WorkerQueues: expectedQList,
				}
				mockClient := new(astro_mocks.Client)
				mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.NoError(err)
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
		})
		s.Run("when executor is Kubernetes", func() {
			s.Run("returns an error if more than one worker queue are requested", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				qList = []inspect.Workerq{
					{
						Name:       "default",
						WorkerType: "test-worker-1",
					},
					{
						Name:       "test-q-2",
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
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "KubernetesExecutor does not support more than one worker queue. (2) were requested")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
			s.Run("returns an error if Celery queue property min_worker_count is requested", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				minCount := 10
				qList = []inspect.Workerq{
					{
						Name:           "default",
						WorkerType:     "test-worker-1",
						MinWorkerCount: &minCount,
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "KubernetesExecutor does not support minimum worker count in the request. It can only be used with CeleryExecutor")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
			s.Run("returns an error if Celery queue property max_worker_count is requested", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				qList = []inspect.Workerq{
					{
						Name:           "default",
						WorkerType:     "test-worker-1",
						MaxWorkerCount: 10,
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "KubernetesExecutor does not support maximum worker count in the request. It can only be used with CeleryExecutor")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
			s.Run("returns an error if Celery queue property worker_concurrency is requested", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				qList = []inspect.Workerq{
					{
						Name:              "default",
						WorkerType:        "test-worker-1",
						WorkerConcurrency: 10,
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "KubernetesExecutor does not support worker concurrency in the request. It can only be used with CeleryExecutor")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
			s.Run("returns an error if invalid input is requested", func() {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				qList = []inspect.Workerq{
					{
						Name:       "default",
						WorkerType: "test-worker-1",
						PodRAM:     "lots",
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
				s.ErrorContains(err, "KubernetesExecutor does not support pod ram in the request. It will be calculated based on the requested worker type")
				s.Equal(expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(s.T())
			})
		})
	})
	s.Run("when action is to create", func() {
		s.Run("transforms formattedDeployment to CreateDeploymentInput if no queues were requested", func() {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor

			expectedDeploymentInput = astro.CreateDeploymentInput{
				WorkspaceID:           workspaceID,
				ClusterID:             clusterID,
				Label:                 deploymentFromFile.Deployment.Configuration.Name,
				Description:           deploymentFromFile.Deployment.Configuration.Description,
				RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				DagDeployEnabled:      deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
				DeploymentSpec: astro.DeploymentCreateSpec{
					Executor: deployment.CeleryExecutor,
					Scheduler: astro.Scheduler{
						AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
						Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
					},
				},
				WorkerQueues: nil,
			}
			mockClient := new(astro_mocks.Client)
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, nil, mockClient)
			s.NoError(err)
			s.Equal(expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("transforms formattedDeployment to CreateDeploymentInput if Kubernetes executor was requested", func() {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
			qList = []inspect.Workerq{
				{
					Name:       "default",
					WorkerType: "test-worker-1",
				},
			}
			deploymentFromFile.Deployment.WorkerQs = qList
			expectedQList = []astro.WorkerQueue{
				{
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-pool-id",
				},
			}
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
			expectedDeploymentInput = astro.CreateDeploymentInput{
				WorkspaceID:           workspaceID,
				ClusterID:             clusterID,
				Label:                 deploymentFromFile.Deployment.Configuration.Name,
				Description:           deploymentFromFile.Deployment.Configuration.Description,
				RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				DagDeployEnabled:      deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
				DeploymentSpec: astro.DeploymentCreateSpec{
					Executor: deployment.KubeExecutor,
					Scheduler: astro.Scheduler{
						AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
						Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
					},
				},
				WorkerQueues: expectedQList,
			}
			mockClient := new(astro_mocks.Client)
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
			s.NoError(err)
			s.Equal(expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns correct deployment input when multiple queues are requested", func() {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			minCount := 3
			qList = []inspect.Workerq{
				{
					Name:              "default",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
					WorkerConcurrency: 200,
					WorkerType:        "test-worker-1",
				},
				{
					Name:              "test-q-2",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
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
					Executor: deployment.CeleryExecutor,
					Scheduler: astro.Scheduler{
						AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
						Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
					},
				},
				WorkerQueues: expectedQList,
			}
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, mockClient)
			s.NoError(err)
			s.Equal(expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when action is to update", func() {
		s.Run("transforms formattedDeployment to UpdateDeploymentInput if no queues were requested", func() {
			deploymentID = "test-deployment-id"
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			existingDeployment := astro.Deployment{
				ID:    deploymentID,
				Label: "test-deployment",
				Cluster: astro.Cluster{
					ID: "test-cluster-id",
				},
			}

			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{
				ID:               deploymentID,
				ClusterID:        clusterID,
				Label:            deploymentFromFile.Deployment.Configuration.Name,
				Description:      deploymentFromFile.Deployment.Configuration.Description,
				DagDeployEnabled: deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, nil, mockClient)
			s.NoError(err)
			s.Equal(expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if the cluster is being changed", func() {
			deploymentID = "test-deployment-id"
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster-1"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			existingDeployment := astro.Deployment{
				ID:    deploymentID,
				Label: "test-deployment",
				Cluster: astro.Cluster{
					ID: "test-cluster-id",
				},
			}

			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			mockClient := new(astro_mocks.Client)
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, "diff-cluster", workspaceID, "update", &existingDeployment, nil, mockClient)
			s.ErrorIs(err, errNotPermitted)
			s.ErrorContains(err, "changing an existing deployment's cluster is not permitted")
			s.Equal(expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with no queues", func() {
			deploymentID = "test-deployment-id"
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor

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
			existingDeployment := astro.Deployment{
				ID:    deploymentID,
				Label: "test-deployment",
				Cluster: astro.Cluster{
					ID:        "test-cluster-id",
					NodePools: existingPools,
				},
				DeploymentSpec: astro.DeploymentSpec{
					Executor: deployment.CeleryExecutor,
				},
				WorkerQueues: expectedQList,
			}

			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{
				ID:               deploymentID,
				ClusterID:        clusterID,
				Label:            deploymentFromFile.Deployment.Configuration.Name,
				Description:      deploymentFromFile.Deployment.Configuration.Description,
				DagDeployEnabled: deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
				DeploymentSpec: astro.DeploymentCreateSpec{
					Executor: deploymentFromFile.Deployment.Configuration.Executor,
					Scheduler: astro.Scheduler{
						AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
						Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
					},
				},
				WorkerQueues: nil, // a default queue is created by the api
			}
			mockClient := new(astro_mocks.Client)
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, nil, mockClient)
			s.NoError(err)
			s.Equal(expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with a queue", func() {
			deploymentID = "test-deployment-id"
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
			qList = []inspect.Workerq{
				{
					Name:       "default",
					WorkerType: "test-worker-1",
				},
			}
			deploymentFromFile.Deployment.WorkerQs = qList
			expectedQList = []astro.WorkerQueue{
				{
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-pool-id",
				},
			}
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
			existingDeployment := astro.Deployment{
				ID:    deploymentID,
				Label: "test-deployment",
				Cluster: astro.Cluster{
					ID:        "test-cluster-id",
					Name:      "test-cluster",
					NodePools: existingPools,
				},
				DeploymentSpec: astro.DeploymentSpec{
					Executor: deployment.CeleryExecutor,
				},
				WorkerQueues: expectedQList,
			}

			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{
				ID:               deploymentID,
				ClusterID:        clusterID,
				Label:            deploymentFromFile.Deployment.Configuration.Name,
				Description:      deploymentFromFile.Deployment.Configuration.Description,
				DagDeployEnabled: deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
				DeploymentSpec: astro.DeploymentCreateSpec{
					Executor: deploymentFromFile.Deployment.Configuration.Executor,
					Scheduler: astro.Scheduler{
						AU:       deploymentFromFile.Deployment.Configuration.SchedulerAU,
						Replicas: deploymentFromFile.Deployment.Configuration.SchedulerCount,
					},
				},
				WorkerQueues: expectedQList,
			}
			mockClient := new(astro_mocks.Client)
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, mockClient)
			s.NoError(err)
			s.Equal(expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns correct update deployment input when multiple queues are requested", func() {
			deploymentID = "test-deployment-id"
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			minCount := 3
			qList = []inspect.Workerq{
				{
					Name:              "default",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
					WorkerConcurrency: 200,
					WorkerType:        "test-worker-1",
				},
				{
					Name:              "test-q-2",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
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
			existingDeployment := astro.Deployment{
				ID:    deploymentID,
				Label: "test-deployment",
				Cluster: astro.Cluster{
					ID: "test-cluster-id",
				},
				WorkerQueues: expectedQList,
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

			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{
				ID:               deploymentID,
				ClusterID:        clusterID,
				Label:            deploymentFromFile.Deployment.Configuration.Name,
				Description:      deploymentFromFile.Deployment.Configuration.Description,
				DagDeployEnabled: deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, mockClient)
			s.NoError(err)
			s.Equal(expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(s.T())
		})
	})
}

func (s *Suite) TestCheckRequiredFields() {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	input.Deployment.Configuration.Description = "test-description"
	s.Run("returns an error if name is missing", func() {
		err = checkRequiredFields(&input, "")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.configuration.name")
	})
	s.Run("returns an error if cluster_name is missing", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		err = checkRequiredFields(&input, "")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.configuration.cluster_name")
	})
	s.Run("returns an error if executor is missing", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		err = checkRequiredFields(&input, "")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.configuration.executor")
	})
	s.Run("returns an error if executor value is invalid", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = "test-executor"
		err = checkRequiredFields(&input, "")
		s.ErrorIs(err, errInvalidValue)
		s.ErrorContains(err, "is not valid. It can either be CeleryExecutor or KubernetesExecutor")
	})
	s.Run("returns an error if alert email is invalid", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		list := []string{"test@test.com", "testing@testing.com", "not-an-email"}
		input.Deployment.AlertEmails = list
		err = checkRequiredFields(&input, "")
		s.ErrorIs(err, errInvalidEmail)
		s.ErrorContains(err, "invalid email: not-an-email")
	})
	s.Run("returns an error if env var keys are missing on create", func() {
		input = inspect.FormattedDeployment{}
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "",
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
		input.Deployment.EnvVars = list
		err = checkRequiredFields(&input, "create")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.environment_variables[0].key")
	})
	s.Run("if queues were requested, it returns an error if queue name is missing", func() {
		input = inspect.FormattedDeployment{}
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		qList := []inspect.Workerq{
			{
				Name:       "",
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input, "create")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.worker_queues[0].name")
	})
	s.Run("if queues were requested, it returns an error if default queue is missing", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		qList := []inspect.Workerq{
			{
				Name:       "test-q-1",
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input, "create")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.worker_queues[0].name = default")
	})
	s.Run("if queues were requested, it returns an error if worker type is missing", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		qList := []inspect.Workerq{
			{
				Name: "default",
			},
			{
				Name:       "default",
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input, "create")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.worker_queues[0].worker_type")
	})
	s.Run("returns nil if there are no missing fields", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		qList := []inspect.Workerq{
			{
				Name:       "default",
				WorkerType: "test-worker-1",
			},
			{
				Name:       "test-q-2",
				WorkerType: "test-worker-2",
			},
		}
		input.Deployment.WorkerQs = qList
		err = checkRequiredFields(&input, "create")
		s.NoError(err)
	})
}

func (s *Suite) TestDeploymentExists() {
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
	s.Run("returns true if deployment already exists", func() {
		actual = deploymentExists(existingDeployments, deploymentToCreate)
		s.True(actual)
	})
	s.Run("returns false if deployment does not exist", func() {
		deploymentToCreate = "test-d-2"
		actual = deploymentExists(existingDeployments, deploymentToCreate)
		s.False(actual)
	})
}

func (s *Suite) TestGetClusterFromName() {
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
	s.Run("returns a cluster id if cluster exists in organization", func() {
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
		s.NoError(err)
		s.Equal(expectedClusterID, actualClusterID)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns error from api if listing cluster fails", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errTest)
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, orgID, mockClient)
		s.ErrorIs(err, errTest)
		s.Equal("", actualClusterID)
		s.Equal([]astro.NodePool(nil), actualNodePools)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if cluster does not exist in organization", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, nil)
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, orgID, mockClient)
		s.ErrorIs(err, errNotFound)
		s.ErrorContains(err, "cluster_name: test-cluster does not exist in organization: test-org-id")
		s.Equal("", actualClusterID)
		s.Equal([]astro.NodePool(nil), actualNodePools)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetWorkspaceIDFromName() {
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
	s.Run("returns a workspace id if workspace exists in organization", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", orgID).Return(existingWorkspaces, nil)
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockClient)
		s.NoError(err)
		s.Equal(expectedWorkspaceID, actualWorkspaceID)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns error from api if listing workspace fails", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, errTest)
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockClient)
		s.ErrorIs(err, errTest)
		s.Equal("", actualWorkspaceID)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if workspace does not exist in organization", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", orgID).Return([]astro.Workspace{}, nil)
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockClient)
		s.ErrorIs(err, errNotFound)
		s.ErrorContains(err, "workspace_name: test-workspace does not exist in organization: test-org-id")
		s.Equal("", actualWorkspaceID)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetNodePoolIDFromName() {
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
	s.Run("returns a nodepool id from cluster for pool with matching worker type", func() {
		actualPoolID, err = getNodePoolIDFromWorkerType(workerType, clusterID, existingPools)
		s.NoError(err)
		s.Equal(expectedPoolID, actualPoolID)
	})
	s.Run("returns an error if no pool with matching worker type exists in the cluster", func() {
		workerType = "worker-3"
		actualPoolID, err = getNodePoolIDFromWorkerType(workerType, clusterID, existingPools)
		s.ErrorIs(err, errNotFound)
		s.ErrorContains(err, "worker_type: worker-3 does not exist in cluster: test-cluster")
		s.Equal("", actualPoolID)
	})
}

func (s *Suite) TestHasEnvVars() {
	s.Run("returns true if there are env vars in the deployment", func() {
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
		s.True(actual)
	})
	s.Run("returns false if there are no env vars in the deployment", func() {
		var deploymentFromFile inspect.FormattedDeployment
		actual := hasEnvVars(&deploymentFromFile)
		s.False(actual)
	})
}

func (s *Suite) TestCreateEnvVars() {
	var (
		expectedEnvVarsInput astro.EnvironmentVariablesInput
		actualEnvVars        []astro.EnvironmentVariablesObject
		deploymentFromFile   inspect.FormattedDeployment
		err                  error
	)
	s.Run("creates env vars if they were requested in a formatted deployment", func() {
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
		s.NoError(err)
		s.Equal(mockResponse, actualEnvVars)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns api error if modifyDeploymentVariable fails", func() {
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
		s.ErrorIs(err, errTest)
		s.Equal(mockResponse, actualEnvVars)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestHasQueues() {
	s.Run("returns true if there are worker queues in the deployment", func() {
		var deploymentFromFile inspect.FormattedDeployment
		minCount := 3
		qList := []inspect.Workerq{
			{
				Name:              "default",
				MaxWorkerCount:    16,
				MinWorkerCount:    &minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    &minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		actual := hasQueues(&deploymentFromFile)
		s.True(actual)
	})
	s.Run("returns false if there are no worker queues in the deployment", func() {
		var deploymentFromFile inspect.FormattedDeployment
		actual := hasQueues(&deploymentFromFile)
		s.False(actual)
	})
}

func (s *Suite) TestGetQueues() {
	var (
		deploymentFromFile           inspect.FormattedDeployment
		actualWQList, existingWQList []astro.WorkerQueue
		existingPools                []astro.NodePool
		err                          error
	)
	s.Run("when the executor is celery", func() {
		s.Run("returns list of queues for the requested deployment", func() {
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
			minCount := 3
			qList := []inspect.Workerq{
				{
					Name:              "default",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
					WorkerConcurrency: 20,
					WorkerType:        "test-worker-1",
				},
				{
					Name:              "test-q-2",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCount,
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
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, []astro.WorkerQueue(nil))
			s.NoError(err)
			s.Equal(expectedWQList, actualWQList)
		})
		s.Run("returns updated list of existing and queues being added", func() {
			existingWQList = []astro.WorkerQueue{
				{
					ID:                "q-id",
					Name:              "default",
					IsDefault:         true,
					MaxWorkerCount:    16,
					MinWorkerCount:    3,
					WorkerConcurrency: 20,
					NodePoolID:        "test-pool-id",
				},
			}
			expectedWQList := []astro.WorkerQueue{
				{
					ID:                "q-id",
					Name:              "default",
					IsDefault:         true,
					MaxWorkerCount:    18,
					MinWorkerCount:    4,
					WorkerConcurrency: 25,
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
			minCountThree := 3
			minCountFour := 4
			qList := []inspect.Workerq{
				{
					Name:              "default",
					MaxWorkerCount:    18,
					MinWorkerCount:    &minCountFour,
					WorkerConcurrency: 25,
					WorkerType:        "test-worker-1",
				},
				{
					Name:              "test-q-2",
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCountThree,
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
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
			s.NoError(err)
			s.Equal(expectedWQList, actualWQList)
		})
		s.Run("returns updated list when multiple queue operations are requested", func() {
			existingWQList = []astro.WorkerQueue{
				{
					ID:                "q-id",
					Name:              "default", // this queue is getting updated
					IsDefault:         true,
					MaxWorkerCount:    16,
					MinWorkerCount:    3,
					WorkerConcurrency: 20,
					NodePoolID:        "test-pool-id",
				},
				{
					ID:                "q-id-1",
					Name:              "q-1", // this queue is getting deleted
					IsDefault:         false,
					MaxWorkerCount:    12,
					MinWorkerCount:    4,
					WorkerConcurrency: 22,
					NodePoolID:        "test-pool-id-2",
				},
			}
			expectedWQList := []astro.WorkerQueue{
				{
					ID:                "q-id",
					Name:              "default",
					IsDefault:         true,
					MaxWorkerCount:    18,
					MinWorkerCount:    4,
					WorkerConcurrency: 25,
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
			minCountThree := 3
			minCountFour := 4
			qList := []inspect.Workerq{
				{
					Name:              "default",
					MaxWorkerCount:    18,
					MinWorkerCount:    &minCountFour,
					WorkerConcurrency: 25,
					WorkerType:        "test-worker-1",
				},
				{
					Name:              "test-q-2", // this queue is being added
					MaxWorkerCount:    16,
					MinWorkerCount:    &minCountThree,
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
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
			s.NoError(err)
			s.Equal(expectedWQList, actualWQList)
		})
	})
	s.Run("when the executor is kubernetes", func() {
		s.Run("returns one default queue regardless of any existing queues", func() {
			existingWQList = []astro.WorkerQueue{
				{
					ID:                "q-id",
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
			expectedWQList := []astro.WorkerQueue{
				{
					Name:       "default",
					IsDefault:  true,
					PodCPU:     "0.1",
					PodRAM:     "0.25Gi",
					NodePoolID: "test-pool-id",
				},
			}
			qList := []inspect.Workerq{
				{
					Name:       "default",
					PodCPU:     "0.1",
					PodRAM:     "0.25Gi",
					WorkerType: "test-worker-1",
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
			deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
			s.NoError(err)
			s.Equal(expectedWQList, actualWQList)
		})
	})
	s.Run("returns an error if unable to determine nodepool id", func() {
		minCount := 3
		qList := []inspect.Workerq{
			{
				Name:              "default",
				MaxWorkerCount:    16,
				MinWorkerCount:    &minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    &minCount,
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
		deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
		actualWQList, err = getQueues(&deploymentFromFile, existingPools, []astro.WorkerQueue(nil))
		s.ErrorContains(err, "worker_type: test-worker-4 does not exist in cluster: test-cluster")
		s.Equal([]astro.WorkerQueue(nil), actualWQList)
	})
}

func (s *Suite) TestHasAlertEmails() {
	s.Run("returns true if there are env vars in the deployment", func() {
		var deploymentFromFile inspect.FormattedDeployment
		list := []string{"test@test.com", "testing@testing.com"}
		deploymentFromFile.Deployment.AlertEmails = list
		actual := hasAlertEmails(&deploymentFromFile)
		s.True(actual)
	})
	s.Run("returns false if there are no env vars in the deployment", func() {
		var deploymentFromFile inspect.FormattedDeployment
		actual := hasAlertEmails(&deploymentFromFile)
		s.False(actual)
	})
}

func (s *Suite) TestCreateAlertEmails() {
	var (
		deploymentFromFile     inspect.FormattedDeployment
		expectedInput          astro.UpdateDeploymentAlertsInput
		expected, actual       astro.DeploymentAlerts
		existingEmails, emails []string
		deploymentID           string
		err                    error
	)
	s.Run("updates alert emails for a deployment when no alert emails exist", func() {
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
		s.NoError(err)
		s.Equal(expected, actual)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("updates alert emails for a deployment with new and existing alert emails", func() {
		existingEmails = []string{
			"test1@email.com",
			"test2@email.com", // this is getting deleted
		}
		emails = []string{
			existingEmails[0],
			"test3@email.com", // this is getting added
		}
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
		s.NoError(err)
		s.Equal(expected, actual)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns api error if updating deployment alert email fails", func() {
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
		s.Error(err)
		s.Equal(expected, actual)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestIsJSON() {
	var (
		valid, invalid string
		actual         bool
	)
	s.Run("returns true for valid json", func() {
		valid = `{"test":"yay"}`
		actual = isJSON([]byte(valid))
		s.True(actual)
	})
	s.Run("returns false for invalid json", func() {
		invalid = `-{"test":"yay",{}`
		actual = isJSON([]byte(invalid))
		s.False(actual)
	})
}

func (s *Suite) TestDeploymentFromName() {
	var (
		existingDeployments       []astro.Deployment
		deploymentToCreate        string
		actual, expectedeployment astro.Deployment
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
	expectedeployment = astro.Deployment{
		ID:          "test-d-2",
		Label:       "test-deployment-2",
		Description: "deployment 2",
	}
	deploymentToCreate = "test-deployment-2"
	s.Run("returns the deployment id for the matching deployment name", func() {
		actual = deploymentFromName(existingDeployments, deploymentToCreate)
		s.Equal(expectedeployment, actual)
	})
	s.Run("returns empty string if deployment name does not match", func() {
		deploymentToCreate = "test-d-2"
		expectedeployment = astro.Deployment{}
		actual = deploymentFromName(existingDeployments, deploymentToCreate)
		s.Equal(expectedeployment, actual)
	})
}

func (s *Suite) TestIsValidEmail() {
	var (
		actual     bool
		emailInput string
	)
	s.Run("returns true if email is valid", func() {
		emailInput = "test123@superomain.cool.com"
		actual = isValidEmail(emailInput)
		s.True(actual)
	})
	s.Run("returns false if email is invalid", func() {
		emailInput = "invalid-email.com"
		actual = isValidEmail(emailInput)
		s.False(actual)
	})
}

func (s *Suite) TestValidateAlertEmails() {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	s.Run("returns an error if alert email is invalid", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []string{"test@test.com", "testing@testing.com", "not-an-email"}
		input.Deployment.AlertEmails = list
		err = checkAlertEmails(&input)
		s.ErrorIs(err, errInvalidEmail)
		s.ErrorContains(err, "invalid email: not-an-email")
	})
	s.Run("returns nil if alert email is valid", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []string{"test@test.com", "testing@testing.com"}
		input.Deployment.AlertEmails = list
		err = checkAlertEmails(&input)
		s.NoError(err)
	})
}

func (s *Suite) TestCheckEnvVars() {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	s.Run("returns an error if env var keys are missing on create", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "",
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
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "create")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.environment_variables[0].key")
	})
	s.Run("returns an error if env var values are missing on create", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
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
				Value:     "",
			},
		}
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "create")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.environment_variables[1].value")
	})
	s.Run("returns an error if env var keys are missing on update", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
				Value:     "val-1",
			},
			{
				IsSecret:  true,
				Key:       "",
				UpdatedAt: "",
				Value:     "val-2",
			},
		}
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "update")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.environment_variables[1].key")
	})
	s.Run("returns nil if env var values are missing on update", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
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
				Value:     "",
			},
		}
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "update")
		s.NoError(err)
	})
}

func (s *Suite) TestIsValidExecutor() {
	s.Run("returns true if executor is Celery", func() {
		actual := isValidExecutor(deployment.CeleryExecutor)
		s.True(actual)
	})
	s.Run("returns true if executor is Kubernetes", func() {
		actual := isValidExecutor(deployment.KubeExecutor)
		s.True(actual)
	})
	s.Run("returns false if executor is neither Celery nor Kubernetes", func() {
		actual := isValidExecutor("test-executor")
		s.False(actual)
	})
}
