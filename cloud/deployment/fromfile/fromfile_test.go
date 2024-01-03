package fromfile

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
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
	executorCelery         = astroplatformcore.DeploymentExecutorCELERY
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	errTest                = errors.New("test error")
	poolID                 = "test-pool-id"
	poolID2                = "test-pool-id-2"
	workloadIdentity       = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	clusterID              = "test-cluster-id"
	clusterName            = "test-cluster"
	highAvailability       = true
	region                 = "test-region"
	cloudProvider          = "test-provider"
	description            = "description 1"
	deploymentResponse     = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                 "test-deployment-id",
			RuntimeVersion:     "4.2.5",
			Namespace:          "test-name",
			WebServerUrl:       "test-url",
			DagDeployEnabled:   false,
			Description:        &description,
			Name:               "test-deployment-label",
			Status:             "HEALTHY",
			Type:               &hybridType,
			ClusterId:          &clusterID,
			Executor:           &executorCelery,
			ClusterName:        &clusterName,
			IsHighAvailability: &highAvailability,
			IsCicdEnforced:     true,
			Region:             &region,
			CloudProvider:      &cloudProvider,
		},
	}
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Status:           "HEALTHY",
			WorkloadIdentity: &workloadIdentity,
			ClusterId:        &clusterID,
			ClusterName:      &clusterName,
		},
	}
	mockCoreDeploymentCreateResponse = []astroplatformcore.Deployment{
		{
			Name:             "test-deployment-label",
			Status:           "HEALTHY",
			WorkloadIdentity: &workloadIdentity,
			ClusterId:        &clusterID,
			ClusterName:      &clusterName,
			Id:               "deployment-id",
		},
	}
	mockListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	mockListDeploymentsCreateResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentCreateResponse,
		},
	}
	cluster = astroplatformcore.Cluster{
		Id:   "test-cluster-id",
		Name: "test-cluster",
		NodePools: &[]astroplatformcore.NodePool{
			{
				Id:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "test-worker-1",
				Name:             "a5",
			},
			{
				Id:               "test-pool-id-2",
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
			{
				Id:               "test-pool-id",
				IsDefault:        false,
				NodeInstanceType: "a5",
			},
		},
	}
	mockGetClusterResponse = astroplatformcore.GetClusterResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &cluster,
	}
	mockListClustersResponse = astroplatformcore.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.ClustersPaginated{
			Clusters: []astroplatformcore.Cluster{
				cluster,
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
		},
	}
	workspaceDescription = "test workspace"
	workspace1           = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &workspaceDescription,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "test-ws-id",
		OrganizationId:               "org-id",
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
	GetDeploymentOptionsResponseOK = astroplatformcore.GetDeploymentOptionsResponse{
		JSON200: &astroplatformcore.DeploymentOptions{
			ResourceQuotas: astroplatformcore.ResourceQuotaOptions{
				ResourceQuota: astroplatformcore.ResourceOption{
					Cpu: astroplatformcore.ResourceRange{
						Ceiling: "2CPU",
						Default: "1CPU",
						Floor:   "0CPU",
					},
					Memory: astroplatformcore.ResourceRange{
						Ceiling: "2GI",
						Default: "1GI",
						Floor:   "0GI",
					},
				},
			},
			WorkerQueues: astroplatformcore.WorkerQueueOptions{
				MaxWorkers: astroplatformcore.Range{
					Ceiling: 200,
					Default: 20,
					Floor:   0,
				},
				MinWorkers: astroplatformcore.Range{
					Ceiling: 20,
					Default: 5,
					Floor:   0,
				},
				WorkerConcurrency: astroplatformcore.Range{
					Ceiling: 200,
					Default: 100,
					Floor:   0,
				},
			},
			WorkerMachines: []astroplatformcore.WorkerMachine{
				{
					Name: "a5",
					Concurrency: astroplatformcore.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
				{
					Name: "a20",
					Concurrency: astroplatformcore.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
			},
			Executors: []string{},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	GetDeploymentOptionsResponseAlphaOK = astrocore.GetDeploymentOptionsResponse{
		JSON200: &astrocore.DeploymentOptions{
			DefaultValues: astrocore.DefaultValueOptions{},
			ResourceQuotas: astrocore.ResourceQuotaOptions{
				ResourceQuota: astrocore.ResourceOption{
					Cpu: astrocore.ResourceRange{
						Ceiling: "2CPU",
						Default: "1CPU",
						Floor:   "0CPU",
					},
					Memory: astrocore.ResourceRange{
						Ceiling: "2GI",
						Default: "1GI",
						Floor:   "0GI",
					},
				},
			},
			Executors: []string{},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	hybridType                   = astroplatformcore.DeploymentTypeHYBRID
	mockCreateDeploymentResponse = astroplatformcore.CreateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Name:          "test-deployment-label",
			Id:            "test-deployment-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
			ClusterId:     &clusterID,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	mockUpdateDeploymentResponse = astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Name:          "test-deployment-label",
			Id:            "test-deployment-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
			ClusterId:     &clusterID,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
)

func TestCreateOrUpdate(t *testing.T) {
	var (
		err            error
		filePath, data string
	)

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
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorContains(t, err, "no context set")
	})
	t.Run("returns an error if cluster does not exist", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errNotFound)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing cluster fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, errTest).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errTest).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("does not update environment variables if input is empty", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.NotNil(t, out)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("does not update alert emails if input is empty", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.NotNil(t, out)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error from the api if creating environment variables fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errTest).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("reads the yaml file and creates a deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "configuration:\n        name: test-deployment-label")
		assert.Contains(t, out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("reads the yaml file and creates a hosted dedicated deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
    deployment_type: DEDICATED
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
		canCiCdDeploy = func(astroAPIToken string) bool {
			return true
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "configuration:\n        name: test-deployment-label")
		assert.Contains(t, out.String(), "metadata:\n        deployment_id: test-deployment-id")
		assert.Contains(t, out.String(), "ci_cd_enforcement: true")
		assert.Contains(t, out.String(), "is_high_availability: true")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("reads the json file and creates a deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		canCiCdDeploy = func(astroAPIToken string) bool {
			return true
		}
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "\"configuration\": {\n            \"name\": \"test-deployment-label\"")
		assert.Contains(t, out.String(), "\"metadata\": {\n            \"deployment_id\": \"test-deployment-id\"")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("reads the json file and creates a hosted standard deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
            "workspace_name": "test-workspace",
			"deployment_type": "STANDARD",
			"region": "test-region",
			"cloud_provider": "test-provider"
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
		mockCoreDeploymentResponse[0].ClusterId = nil
		mockCoreDeploymentCreateResponse[0].ClusterId = nil
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "\"configuration\": {\n            \"name\": \"test-deployment-label\"")
		assert.Contains(t, out.String(), "\"metadata\": {\n            \"deployment_id\": \"test-deployment-id\"")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if workspace does not exist", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&EmptyListWorkspacesResponseOK, nil).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errNotFound)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&EmptyListWorkspacesResponseOK, errTest).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errTest)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if deployment already exists", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorContains(t, err, "deployment: test-deployment-label already exists: use deployment update --deployment-file deployment.yaml instead")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if creating deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
                "max_worker_count": 200,
                "min_worker_count": 12,
                "worker_concurrency": 500,
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "worker queue option is invalid: worker concurrency")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error from the api if get deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		deploymentResponse.JSON200.ClusterId = &clusterID
		mockListDeploymentsCreateResponse.JSON200.Deployments[0].ClusterId = &clusterID
		mockListDeploymentsResponse.JSON200.Deployments[0].ClusterId = &clusterID
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, errCreateFailed).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errCreateFailed)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("reads the yaml file and updates an existing deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "configuration:\n        name: test-deployment-label")
		assert.Contains(t, out.String(), "\n        description: description 1")
		assert.Contains(t, out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("reads the yaml file and updates an existing hosted standard deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
    cluster_name: clusterName
    workspace_name: test-workspace
    deployment_type: STANDARD
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 20
      min_worker_count: 5
      worker_concurrency: 10
      worker_type: a5
    - name: test-queue-1
      is_default: false
      max_worker_count: 20
      min_worker_count: 8
      worker_concurrency: 10
      worker_type: a5
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreDeploymentResponse[0].ClusterId = nil
		mockCoreDeploymentCreateResponse[0].ClusterId = nil
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "configuration:\n        name: test-deployment-label")
		assert.Contains(t, out.String(), "\n        description: description 1")
		assert.Contains(t, out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("return an error when enabling dag deploy for ci-cd enforced deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreDeploymentResponse[0].ClusterId = &clusterID
		mockCoreDeploymentCreateResponse[0].ClusterId = &clusterID
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		canCiCdDeploy = func(astroAPIToken string) bool {
			return false
		}

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out)
		defer testUtil.MockUserInput(t, "n")()
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("reads the json file and updates an existing deployment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "test-deployment-label")
		assert.Contains(t, out.String(), "description 1")
		assert.Contains(t, out.String(), "test-deployment-id")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if deployment does not exist", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorContains(t, err, "deployment: test-deployment-label does not exist: use deployment create --deployment-file deployment.yaml instead")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if updating deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
                "worker_concurrency": 500,
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, nil)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "worker queue option is invalid: worker concurrency")
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error from the api if update deployment fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateFailed).Times(1)
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, nil)
		assert.ErrorIs(t, err, errUpdateFailed)
		assert.ErrorContains(t, err, "failed to update deployment with input")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestGetCreateOrUpdateInput(t *testing.T) {
	var (
		deploymentFromFile                   inspect.FormattedDeployment
		qList                                []inspect.Workerq
		existingPools                        []astroplatformcore.NodePool
		expectedQList                        []astroplatformcore.WorkerQueue
		clusterID, workspaceID, deploymentID string
		err                                  error
	)
	clusterID = "test-cluster-id"
	workspaceID = "test-workspace-id"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("returns error if worker type does not match existing pools", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
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
				MinWorkerCount:    minCount,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-8",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astroplatformcore.NodePool{
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

		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "worker_type: test-worker-8 does not exist in cluster: test-cluster")
	})
	t.Run("returns error if queue options are invalid", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
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
				MinWorkerCount:    minCountThirty,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCountThree,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astroplatformcore.NodePool{
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
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "worker queue option is invalid: min worker count")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns error if getting worker queue options fails", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
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
				MinWorkerCount:    minCountThirty,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCountThree,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astroplatformcore.NodePool{
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
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errTest).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errTest)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("sets default queue options if none were requested", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
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
				MinWorkerCount: minCount,
			},
			{
				Name:           "test-q-2",
				WorkerType:     "test-worker-2",
				MinWorkerCount: minCount,
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astroplatformcore.NodePool{
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
		expectedQList = []astroplatformcore.WorkerQueue{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    125,
				MinWorkerCount:    5,
				WorkerConcurrency: 180,
				NodePoolId:        &poolID,
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    125,
				MinWorkerCount:    5,
				WorkerConcurrency: 180,
				NodePoolId:        &poolID2,
			},
		}
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if more than one worker queue are requested", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
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
		existingPools = []astroplatformcore.NodePool{
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
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "don't use 'worker_queues' to update default queue with KubernetesExecutor")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("transforms formattedDeployment to CreateDeploymentInput if no queues were requested", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
		dagDeploy := true
		deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy

		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, nil, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("transforms formattedDeployment to CreateDeploymentInput if Kubernetes executor was requested", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
		dagDeploy := true
		deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy

		existingPools = []astroplatformcore.NodePool{
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

		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns correct deployment input when multiple queues are requested", func(t *testing.T) {
		deploymentFromFile = inspect.FormattedDeployment{}
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
				MinWorkerCount:    minCount,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astroplatformcore.NodePool{
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
		expectedQList = []astroplatformcore.WorkerQueue{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				NodePoolId:        &poolID,
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				NodePoolId:        &poolID2,
			},
		}

		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("transforms formattedDeployment to UpdateDeploymentInput if no queues were requested", func(t *testing.T) {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
		dagDeploy := true
		deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
		clusterID := "test-cluster-id"
		existingDeployment := astroplatformcore.Deployment{
			Id:        deploymentID,
			Name:      "test-deployment",
			ClusterId: &clusterID,
		}
		existingPools = []astroplatformcore.NodePool{
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

		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if the cluster is being changed", func(t *testing.T) {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster-1"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
		dagDeploy := true
		clusterID := "test-cluster-id"
		existingDeployment := astroplatformcore.Deployment{
			Id:        deploymentID,
			Name:      "test-deployment",
			ClusterId: &clusterID,
		}
		err = createOrUpdateDeployment(&deploymentFromFile, "diff-cluster", workspaceID, "update", &existingDeployment, nil, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errNotPermitted)
		assert.ErrorContains(t, err, "changing an existing deployment's cluster is not permitted")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with no queues", func(t *testing.T) {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
		dagDeploy := true
		deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy

		existingPools = []astroplatformcore.NodePool{
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
		clusterID := "test-cluster-id"
		existingDeployment := astroplatformcore.Deployment{
			Id:           deploymentID,
			Name:         "test-deployment",
			ClusterId:    &clusterID,
			Executor:     &executorCelery,
			WorkerQueues: &expectedQList,
		}

		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with a queue", func(t *testing.T) {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
		dagDeploy := true
		deploymentFromFile.Deployment.Configuration.DagDeployEnabled = &dagDeploy
		deploymentFromFile.Deployment.Configuration.DefaultWorkerType = "test-worker-1"
		existingPools = []astroplatformcore.NodePool{
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
		clusterID := "test-cluster-id"
		clusterName := "test-cluster"
		existingDeployment := astroplatformcore.Deployment{
			Id:           deploymentID,
			Name:         "test-deployment",
			ClusterId:    &clusterID,
			ClusterName:  &clusterName,
			Executor:     &executorCelery,
			WorkerQueues: &expectedQList,
		}

		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns correct update deployment input when multiple queues are requested", func(t *testing.T) {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
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
				MinWorkerCount:    minCount,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
				WorkerConcurrency: 200,
				WorkerType:        "test-worker-2",
			},
		}
		deploymentFromFile.Deployment.WorkerQs = qList
		existingPools = []astroplatformcore.NodePool{
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
		expectedQList = []astroplatformcore.WorkerQueue{
			{
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				NodePoolId:        &poolID,
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 200,
				NodePoolId:        &poolID2,
			},
		}
		clusterID := "test-cluster-id"
		existingDeployment := astroplatformcore.Deployment{
			Id:           deploymentID,
			Name:         "test-deployment",
			ClusterId:    &clusterID,
			WorkerQueues: &expectedQList,
		}

		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
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
		existingDeployments []astroplatformcore.Deployment
		deploymentToCreate  string
		actual              bool
	)
	description = "deployment 1"
	description2 := "deployment 2"
	existingDeployments = []astroplatformcore.Deployment{
		{
			Id:          "test-d-1",
			Name:        "test-deployment-1",
			Description: &description,
		},
		{
			Id:          "test-d-2",
			Name:        "test-deployment-2",
			Description: &description2,
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
		actualNodePools                                 []astroplatformcore.NodePool
		err                                             error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedClusterID = "test-cluster-id"
	clusterName = "test-cluster"
	t.Run("returns a cluster id if cluster exists in organization", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgShortName, mockPlatformCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedClusterID, actualClusterID)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns error from api if listing cluster fails", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListClustersResponse{}, errTest).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgShortName, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, "", actualClusterID)
		assert.Equal(t, []astroplatformcore.NodePool(nil), actualNodePools)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster does not exist in organization", func(t *testing.T) {
		mockListClustersResponse = astroplatformcore.ListClustersResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.ClustersPaginated{
				Clusters: []astroplatformcore.Cluster{},
			},
		}
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgShortName, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errNotFound)
		assert.ErrorContains(t, err, "cluster_name: test-cluster does not exist in organization: test-org-short-name")
		assert.Equal(t, "", actualClusterID)
		assert.Equal(t, []astroplatformcore.NodePool(nil), actualNodePools)
		mockPlatformCoreClient.AssertExpectations(t)
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
		existingPools                                       []astroplatformcore.NodePool
		err                                                 error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedPoolID = "test-pool-id"
	workerType = "worker-1"
	clusterID = "test-cluster-id"
	existingPools = []astroplatformcore.NodePool{
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
		actualEnvVars      []astroplatformcore.DeploymentEnvironmentVariableRequest
		deploymentFromFile inspect.FormattedDeployment
		err                error
	)
	t.Run("creates env vars if they were requested in a formatted deployment", func(t *testing.T) {
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
		val1 := "val-1"
		val2 := "val-2"
		expectedList := []astroplatformcore.DeploymentEnvironmentVariableRequest{
			{
				IsSecret: false,
				Key:      "key-1",
				Value:    &val1,
			},
			{
				IsSecret: true,
				Key:      "key-2",
				Value:    &val2,
			},
		}
		deploymentFromFile.Deployment.EnvVars = list
		actualEnvVars = createEnvVarsRequest(&deploymentFromFile)
		assert.NoError(t, err)
		assert.Equal(t, expectedList, actualEnvVars)
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
				MinWorkerCount:    minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
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
		existingWQList     []astroplatformcore.WorkerQueue
		existingPools      []astroplatformcore.NodePool
		err                error
	)
	t.Run("returns list of queues for the requested deployment", func(t *testing.T) {
		minCount := 3
		qList := []inspect.Workerq{
			{
				Name:              "default",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-2",
			},
		}
		existingPools = []astroplatformcore.NodePool{
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
		_, err = getQueues(&deploymentFromFile, existingPools, []astroplatformcore.WorkerQueue(nil))
		assert.NoError(t, err)
	})
	t.Run("returns updated list of existing and queues being added", func(t *testing.T) {
		existingWQList = []astroplatformcore.WorkerQueue{
			{
				Id:                "q-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				NodePoolId:        &poolID,
			},
		}
		minCountThree := 3
		minCountFour := 4
		qList := []inspect.Workerq{
			{
				Name:              "default",
				MaxWorkerCount:    18,
				MinWorkerCount:    minCountFour,
				WorkerConcurrency: 25,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCountThree,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-2",
			},
		}
		existingPools = []astroplatformcore.NodePool{
			{
				Id:               poolID,
				IsDefault:        true,
				NodeInstanceType: "test-worker-1",
			},
			{
				Id:               poolID2,
				IsDefault:        false,
				NodeInstanceType: "test-worker-2",
			},
		}
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.WorkerQs = qList
		deploymentFromFile.Deployment.Configuration.Executor = deployment.CeleryExecutor
		_, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
		assert.NoError(t, err)
	})
	t.Run("returns updated list when multiple queue operations are requested", func(t *testing.T) {
		existingWQList = []astroplatformcore.WorkerQueue{
			{
				Id:                "q-id",
				Name:              "default", // this queue is getting updated
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				NodePoolId:        &poolID,
			},
			{
				Id:                "q-id-1",
				Name:              "q-1", // this queue is getting deleted
				IsDefault:         false,
				MaxWorkerCount:    12,
				MinWorkerCount:    4,
				WorkerConcurrency: 22,
				NodePoolId:        &poolID2,
			},
		}
		minCountThree := 3
		minCountFour := 4
		qList := []inspect.Workerq{
			{
				Name:              "default",
				MaxWorkerCount:    18,
				MinWorkerCount:    minCountFour,
				WorkerConcurrency: 25,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2", // this queue is being added
				MaxWorkerCount:    16,
				MinWorkerCount:    minCountThree,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-2",
			},
		}
		existingPools = []astroplatformcore.NodePool{
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
		_, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
		assert.NoError(t, err)
	})
	t.Run("returns one default queue regardless of any existing queues", func(t *testing.T) {
		existingWQList = []astroplatformcore.WorkerQueue{
			{
				Id:                "q-id",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				NodePoolId:        &poolID,
			},
			{
				Name:              "test-q-2",
				IsDefault:         false,
				MaxWorkerCount:    16,
				MinWorkerCount:    3,
				WorkerConcurrency: 20,
				NodePoolId:        &poolID,
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
		existingPools = []astroplatformcore.NodePool{
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
		_, err = getQueues(&deploymentFromFile, existingPools, existingWQList)
		assert.NoError(t, err)
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
				MinWorkerCount:    minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-1",
			},
			{
				Name:              "test-q-2",
				MaxWorkerCount:    16,
				MinWorkerCount:    minCount,
				WorkerConcurrency: 20,
				WorkerType:        "test-worker-4",
			},
		}
		existingPools = []astroplatformcore.NodePool{
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
		_, err = getQueues(&deploymentFromFile, existingPools, []astroplatformcore.WorkerQueue(nil))
		assert.ErrorContains(t, err, "worker_type: test-worker-4 does not exist in cluster: test-cluster")
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
		existingDeployments       []astroplatformcore.Deployment
		deploymentToCreate        string
		actual, expectedeployment astroplatformcore.Deployment
	)
	description = "deployment 1"
	description2 := "deployment 2"
	existingDeployments = []astroplatformcore.Deployment{
		{
			Id:          "test-d-1",
			Name:        "test-deployment-1",
			Description: &description,
		},
		{
			Id:          "test-d-2",
			Name:        "test-deployment-2",
			Description: &description2,
		},
	}
	description = "deployment 2"
	expectedeployment = astroplatformcore.Deployment{
		Id:          "test-d-2",
		Name:        "test-deployment-2",
		Description: &description,
	}
	deploymentToCreate = "test-deployment-2"
	t.Run("returns the deployment id for the matching deployment name", func(t *testing.T) {
		actual = deploymentFromName(existingDeployments, deploymentToCreate)
		assert.Equal(t, expectedeployment, actual)
	})
	t.Run("returns empty string if deployment name does not match", func(t *testing.T) {
		deploymentToCreate = "test-d-2"
		expectedeployment = astroplatformcore.Deployment{}
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
