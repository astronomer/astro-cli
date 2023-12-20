package fromfile

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

const (
	mockOrgShortName = "test-org-short-name"
)

var (
	errTest                    = errors.New("test error")
	limit                      = 1000
	workloadIdentity           = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	mockCoreDeploymentResponse = []astrocore.Deployment{
		{
			Status:           "HEALTHY",
			WorkloadIdentity: &workloadIdentity,
		},
	}
	mockListDeploymentsResponse = astrocore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	clusterType       = []astrocore.ListClustersParamsTypes{astrocore.BRINGYOUROWNCLOUD, astrocore.HOSTED}
	clusterListParams = &astrocore.ListClustersParams{
		Types: &clusterType,
		Limit: &limit,
	}
	mockListClustersResponse = astrocore.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ClustersPaginated{
			Clusters: []astrocore.Cluster{
				{
					Id:   "test-cluster-id",
					Name: "test-cluster",
					NodePools: []astrocore.NodePool{
						{
							Id:               "test-pool-id",
							IsDefault:        false,
							NodeInstanceType: "test-worker-1",
						},
						{
							Id:               "test-pool-id-2",
							IsDefault:        false,
							NodeInstanceType: "test-worker-2",
						},
					},
				},
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
		},
	}
	description = "test workspace"
	workspace1  = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &description,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "test-ws-id",
	}

	workspaces = []astrocore.Workspace{
		workspace1,
	}

	ListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}

	EmptyListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: []astrocore.Workspace{},
		},
	}
)

func TestCreateOrUpdate(t *testing.T) {
	var (
		err                           error
		filePath, data, orgID         string
		mockWorkerQueueDefaultOptions astro.WorkerQueueDefaultOptions
		emails                        []string
		mockAlertEmailResponse        astro.DeploymentAlerts
		createdDeployment             astro.Deployment
	)

	t.Run("common across create or update", func(t *testing.T) {
		t.Run("returns an error if file does not exist", func(t *testing.T) {
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil)
			assert.ErrorContains(t, err, "open deployment.yaml: no such file or directory")
		})
		t.Run("returns an error if file exists but user provides incorrect path", func(t *testing.T) {
			filePath = "./2/deployment.yaml"
			data = "test"
			err = fileutil.WriteStringToFile(filePath, data)
			assert.NoError(t, err)
			defer afero.NewOsFs().RemoveAll("./2")
			err = CreateOrUpdate("1/deployment.yaml", "create", nil, nil, nil)
			assert.ErrorContains(t, err, "open 1/deployment.yaml: no such file or directory")
		})
		t.Run("returns an error if file is empty", func(t *testing.T) {
			filePath = "./deployment.yaml"
			data = ""
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil)
			assert.ErrorIs(t, err, errEmptyFile)
			assert.ErrorContains(t, err, "deployment.yaml has no content")
		})
		t.Run("returns an error if unmarshalling fails", func(t *testing.T) {
			filePath = "./deployment.yaml"
			data = "test"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil)
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
    deployment_type: HYBRID
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
    workloadIdentity: astro-great-release-name@provider-account.iam.gserviceaccount.com
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
`
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil)
			assert.ErrorContains(t, err, "missing required field: deployment.configuration.name")
		})
		t.Run("returns an error if getting context fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.ErrorReturningContext)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    workloadIdentity: astro-great-release-name@provider-account.iam.gserviceaccount.com
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`

			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorContains(t, err, "no context set")
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error if cluster does not exist", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    workloadIdentity: astro-great-release-name@provider-account.iam.gserviceaccount.com
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errNotFound)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if listing cluster fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    workloadIdentity: astro-great-release-name@provider-account.iam.gserviceaccount.com
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&astrocore.ListClustersResponse{}, errTest).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errTest)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if listing deployment fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    workloadIdentity: astro-great-release-name@provider-account.iam.gserviceaccount.com
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errTest)
			mockClient.AssertExpectations(t)
			mockClient.AssertExpectations(t)
		})
		t.Run("does not update environment variables if input is empty", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
            "deployment_type": "HYBRID"
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
            "workloadIdentity": "astro-great-release-name@provider-account.iam.gserviceaccount.com",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
			createdDeployment = astro.Deployment{
				ID:    "test-deployment-id",
				Label: "test-deployment-label",
			}
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
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
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, nil)
			mockClient.On("ListDeployments", orgID, "test-ws-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.NotNil(t, out)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("does not update alert emails if input is empty", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    workloadIdentity: astro-great-release-name@provider-account.iam.gserviceaccount.com
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails: []
`
			createdDeployment = astro.Deployment{
				ID:    "test-deployment-id",
				Label: "test-deployment-label",
			}
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
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
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
			mockClient.On("ListDeployments", orgID, "test-ws-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.NotNil(t, out)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error from the api if creating environment variables fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
            "deployment_type": "HYBRID"
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
            "workloadIdentity": "astro-great-release-name@provider-account.iam.gserviceaccount.com",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
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
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errTest)
			assert.ErrorContains(t, err, "\n failed to create alert emails")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error from the api if creating alert emails fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
            "deployment_type": "HYBRID"
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
            "workloadIdentity": "astro-great-release-name@provider-account.iam.gserviceaccount.com",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
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
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errTest)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
	})
	t.Run("when action is create", func(t *testing.T) {
		t.Run("reads the yaml file and creates a deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    executor: CeleryExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
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
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "test-ws-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "configuration:\n        name: "+createdDeployment.Label)
			assert.Contains(t, out.String(), "metadata:\n        deployment_id: "+createdDeployment.ID)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("reads the yaml file and creates a hosted dedicated deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HOSTED_DEDICATED
    is_high_availability: true
    ci_cd_enforcement: true
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 10
      worker_type: a5
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 10
      worker_type: a5
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
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
				Type:  "HOSTED_DEDICATED",
			}
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				AstroMachines: []astro.Machine{
					{
						Type:               "a5",
						ConcurrentTasks:    5,
						ConcurrentTasksMax: 15,
					},
					{
						Type:               "a10",
						ConcurrentTasks:    10,
						ConcurrentTasksMax: 30,
					},
					{
						Type:               "a20",
						ConcurrentTasks:    20,
						ConcurrentTasksMax: 60,
					},
				},
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "test-ws-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "configuration:\n        name: "+createdDeployment.Label)
			assert.Contains(t, out.String(), "ci_cd_enforcement: "+strconv.FormatBool(createdDeployment.APIKeyOnlyDeployments))
			assert.Contains(t, out.String(), "is_high_availability: "+strconv.FormatBool(createdDeployment.IsHighAvailability))
			assert.Contains(t, out.String(), "metadata:\n        deployment_id: "+createdDeployment.ID)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("reads the json file and creates a deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HYBRID"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
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
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "test-ws-id").Return([]astro.Deployment{createdDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "\"configuration\": {\n            \"name\": \""+createdDeployment.Label+"\"")
			assert.Contains(t, out.String(), "\"metadata\": {\n            \"deployment_id\": \""+createdDeployment.ID+"\"")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("reads the json file and creates a hosted standard deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HOSTED_SHARED"
        },
        "worker_queues": [
            {
                "name": "default",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 10,
                "worker_type": "a5"
            },
            {
                "name": "test-queue-1",
                "is_default": false,
                "max_worker_count": 175,
                "min_worker_count": 8,
                "worker_concurrency": 10,
                "worker_type": "a5"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
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
				Type:  "HOSTED_SHARED",
			}
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockOKResponse := &astrocore.GetSharedClusterResponse{
				HTTPResponse: &http.Response{
					StatusCode: 200,
				},
				JSON200: &astrocore.SharedCluster{Id: "test-cluster-id"},
			}
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				AstroMachines: []astro.Machine{
					{
						Type:               "a5",
						ConcurrentTasks:    5,
						ConcurrentTasksMax: 15,
					},
					{
						Type:               "a10",
						ConcurrentTasks:    10,
						ConcurrentTasksMax: 30,
					},
					{
						Type:               "a20",
						ConcurrentTasks:    20,
						ConcurrentTasksMax: 60,
					},
				},
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, mock.Anything).Return(mockOKResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "test-ws-id").Return([]astro.Deployment{createdDeployment}, nil)
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "\"configuration\": {\n            \"name\": \""+createdDeployment.Label+"\"")
			assert.Contains(t, out.String(), "\"metadata\": {\n            \"deployment_id\": \""+createdDeployment.ID+"\"")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if workspace does not exist", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&EmptyListWorkspacesResponseOK, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errNotFound)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if listing workspace fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&EmptyListWorkspacesResponseOK, errTest).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errTest)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if deployment already exists", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return(existingDeployments, nil)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorContains(t, err, "deployment: test-deployment-label already exists: use deployment update --deployment-file deployment.yaml instead")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if creating deployment input fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HYBRID"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1@test.com",
            "email2@test.com"
        ]
    }
}`
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
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.Error(t, err)
			assert.ErrorContains(t, err, "worker queue option is invalid: worker concurrency")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error from the api if create deployment fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HYBRID"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1@test.com",
            "email2@test.com"
        ]
    }
}`
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
			mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "create", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errCreateFailed)
			assert.ErrorContains(t, err, "test error: failed to create deployment with input")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
	})
	t.Run("when action is update", func(t *testing.T) {
		t.Run("reads the yaml file and updates an existing deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    executor: CeleryExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
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
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(updatedDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "configuration:\n        name: "+existingDeployment.Label)
			assert.Contains(t, out.String(), "\n        description: "+updatedDeployment.Description)
			assert.Contains(t, out.String(), "metadata:\n        deployment_id: "+existingDeployment.ID)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("reads the yaml file and updates an existing hosted standard deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    cluster_name: us-east-1
    workspace_name: test-workspace
    deployment_type: HOSTED_SHARED
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 20
      min_worker_count: 5
      worker_concurrency: 10
      worker_type: a10
    - name: test-queue-1
      is_default: false
      max_worker_count: 20
      min_worker_count: 8
      worker_concurrency: 10
      worker_type: a10
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: HEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
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
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				AstroMachines: []astro.Machine{
					{
						Type:               "a5",
						ConcurrentTasks:    5,
						ConcurrentTasksMax: 15,
					},
					{
						Type:               "a10",
						ConcurrentTasks:    10,
						ConcurrentTasksMax: 30,
					},
					{
						Type:               "a20",
						ConcurrentTasks:    20,
						ConcurrentTasksMax: 60,
					},
				},
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(updatedDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "configuration:\n        name: "+existingDeployment.Label)
			assert.Contains(t, out.String(), "\n        description: "+updatedDeployment.Description)
			assert.Contains(t, out.String(), "metadata:\n        deployment_id: "+existingDeployment.ID)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("return an error when enabling dag deploy for ci-cd enforced deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    ci_cd_enforcement: true
    executor: CeleryExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			existingDeployment := astro.Deployment{
				ID:                    "test-deployment-id",
				Label:                 "test-deployment-label",
				Description:           "description",
				APIKeyOnlyDeployments: true,
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
			updatedDeployment := astro.Deployment{
				ID:          "test-deployment-id",
				Label:       "test-deployment-label",
				Description: "description 1",
			}
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			canCiCdDeploy = func(astroAPIToken string) bool {
				return false
			}

			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, out)
			defer testUtil.MockUserInput(t, "n")()
			assert.NoError(t, err)
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("reads the json file and updates an existing deployment", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HYBRID"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "test1@test.com",
            "test2@test.com"
        ]
    }
}`
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
			depIds := []string{createdDeployment.ID}
			deploymentListParams := &astrocore.ListDeploymentsParams{
				DeploymentIds: &depIds,
			}
			mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(updatedDeployment, nil)
			mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockEnvVarResponse, nil)
			mockClient.On("UpdateAlertEmails", mock.Anything).Return(mockAlertEmailResponse, nil)
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), "\"configuration\": {\n            \"name\": \""+existingDeployment.Label+"\"")
			assert.Contains(t, out.String(), "\n            \"description\": \""+updatedDeployment.Description+"\"")
			assert.Contains(t, out.String(), "\"metadata\": {\n            \"deployment_id\": \""+existingDeployment.ID+"\"")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if deployment does not exist", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
    deployment_type: HYBRID
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
			orgID = "test-org-id"
			fileutil.WriteStringToFile(filePath, data)
			defer afero.NewOsFs().Remove(filePath)
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, nil)
			assert.ErrorContains(t, err, "deployment: test-deployment-label does not exist: use deployment create --deployment-file deployment.yaml instead")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error if creating update deployment input fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HYBRID"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1@test.com",
            "email2@test.com"
        ]
    }
}`
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
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, nil)
			assert.Error(t, err)
			assert.ErrorContains(t, err, "worker queue option is invalid: worker concurrency")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
		t.Run("returns an error from the api if update deployment fails", func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			mockClient := new(astro_mocks.Client)
			mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
            "workspace_name": "test-workspace",
			"deployment_type": "HYBRID"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url"
        },
        "alert_emails": [
            "email1@test.com",
            "email2@test.com"
        ]
    }
}`
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
			mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
			mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{existingDeployment}, nil)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errTest)
			err = CreateOrUpdate("deployment.yaml", "update", mockClient, mockCoreClient, nil)
			assert.ErrorIs(t, err, errUpdateFailed)
			assert.ErrorContains(t, err, "test error: failed to update deployment with input")
			mockClient.AssertExpectations(t)
			mockCoreClient.AssertExpectations(t)
		})
	})
}

func TestGetCreateOrUpdateInput(t *testing.T) {
	var (
		expectedDeploymentInput, actualCreateInput       astro.CreateDeploymentInput
		expectedUpdateDeploymentInput, actualUpdateInput astro.UpdateDeploymentInput
		deploymentFromFile                               inspect.FormattedDeployment
		qList                                            []inspect.Workerq
		existingPools                                    []astrocore.NodePool
		expectedQList                                    []astro.WorkerQueue
		clusterID, workspaceID, deploymentID             string
		err                                              error
		mockWorkerQueueDefaultOptions                    astro.WorkerQueueDefaultOptions
	)
	clusterID = "test-cluster-id"
	workspaceID = "test-workspace-id"
	t.Run("common across create and update", func(t *testing.T) {
		t.Run("returns error if worker type does not match existing pools", func(t *testing.T) {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			dagDeploy := true
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
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
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
			assert.ErrorContains(t, err, "worker_type: test-worker-8 does not exist in cluster: test-cluster")
			assert.Equal(t, expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("when executor is Celery", func(t *testing.T) {
			t.Run("returns error if queue options are invalid", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
				dagDeploy := true
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
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "worker queue option is invalid: min worker count")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
			t.Run("returns error if getting worker queue options fails", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
				dagDeploy := true
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
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errTest).Once()
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "failed to get worker queue default options")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
			t.Run("sets default queue options if none were requested", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				dagDeploy := true
				deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
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
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
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
					DagDeployEnabled:      *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.NoError(t, err)
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
		})
		t.Run("when executor is Kubernetes", func(t *testing.T) {
			t.Run("returns an error if more than one worker queue are requested", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				dagDeploy := true
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
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "KubernetesExecutor does not support more than one worker queue. (2) were requested")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
			t.Run("returns an error if Celery queue property min_worker_count is requested", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				dagDeploy := true
				minCount := 10
				qList = []inspect.Workerq{
					{
						Name:           "default",
						WorkerType:     "test-worker-1",
						MinWorkerCount: &minCount,
					},
				}
				deploymentFromFile.Deployment.WorkerQs = qList
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "KubernetesExecutor does not support minimum worker count in the request. It can only be used with CeleryExecutor")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
			t.Run("returns an error if Celery queue property max_worker_count is requested", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				dagDeploy := true
				minCount := -1
				qList = []inspect.Workerq{
					{
						Name:           "default",
						WorkerType:     "test-worker-1",
						MinWorkerCount: &minCount,
						MaxWorkerCount: 10,
					},
				}
				deploymentFromFile.Deployment.WorkerQs = qList
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "KubernetesExecutor does not support maximum worker count in the request. It can only be used with CeleryExecutor")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
			t.Run("returns an error if Celery queue property worker_concurrency is requested", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				dagDeploy := true
				minCount := -1
				qList = []inspect.Workerq{
					{
						Name:              "default",
						WorkerType:        "test-worker-1",
						MinWorkerCount:    &minCount,
						WorkerConcurrency: 10,
					},
				}
				deploymentFromFile.Deployment.WorkerQs = qList
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "KubernetesExecutor does not support worker concurrency in the request. It can only be used with CeleryExecutor")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
			t.Run("returns an error if invalid input is requested", func(t *testing.T) {
				deploymentFromFile = inspect.FormattedDeployment{}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
				deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
				deploymentFromFile.Deployment.Configuration.Description = "test-description"
				deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
				deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
				deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
				deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
				dagDeploy := true
				qList = []inspect.Workerq{
					{
						Name:       "default",
						WorkerType: "test-worker-1",
						PodRAM:     "lots",
					},
				}
				deploymentFromFile.Deployment.WorkerQs = qList
				existingPools = []astrocore.NodePool{
					{
						Id:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-worker-1",
					},
					{
						Id:               "test-pool-id-2",
						IsDefault:        false,
						NodeInstanceType: "test-worker-2",
					},
				}
				expectedDeploymentInput = astro.CreateDeploymentInput{}
				mockClient := new(astro_mocks.Client)
				actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
				assert.ErrorContains(t, err, "KubernetesExecutor does not support pod ram in the request. It will be calculated based on the requested worker type")
				assert.Equal(t, expectedDeploymentInput, actualCreateInput)
				mockClient.AssertExpectations(t)
			})
		})
	})
	t.Run("when action is to create", func(t *testing.T) {
		t.Run("transforms formattedDeployment to CreateDeploymentInput if no queues were requested", func(t *testing.T) {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy

			expectedDeploymentInput = astro.CreateDeploymentInput{
				WorkspaceID:           workspaceID,
				ClusterID:             clusterID,
				Label:                 deploymentFromFile.Deployment.Configuration.Name,
				Description:           deploymentFromFile.Deployment.Configuration.Description,
				RuntimeReleaseVersion: deploymentFromFile.Deployment.Configuration.RunTimeVersion,
				DagDeployEnabled:      *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, nil, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("transforms formattedDeployment to CreateDeploymentInput if Kubernetes executor was requested", func(t *testing.T) {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
			minCount := -1
			qList = []inspect.Workerq{
				{
					Name:           "default",
					WorkerType:     "test-worker-1",
					MinWorkerCount: &minCount,
				},
			}
			deploymentFromFile.Deployment.WorkerQs = qList
			expectedQList = []astro.WorkerQueue{
				{
					Name:           "default",
					IsDefault:      true,
					NodePoolID:     "test-pool-id",
					MinWorkerCount: minCount,
				},
			}
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
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
				DagDeployEnabled:      *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns correct deployment input when multiple queues are requested", func(t *testing.T) {
			deploymentFromFile = inspect.FormattedDeployment{}
			expectedDeploymentInput = astro.CreateDeploymentInput{}
			deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
			deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
			deploymentFromFile.Deployment.Configuration.Description = "test-description"
			deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
			deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
			deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
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
				DagDeployEnabled:      *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			actualCreateInput, _, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "create", &astro.Deployment{}, existingPools, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedDeploymentInput, actualCreateInput)
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("when action is to update", func(t *testing.T) {
		t.Run("transforms formattedDeployment to UpdateDeploymentInput if no queues were requested", func(t *testing.T) {
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
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
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
				DagDeployEnabled: *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, nil, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error if the cluster is being changed", func(t *testing.T) {
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
			dagDeploy := true
			existingDeployment := astro.Deployment{
				ID:    deploymentID,
				Label: "test-deployment",
				Cluster: astro.Cluster{
					ID: "test-cluster-id",
				},
			}

			expectedUpdateDeploymentInput = astro.UpdateDeploymentInput{}
			mockClient := new(astro_mocks.Client)
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, "diff-cluster", workspaceID, "update", &existingDeployment, nil, dagDeploy, mockClient)
			assert.ErrorIs(t, err, errNotPermitted)
			assert.ErrorContains(t, err, "changing an existing deployment's cluster is not permitted")
			assert.Equal(t, expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with no queues", func(t *testing.T) {
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
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy

			existingPools := []astro.NodePool{
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
				DagDeployEnabled: *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, nil, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with a queue", func(t *testing.T) {
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
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
			minCount := -1
			qList = []inspect.Workerq{
				{
					Name:           "default",
					WorkerType:     "test-worker-1",
					MinWorkerCount: &minCount,
				},
			}
			deploymentFromFile.Deployment.WorkerQs = qList
			expectedQList = []astro.WorkerQueue{
				{
					Name:           "default",
					IsDefault:      true,
					NodePoolID:     "test-pool-id",
					MinWorkerCount: minCount,
				},
			}
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
					IsDefault:        false,
					NodeInstanceType: "test-worker-2",
				},
			}
			existingPools1 := []astro.NodePool{
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
					NodePools: existingPools1,
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
				DagDeployEnabled: *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns correct update deployment input when multiple queues are requested", func(t *testing.T) {
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
			dagDeploy := true
			deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        false,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
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
				DagDeployEnabled: *deploymentFromFile.Deployment.Configuration.DagDeployEnabled,
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
			_, actualUpdateInput, err = getCreateOrUpdateInput(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, mockClient)
			assert.NoError(t, err)
			assert.Equal(t, expectedUpdateDeploymentInput, actualUpdateInput)
			mockClient.AssertExpectations(t)
		})
	})
}

func TestCheckRequiredFields(t *testing.T) {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	input.Deployment.Configuration.Description = "test-description"
	t.Run("returns an error if name is missing", func(t *testing.T) {
		err = checkRequiredFields(&input, "")
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.configuration.name")
	})
	t.Run("returns an error if cluster_name is missing", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		err = checkRequiredFields(&input, "")
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.configuration.cluster_name")
	})
	t.Run("returns an error if executor is missing", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		err = checkRequiredFields(&input, "")
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.configuration.executor")
	})
	t.Run("returns an error if executor value is invalid", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = "test-executor"
		err = checkRequiredFields(&input, "")
		assert.ErrorIs(t, err, errInvalidValue)
		assert.ErrorContains(t, err, "is not valid. It can either be CeleryExecutor or KubernetesExecutor")
	})
	t.Run("returns an error if alert email is invalid", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster"
		input.Deployment.Configuration.Executor = deployment.CeleryExecutor
		list := []string{"test@test.com", "testing@testing.com", "not-an-email"}
		input.Deployment.AlertEmails = list
		err = checkRequiredFields(&input, "")
		assert.ErrorIs(t, err, errInvalidEmail)
		assert.ErrorContains(t, err, "invalid email: not-an-email")
	})
	t.Run("returns an error if env var keys are missing on create", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.environment_variables[0].key")
	})
	t.Run("if queues were requested, it returns an error if queue name is missing", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.worker_queues[0].name")
	})
	t.Run("if queues were requested, it returns an error if default queue is missing", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: default queue is missing under deployment.worker_queues")
	})
	t.Run("if queues were requested, it returns an error if worker type is missing", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.worker_queues[0].worker_type")
	})
	t.Run("returns nil if there are no missing fields", func(t *testing.T) {
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
		clusterName, expectedClusterID, actualClusterID string
		actualNodePools                                 []astrocore.NodePool
		err                                             error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedClusterID = "test-cluster-id"
	clusterName = "test-cluster"
	t.Run("returns a cluster id if cluster exists in organization", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgShortName, mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedClusterID, actualClusterID)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns error from api if listing cluster fails", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&astrocore.ListClustersResponse{}, errTest).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgShortName, mockCoreClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, "", actualClusterID)
		assert.Equal(t, []astrocore.NodePool(nil), actualNodePools)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster does not exist in organization", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockListClustersResponse = astrocore.ListClustersResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.ClustersPaginated{
				Clusters: []astrocore.Cluster{},
			},
		}
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgShortName, mockCoreClient)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "cluster_name: test-cluster does not exist in organization: test-org-short-name")
		assert.Equal(t, "", actualClusterID)
		assert.Equal(t, []astrocore.NodePool(nil), actualNodePools)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestGetWorkspaceIDFromName(t *testing.T) {
	var (
		workspaceName, expectedWorkspaceID, actualWorkspaceID, orgID string
		err                                                          error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkspaceID = "test-ws-id"
	workspaceName = "test-workspace"
	orgID = "test-org-id"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	t.Run("returns a workspace id if workspace exists in organization", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedWorkspaceID, actualWorkspaceID)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns error from api if listing workspace fails", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errTest).Once()
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockCoreClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, "", actualWorkspaceID)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if workspace does not exist in organization", func(t *testing.T) {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&EmptyListWorkspacesResponseOK, nil).Once()

		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, orgID, mockCoreClient)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "workspace_name: test-workspace does not exist in organization: test-org-id")
		assert.Equal(t, "", actualWorkspaceID)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestGetNodePoolIDFromName(t *testing.T) {
	var (
		workerType, expectedPoolID, actualPoolID, clusterID string
		existingPools                                       []astrocore.NodePool
		err                                                 error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedPoolID = "test-pool-id"
	workerType = "worker-1"
	clusterID = "test-cluster-id"
	existingPools = []astrocore.NodePool{
		{
			Id:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-1",
		},
		{
			Id:               "test-pool-id",
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
		deploymentFromFile           inspect.FormattedDeployment
		actualWQList, existingWQList []astro.WorkerQueue
		existingPools                []astrocore.NodePool
		err                          error
	)
	t.Run("when the executor is celery", func(t *testing.T) {
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        true,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
					IsDefault:        false,
					NodeInstanceType: "test-worker-2",
				},
			}
			deploymentFromFile = inspect.FormattedDeployment{}
			deploymentFromFile.Deployment.WorkerQs = qList
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, []astro.WorkerQueue(nil))
			assert.NoError(t, err)
			assert.Equal(t, expectedWQList, actualWQList)
		})
		t.Run("returns updated list of existing and queues being added", func(t *testing.T) {
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        true,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
					IsDefault:        false,
					NodeInstanceType: "test-worker-2",
				},
			}
			deploymentFromFile = inspect.FormattedDeployment{}
			deploymentFromFile.Deployment.WorkerQs = qList
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
			assert.NoError(t, err)
			assert.Equal(t, expectedWQList, actualWQList)
		})
		t.Run("returns updated list when multiple queue operations are requested", func(t *testing.T) {
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        true,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
					IsDefault:        false,
					NodeInstanceType: "test-worker-2",
				},
			}
			deploymentFromFile = inspect.FormattedDeployment{}
			deploymentFromFile.Deployment.WorkerQs = qList
			deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
			assert.NoError(t, err)
			assert.Equal(t, expectedWQList, actualWQList)
		})
	})
	t.Run("when the executor is kubernetes", func(t *testing.T) {
		t.Run("returns one default queue regardless of any existing queues", func(t *testing.T) {
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
					Name:           "default",
					IsDefault:      true,
					PodCPU:         "0.1",
					PodRAM:         "0.25Gi",
					MinWorkerCount: -1,
					NodePoolID:     "test-pool-id",
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
			existingPools = []astrocore.NodePool{
				{
					Id:               "test-pool-id",
					IsDefault:        true,
					NodeInstanceType: "test-worker-1",
				},
				{
					Id:               "test-pool-id-2",
					IsDefault:        false,
					NodeInstanceType: "test-worker-2",
				},
			}
			deploymentFromFile = inspect.FormattedDeployment{}
			deploymentFromFile.Deployment.WorkerQs = qList
			deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
			actualWQList, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
			assert.NoError(t, err)
			assert.Equal(t, expectedWQList, actualWQList)
		})
	})
	t.Run("returns an error if unable to determine nodepool id", func(t *testing.T) {
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
		existingPools = []astrocore.NodePool{
			{
				Id:               "test-pool-id",
				IsDefault:        true,
				NodeInstanceType: "test-worker-1",
			},
			{
				Id:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.WorkerQs = qList
		deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
		actualWQList, err = getQueues(&deploymentFromFile, existingPools, []astro.WorkerQueue(nil))
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
		deploymentFromFile     inspect.FormattedDeployment
		expectedInput          astro.UpdateDeploymentAlertsInput
		expected, actual       astro.DeploymentAlerts
		existingEmails, emails []string
		deploymentID           string
		err                    error
	)
	t.Run("updates alert emails for a deployment when no alert emails exist", func(t *testing.T) {
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
	t.Run("updates alert emails for a deployment with new and existing alert emails", func(t *testing.T) {
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

func TestDeploymentFromName(t *testing.T) {
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
	t.Run("returns the deployment id for the matching deployment name", func(t *testing.T) {
		actual = deploymentFromName(existingDeployments, deploymentToCreate)
		assert.Equal(t, expectedeployment, actual)
	})
	t.Run("returns empty string if deployment name does not match", func(t *testing.T) {
		deploymentToCreate = "test-d-2"
		expectedeployment = astro.Deployment{}
		actual = deploymentFromName(existingDeployments, deploymentToCreate)
		assert.Equal(t, expectedeployment, actual)
	})
}

func TestIsValidEmail(t *testing.T) {
	var (
		actual     bool
		emailInput string
	)
	t.Run("returns true if email is valid", func(t *testing.T) {
		emailInput = "test123@superomain.cool.com"
		actual = isValidEmail(emailInput)
		assert.True(t, actual)
	})
	t.Run("returns false if email is invalid", func(t *testing.T) {
		emailInput = "invalid-email.com"
		actual = isValidEmail(emailInput)
		assert.False(t, actual)
	})
}

func TestValidateAlertEmails(t *testing.T) {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	t.Run("returns an error if alert email is invalid", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []string{"test@test.com", "testing@testing.com", "not-an-email"}
		input.Deployment.AlertEmails = list
		err = checkAlertEmails(&input)
		assert.ErrorIs(t, err, errInvalidEmail)
		assert.ErrorContains(t, err, "invalid email: not-an-email")
	})
	t.Run("returns nil if alert email is valid", func(t *testing.T) {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []string{"test@test.com", "testing@testing.com"}
		input.Deployment.AlertEmails = list
		err = checkAlertEmails(&input)
		assert.NoError(t, err)
	})
}

func TestCheckEnvVars(t *testing.T) {
	var (
		err   error
		input inspect.FormattedDeployment
	)
	t.Run("returns an error if env var keys are missing on create", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.environment_variables[0].key")
	})
	t.Run("returns an error if env var values are missing on create", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.environment_variables[1].value")
	})
	t.Run("returns an error if env var keys are missing on update", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errRequiredField)
		assert.ErrorContains(t, err, "missing required field: deployment.environment_variables[1].key")
	})
	t.Run("returns nil if env var values are missing on update", func(t *testing.T) {
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
		assert.NoError(t, err)
	})
}

func TestIsValidExecutor(t *testing.T) {
	t.Run("returns true if executor is Celery", func(t *testing.T) {
		actual := isValidExecutor(deployment.CeleryExecutor)
		assert.True(t, actual)
	})
	t.Run("returns true if executor is Kubernetes", func(t *testing.T) {
		actual := isValidExecutor(deployment.KubeExecutor)
		assert.True(t, actual)
	})
	t.Run("returns false if executor is neither Celery nor Kubernetes", func(t *testing.T) {
		actual := isValidExecutor("test-executor")
		assert.False(t, actual)
	})
}
