package fromfile

import (
	"bytes"
	"errors"
	"net/http"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/ghodss/yaml"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	mockOrgID = "test-org-id"
)

var (
	executorCelery                      astroplatformcore.DeploymentExecutor
	mockPlatformCoreClient              *astroplatformcore_mocks.ClientWithResponsesInterface
	errTest                             error
	poolID                              string
	poolID2                             string
	val1                                string
	val2                                string
	workloadIdentity                    string
	clusterID                           string
	clusterName                         string
	highAvailability                    bool
	isDevelopmentMode                   bool
	region                              string
	cloudProvider                       astroplatformcore.DeploymentCloudProvider
	description                         string
	schedulerAU                         int
	schedulerTestSize                   astroplatformcore.DeploymentSchedulerSize
	defaultTaskPodCPU                   string
	defaultTaskPodMemory                string
	resourceQuotaCPU                    string
	resourceQuotaMemory                 string
	hibernationDescription              string
	hibernationSchedules                []astroplatformcore.DeploymentHibernationSchedule
	deploymentResponse                  astroplatformcore.GetDeploymentResponse
	taskLogBucket                       string
	taskLogURLPattern                   string
	executorAstro                       astroplatformcore.DeploymentExecutor
	dedicatedType                       astroplatformcore.DeploymentType
	deploymentResponseRemoteExecution   astroplatformcore.GetDeploymentResponse
	mockCoreDeploymentResponse          []astroplatformcore.Deployment
	mockCoreDeploymentCreateResponse    []astroplatformcore.Deployment
	mockListDeploymentsResponse         astroplatformcore.ListDeploymentsResponse
	mockListDeploymentsCreateResponse   astroplatformcore.ListDeploymentsResponse
	cluster                             astroplatformcore.Cluster
	mockGetClusterResponse              astroplatformcore.GetClusterResponse
	mockListClustersResponse            astroplatformcore.ListClustersResponse
	workspaceDescription                string
	workspace1                          astrocore.Workspace
	workspaces                          []astrocore.Workspace
	ListWorkspacesResponseOK            astrocore.ListWorkspacesResponse
	EmptyListWorkspacesResponseOK       astrocore.ListWorkspacesResponse
	GetDeploymentOptionsResponseOK      astroplatformcore.GetDeploymentOptionsResponse
	GetDeploymentOptionsResponseAlphaOK astrocore.GetDeploymentOptionsResponse
	hybridType                          astroplatformcore.DeploymentType
	mockCreateDeploymentResponse        astroplatformcore.CreateDeploymentResponse
	mockUpdateDeploymentResponse        astroplatformcore.UpdateDeploymentResponse
)

func MockResponseInit() {
	executorCelery = astroplatformcore.DeploymentExecutorCELERY
	errTest = errors.New("test error")
	poolID = "test-pool-id"
	poolID2 = "test-pool-id-2"
	val1 = "val-1"
	val2 = "val-2"
	workloadIdentity = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	clusterID = "test-cluster-id"
	clusterName = "test-cluster"
	highAvailability = true
	isDevelopmentMode = true
	region = "test-region"
	cloudProvider = astroplatformcore.DeploymentCloudProviderAWS
	description = "description 1"
	schedulerAU = 5
	schedulerTestSize = astroplatformcore.DeploymentSchedulerSizeSMALL
	defaultTaskPodCPU = "defaultTaskPodCPU"
	defaultTaskPodMemory = "defaultTaskPodMemory"
	resourceQuotaCPU = "resourceQuotaCPU"
	resourceQuotaMemory = "ResourceQuotaMemory"
	hibernationDescription = "hibernation schedule 1"
	hibernationSchedules = []astroplatformcore.DeploymentHibernationSchedule{
		{
			HibernateAtCron: "1 * * * *",
			WakeAtCron:      "2 * * * *",
			Description:     &hibernationDescription,
			IsEnabled:       true,
		},
	}
	deploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                   "test-deployment-id",
			RuntimeVersion:       "4.2.5",
			Namespace:            "test-name",
			WebServerUrl:         "test-url",
			IsDagDeployEnabled:   false,
			Description:          &description,
			Name:                 "test-deployment-label",
			Status:               "HEALTHY",
			Type:                 &hybridType,
			ClusterId:            &clusterID,
			Executor:             &executorCelery,
			ClusterName:          &clusterName,
			IsHighAvailability:   &highAvailability,
			IsDevelopmentMode:    &isDevelopmentMode,
			SchedulerAu:          &schedulerAU,
			DefaultTaskPodCpu:    &defaultTaskPodCPU,
			DefaultTaskPodMemory: &defaultTaskPodMemory,
			ResourceQuotaCpu:     &resourceQuotaCPU,
			ResourceQuotaMemory:  &resourceQuotaMemory,
			SchedulerSize:        &schedulerTestSize,
			WorkspaceName:        &workspace1.Name,
			IsCicdEnforced:       true,
			Region:               &region,
			CloudProvider:        &cloudProvider,
			ScalingSpec: &astroplatformcore.DeploymentScalingSpec{
				HibernationSpec: &astroplatformcore.DeploymentHibernationSpec{
					Schedules: &hibernationSchedules,
				},
			},
		},
	}
	taskLogBucket = "task-log-bucket"
	taskLogURLPattern = "task-log-url-pattern"
	executorAstro = astroplatformcore.DeploymentExecutorASTRO
	dedicatedType = astroplatformcore.DeploymentTypeDEDICATED
	deploymentResponseRemoteExecution = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                 "test-deployment-id",
			RuntimeVersion:     "3.0-1",
			Namespace:          "test-name",
			WebServerUrl:       "test-url",
			IsDagDeployEnabled: false,
			Description:        &description,
			Name:               "test-deployment-label",
			Status:             "HEALTHY",
			Type:               &dedicatedType,
			ClusterId:          &clusterID,
			Executor:           &executorAstro,
			ClusterName:        &clusterName,
			IsHighAvailability: &highAvailability,
			SchedulerAu:        &schedulerAU,
			SchedulerSize:      &schedulerTestSize,
			WorkspaceName:      &workspace1.Name,
			RemoteExecution: &astroplatformcore.DeploymentRemoteExecution{
				Enabled:                true,
				AllowedIpAddressRanges: []string{"0.0.0.0/0"},
				TaskLogBucket:          &taskLogBucket,
				TaskLogUrlPattern:      &taskLogURLPattern,
			},
		},
	}
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Status:           "HEALTHY",
			Id:               "test-deployment-id",
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
			Id:               "test-deployment-id",
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
	workspace1 = astrocore.Workspace{
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
	hybridType = astroplatformcore.DeploymentTypeHYBRID
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
}

type Suite struct {
	suite.Suite
}

func TestFromFile(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) SetupTest() {
	// init mocks
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)

	// init responses object
	MockResponseInit()
}

func (s *Suite) TearDownSubTest() {
	// assert expectations
	mockPlatformCoreClient.AssertExpectations(s.T())

	// reset mocks
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)

	// reset responses object
	MockResponseInit()
}

var _ suite.SetupTestSuite = (*Suite)(nil)

func (s *Suite) TestCreateOrUpdate() {
	var (
		err            error
		filePath, data string
	)

	s.Run("returns an error if file does not exist", func() {
		err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil, false, 0*time.Second)
		s.ErrorContains(err, "open deployment.yaml: no such file or directory")
	})
	s.Run("returns an error if file exists but user provides incorrect path", func() {
		filePath = "./2/deployment.yaml"
		data = "test"
		err = fileutil.WriteStringToFile(filePath, data)
		s.NoError(err)
		defer afero.NewOsFs().RemoveAll("./2")
		err = CreateOrUpdate("1/deployment.yaml", "create", nil, nil, nil, false, 0*time.Second)
		s.ErrorContains(err, "open 1/deployment.yaml: no such file or directory")
	})
	s.Run("returns an error if file is empty", func() {
		filePath = "./deployment.yaml"
		data = ""
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil, false, 0*time.Second)
		s.ErrorIs(err, errEmptyFile)
		s.ErrorContains(err, "deployment.yaml has no content")
	})
	s.Run("returns an error if unmarshalling fails", func() {
		filePath = "./deployment.yaml"
		data = "test"
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil, false, 0*time.Second)
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = CreateOrUpdate("deployment.yaml", "create", nil, nil, nil, false, 0*time.Second)
		s.ErrorContains(err, "missing required field: deployment.configuration.name")
	})
	s.Run("returns an error if getting context fails", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`

		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorContains(err, "no context set")
	})
	s.Run("returns an error if cluster does not exist", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errNotFound)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if listing cluster fails", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, errTest).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errTest)
	})
	s.Run("returns an error if listing deployment fails", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errTest).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errTest)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("does not update environment variables if input is empty", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.NotNil(out)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("does not update alert emails if input is empty", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.NotNil(out)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error from the api if creating environment variables fails", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errTest)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and creates a deployment", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and creates a deployment with kube executor", func() {
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
    executor: KubernetesExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and creates a hosted dedicated deployment", func() {
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
    scheduler_size: small
    cluster_name: test-cluster
    workspace_name: test-workspace
    cloud_provider: gcp
    scheduler_size: small
    deployment_type: DEDICATED
    cloud_provider: gcp
    is_high_availability: true
    is_development_mode: true
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
  hibernation_schedules:
    - hibernate_at: 1 * * * *
      wake_at: 2 * * * *
      description: hibernation schedule 1
      enabled: true
`
		canCiCdDeploy = func(astroAPIToken string) bool {
			return true
		}
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.CreateDeploymentRequest) bool {
				request, _ := input.AsCreateDedicatedDeploymentRequest()
				schedules := *request.ScalingSpec.HibernationSpec.Schedules
				schedule := schedules[0]
				return request.Name == "test-deployment-label" && request.IsCicdEnforced && request.IsHighAvailability && *request.IsDevelopmentMode && schedule.IsEnabled && *schedule.Description == "hibernation schedule 1" && schedule.HibernateAtCron == "1 * * * *" && schedule.WakeAtCron == "2 * * * *"
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				request, _ := input.AsUpdateDedicatedDeploymentRequest()
				schedules := *request.ScalingSpec.HibernationSpec.Schedules
				schedule := schedules[0]
				return request.Name == "test-deployment-label" && request.IsCicdEnforced && request.IsHighAvailability && schedule.IsEnabled && *schedule.Description == "hibernation schedule 1" && schedule.HibernateAtCron == "1 * * * *" && schedule.WakeAtCron == "2 * * * *"
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		s.Contains(out.String(), "ci_cd_enforcement: true")
		s.Contains(out.String(), "is_high_availability: true")
		s.Contains(out.String(), "is_development_mode: true")
		s.Contains(out.String(), "hibernation_schedules:\n        - hibernate_at: 1 * * * *\n          wake_at: 2 * * * *\n          description: hibernation schedule 1\n          enabled: true\n\n")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and creates a hosted dedicated deployment with remote execution config", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		filePath = "./deployment.yaml"
		data = `
deployment:
  configuration:
    name: test-deployment-label
    description: description
    runtime_version: 3.0-1
    dag_deploy_enabled: false
    executor: AstroExecutor
    scheduler_au: 5
    scheduler_count: 3
    scheduler_size: small
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: DEDICATED
    is_high_availability: true
    remote_execution:
      enabled: true
      allowed_ip_address_ranges:
        - 0.0.0.0/0
      task_log_bucket: task-log-bucket
      task_log_url_pattern: task-log-url-pattern
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
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
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.CreateDeploymentRequest) bool {
				request, _ := input.AsCreateDedicatedDeploymentRequest()
				return request.Name == "test-deployment-label" && request.RemoteExecution != nil
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				request, _ := input.AsUpdateDedicatedDeploymentRequest()
				return request.Name == "test-deployment-label" && request.RemoteExecution != nil
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "test-deployment-id").Return(&deploymentResponseRemoteExecution, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		s.Contains(out.String(), "remote_execution:\n            enabled: true\n            allowed_ip_address_ranges:\n                - 0.0.0.0/0\n            task_log_bucket: task-log-bucket\n            task_log_url_pattern: task-log-url-pattern\n")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the json file and creates a deployment", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "\"configuration\": {\n            \"name\": \"test-deployment-label\"")
		s.Contains(out.String(), "\"metadata\": {\n            \"deployment_id\": \"test-deployment-id\"")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the json file and creates a hosted standard deployment", func() {
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
			"scheduler_size": "large",
            "workspace_name": "test-workspace",
			"deployment_type": "STANDARD",
			"region": "test-region",
			"cloud_provider": "aws",
			"is_development_mode": true,
			"workload_identity": "test-workload-identity"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		deploymentResponse.JSON200.ClusterId = nil
		standardType := astroplatformcore.DeploymentTypeSTANDARD
		deploymentResponse.JSON200.Type = &standardType
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.CreateDeploymentRequest) bool {
				request, err := input.AsCreateStandardDeploymentRequest()
				s.NoError(err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == "test-workload-identity" &&
					request.Type == astroplatformcore.CreateStandardDeploymentRequestTypeSTANDARD
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "\"configuration\": {\n            \"name\": \"test-deployment-label\"")
		s.Contains(out.String(), "\"metadata\": {\n            \"deployment_id\": \"test-deployment-id\"")
		s.Contains(out.String(), "\"is_development_mode\": true")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the json file and creates a hosted standard deployment with astro executor", func() {
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
            "runtime_version": "3.0-1",
            "dag_deploy_enabled": true,
            "executor": "AstroExecutor",
            "scheduler_au": 5,
            "scheduler_count": 3,
			"scheduler_size": "large",
            "workspace_name": "test-workspace",
			"deployment_type": "STANDARD",
			"region": "test-region",
			"cloud_provider": "aws",
			"is_development_mode": true,
			"workload_identity": "test-workload-identity"
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		deploymentResponse.JSON200.ClusterId = nil
		standardType := astroplatformcore.DeploymentTypeSTANDARD
		deploymentResponse.JSON200.Type = &standardType
		deploymentResponse.JSON200.Executor = &executorAstro
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.CreateDeploymentRequest) bool {
				request, err := input.AsCreateStandardDeploymentRequest()
				s.NoError(err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == "test-workload-identity" &&
					request.Type == astroplatformcore.CreateStandardDeploymentRequestTypeSTANDARD && request.Executor == astroplatformcore.CreateStandardDeploymentRequestExecutorASTRO
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "\"configuration\": {\n            \"name\": \"test-deployment-label\"")
		s.Contains(out.String(), "\"metadata\": {\n            \"deployment_id\": \"test-deployment-id\"")
		s.Contains(out.String(), "\"is_development_mode\": true")
		s.Contains(out.String(), "\"executor\": \"ASTRO\"")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if listing workspace fails", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&EmptyListWorkspacesResponseOK, errTest).Times(1)
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errTest)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if deployment already exists", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
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
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorContains(err, "deployment: test-deployment-label already exists: use deployment update --deployment-file deployment.yaml instead")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if creating deployment fails", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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

		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.Error(err)
		s.ErrorContains(err, "worker queue option is invalid: worker concurrency")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error from the api if get deployment fails", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, errCreateFailed).Once()
		err = CreateOrUpdate("deployment.yaml", "create", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errCreateFailed)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and updates an existing deployment", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "\n        description: description 1")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and updates an existing kube deployment", func() {
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
    executor: KubernetesExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "\n        description: description 1")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and updates an existing hosted standard deployment", func() {
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
    cloud_provider: azure
    scheduler_size: medium
    workspace_name: test-workspace
    deployment_type: STANDARD
    workload_identity: test-workload-identity
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
  hibernation_schedules:
    - hibernate_at: 1 * * * *
      wake_at: 2 * * * *
      description: hibernation schedule 1
      enabled: true
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreDeploymentResponse[0].ClusterId = nil
		mockCoreDeploymentCreateResponse[0].ClusterId = nil
		deploymentResponse.JSON200.ClusterId = nil
		standardType := astroplatformcore.DeploymentTypeSTANDARD
		deploymentResponse.JSON200.Type = &standardType
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				request, err := input.AsUpdateStandardDeploymentRequest()
				s.NoError(err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == "test-workload-identity" &&
					request.Type == astroplatformcore.UpdateStandardDeploymentRequestTypeSTANDARD
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "\n        description: description 1")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		s.Contains(out.String(), "hibernation_schedules:\n        - hibernate_at: 1 * * * *\n          wake_at: 2 * * * *\n          description: hibernation schedule 1\n          enabled: true\n\n")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and updates an existing hosted dedicated deployment", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockCoreDeploymentResponse[0].ClusterId = &clusterID
		mockCoreDeploymentCreateResponse[0].ClusterId = &clusterID
		deploymentResponse.JSON200.ClusterId = &clusterID
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
    cluster_name: test-cluster
    cloud_provider: azure
    scheduler_size: medium
    workspace_name: test-workspace
    deployment_type: DEDICATED
    workload_identity: test-workload-identity
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				request, err := input.AsUpdateDedicatedDeploymentRequest()
				s.NoError(err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == "test-workload-identity" &&
					request.Type == astroplatformcore.UpdateDedicatedDeploymentRequestTypeDEDICATED
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "\n        description: description 1")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("return an error when enabling dag deploy for ci-cd enforced deployment", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreDeploymentResponse[0].ClusterId = &clusterID
		mockCoreDeploymentCreateResponse[0].ClusterId = &clusterID
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		canCiCdDeploy = func(astroAPIToken string) bool {
			return false
		}

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		defer testUtil.MockUserInput(s.T(), "n")()
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the json file and updates an existing deployment", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "test-deployment-label")
		s.Contains(out.String(), "description 1")
		s.Contains(out.String(), "test-deployment-id")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("reads the yaml file and updates an existing hosted standard deployment with two different queue types", func() {
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
    cloud_provider: azure
    scheduler_size: medium
    workspace_name: test-workspace
    deployment_type: STANDARD
    workload_identity: test-workload-identity
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
      worker_type: a10
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: HEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
  hibernation_schedules:
    - hibernate_at: 1 * * * *
      wake_at: 2 * * * *
      description: hibernation schedule 1
      enabled: true
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreDeploymentResponse[0].ClusterId = nil
		mockCoreDeploymentCreateResponse[0].ClusterId = nil
		deploymentResponse.JSON200.ClusterId = nil
		standardType := astroplatformcore.DeploymentTypeSTANDARD
		deploymentResponse.JSON200.Type = &standardType
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				request, err := input.AsUpdateStandardDeploymentRequest()
				s.NoError(err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == "test-workload-identity" &&
					request.Type == astroplatformcore.UpdateStandardDeploymentRequestTypeSTANDARD &&
					len(*request.WorkerQueues) == 2 &&
					(*request.WorkerQueues)[0].AstroMachine == "A5" &&
					(*request.WorkerQueues)[1].AstroMachine == "A10"
			},
		)).Return(&mockUpdateDeploymentResponse, nil)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, out, false, 0*time.Second)
		s.NoError(err)
		s.Contains(out.String(), "configuration:\n        name: test-deployment-label")
		s.Contains(out.String(), "\n        description: description 1")
		s.Contains(out.String(), "metadata:\n        deployment_id: test-deployment-id")
		s.Contains(out.String(), "hibernation_schedules:\n        - hibernate_at: 1 * * * *\n          wake_at: 2 * * * *\n          description: hibernation schedule 1\n          enabled: true\n\n")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if deployment does not exist", func() {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorContains(err, "deployment: test-deployment-label does not exist: use deployment create --deployment-file deployment.yaml instead")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if updating deployment fails", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.Error(err)
		s.ErrorContains(err, "worker queue option is invalid: worker concurrency")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error from the api if update deployment fails", func() {
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
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id",
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
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateFailed).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err = CreateOrUpdate("deployment.yaml", "update", mockPlatformCoreClient, mockCoreClient, nil, false, 0*time.Second)
		s.ErrorIs(err, errUpdateFailed)
		s.ErrorContains(err, "failed to update deployment with input")
		mockCoreClient.AssertExpectations(s.T())
	})
}

// TestWaitForStatus verifies the behavior of createOrUpdateDeployment when the
// waitForStatus flag is toggled. When waitForStatus is true, the helper
// should poll the deployment until it becomes healthy by repeatedly calling
// CoreGetDeployment. When waitForStatus is false (the default), no polling
// should occur. Additional subtests validate error propagation when the
// deployment never becomes healthy (timeout) and when the API returns an
// error during polling.
func (s *Suite) TestWaitForStatus() {
	// a minimal deployment file encoded in YAML that satisfies the required
	// fields for createOrUpdateDeployment.
	const minimalDeploymentYAML = `
deployment:
  configuration:
    name: test-deployment
    workspace_name: test-workspace
    deployment_type: HYBRID
    executor: CeleryExecutor
`

	s.Run("create with wait polls until healthy", func() {
		// HealthPoll should call CoreGetDeployment until it returns a deployment
		// with status HEALTHY.  We simulate one intermediate DEPLOYING status
		// followed by a HEALTHY status. The callCount should therefore end up at 2.
		callCount := 0
		statuses := []astroplatformcore.DeploymentStatus{
			astroplatformcore.DeploymentStatusDEPLOYING,
			astroplatformcore.DeploymentStatusHEALTHY,
		}

		// Save originals so we can restore after the subtest
		origCreate := deployment.CoreCreateDeployment
		origGet := deployment.CoreGetDeployment
		origSleep := deployment.SleepTime
		origTick := deployment.TickNum
		defer func() {
			deployment.CoreCreateDeployment = origCreate
			deployment.CoreGetDeployment = origGet
			deployment.SleepTime = origSleep
			deployment.TickNum = origTick
		}()

		// Patch CoreCreateDeployment to return a stub deployment
		deployment.CoreCreateDeployment = func(orgID string, req astroplatformcore.CreateDeploymentJSONRequestBody, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			return astroplatformcore.Deployment{Id: "test-id", Name: "test-deployment"}, nil
		}
		// Patch CoreGetDeployment to return DEPLOYING on the first call,
		// then HEALTHY on the second call.
		deployment.CoreGetDeployment = func(orgID, deploymentID string, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			status := astroplatformcore.DeploymentStatusDEPLOYING
			if callCount < len(statuses) {
				status = statuses[callCount]
			}
			callCount++
			return astroplatformcore.Deployment{Id: deploymentID, Name: "test-deployment", Status: status}, nil
		}

		// Reduce wait timings so the test does not take long
		deployment.SleepTime = 0
		deployment.TickNum = 1

		// Build the FormattedDeployment from YAML
		var fd inspect.FormattedDeployment
		err := yaml.Unmarshal([]byte(minimalDeploymentYAML), &fd)
		s.Require().NoError(err)

		// Run createOrUpdateDeployment with waitForStatus=true, allow time for 2 polls
		err = createOrUpdateDeployment(&fd, "", "ws-id", createAction, &astroplatformcore.Deployment{}, nil, false, nil, nil, nil, true, 3*time.Second)
		s.NoError(err)
		s.Equal(2, callCount, "expected two polling iterations before becoming healthy")
	})

	s.Run("create without wait does not poll", func() {
		callCount := 0
		origCreate := deployment.CoreCreateDeployment
		origGet := deployment.CoreGetDeployment
		defer func() {
			deployment.CoreCreateDeployment = origCreate
			deployment.CoreGetDeployment = origGet
		}()

		deployment.CoreCreateDeployment = func(orgID string, req astroplatformcore.CreateDeploymentJSONRequestBody, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			return astroplatformcore.Deployment{Id: "test-id", Name: "test-deployment"}, nil
		}
		deployment.CoreGetDeployment = func(orgID, deploymentID string, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			callCount++
			return astroplatformcore.Deployment{Id: deploymentID, Name: "test-deployment", Status: astroplatformcore.DeploymentStatusHEALTHY}, nil
		}

		var fd inspect.FormattedDeployment
		err := yaml.Unmarshal([]byte(minimalDeploymentYAML), &fd)
		s.Require().NoError(err)

		err = createOrUpdateDeployment(&fd, "", "ws-id", createAction, &astroplatformcore.Deployment{}, nil, false, nil, nil, nil, false, 0*time.Second)
		s.NoError(err)
		s.Equal(0, callCount, "expected no polling when waitForStatus is false")
	})

	s.Run("poll times out when deployment never becomes healthy", func() {
		origCreate := deployment.CoreCreateDeployment
		origGet := deployment.CoreGetDeployment
		origSleep := deployment.SleepTime
		origTick := deployment.TickNum
		defer func() {
			deployment.CoreCreateDeployment = origCreate
			deployment.CoreGetDeployment = origGet
			deployment.SleepTime = origSleep
			deployment.TickNum = origTick
		}()

		deployment.CoreCreateDeployment = func(orgID string, req astroplatformcore.CreateDeploymentJSONRequestBody, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			return astroplatformcore.Deployment{Id: "test-id", Name: "test-deployment"}, nil
		}
		// Always return DEPLOYING; HealthPoll will eventually time out.
		deployment.CoreGetDeployment = func(orgID, deploymentID string, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			return astroplatformcore.Deployment{Id: deploymentID, Name: "test-deployment", Status: astroplatformcore.DeploymentStatusDEPLOYING}, nil
		}
		deployment.SleepTime = 0
		deployment.TickNum = 1

		var fd inspect.FormattedDeployment
		err := yaml.Unmarshal([]byte(minimalDeploymentYAML), &fd)
		s.Require().NoError(err)

		err = createOrUpdateDeployment(&fd, "", "ws-id", createAction, &astroplatformcore.Deployment{}, nil, false, nil, nil, nil, true, 100*time.Millisecond)
		s.ErrorIs(err, deployment.ErrTimedOut, "expected ErrTimedOut when deployment does not become healthy")
	})

	s.Run("poll returns API error when CoreGetDeployment errors", func() {
		origCreate := deployment.CoreCreateDeployment
		origGet := deployment.CoreGetDeployment
		origSleep := deployment.SleepTime
		origTick := deployment.TickNum
		defer func() {
			deployment.CoreCreateDeployment = origCreate
			deployment.CoreGetDeployment = origGet
			deployment.SleepTime = origSleep
			deployment.TickNum = origTick
		}()

		deployment.CoreCreateDeployment = func(orgID string, req astroplatformcore.CreateDeploymentJSONRequestBody, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			return astroplatformcore.Deployment{Id: "test-id", Name: "test-deployment"}, nil
		}
		// Simulate an API error
		apiErr := errors.New("api error during polling")
		deployment.CoreGetDeployment = func(orgID, deploymentID string, pc astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
			return astroplatformcore.Deployment{}, apiErr
		}
		deployment.SleepTime = 0
		deployment.TickNum = 1

		var fd inspect.FormattedDeployment
		err := yaml.Unmarshal([]byte(minimalDeploymentYAML), &fd)
		s.Require().NoError(err)

		err = createOrUpdateDeployment(&fd, "", "ws-id", createAction, &astroplatformcore.Deployment{}, nil, false, nil, nil, nil, true, 3*time.Second)
		s.ErrorIs(err, apiErr, "expected error returned from CoreGetDeployment to propagate")
	})
}

func (s *Suite) TestGetCreateOrUpdateInput() {
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
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	s.Run("returns error if worker type does not match existing pools", func() {
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

		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.ErrorContains(err, "worker_type: test-worker-8 does not exist in cluster: test-cluster")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns error if queue options are invalid", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.ErrorContains(err, "worker queue option is invalid: min worker count")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns error if getting worker queue options fails", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.ErrorIs(err, errTest)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("sets default queue options if none were requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if more than one worker queue are requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.ErrorContains(err, "don't use 'worker_queues' to update default queue with KubernetesExecutor")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to CreateDeploymentInput if no queues were requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, nil, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to CreateDeploymentInput if Kubernetes executor was requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns correct deployment input when multiple queues are requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &astroplatformcore.Deployment{}, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to UpdateDeploymentInput if no queues were requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "create", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if the cluster is being changed", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, "diff-cluster", workspaceID, "update", &existingDeployment, nil, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.ErrorIs(err, errNotPermitted)
		s.ErrorContains(err, "changing an existing deployment's cluster is not permitted")
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with no queues", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with a queue", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with a queue on standard", func() {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
		deploymentFromFile.Deployment.Configuration.DeploymentType = "STANDARD"
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
		defaultTaskPodMemory = "10"
		defaultTaskPodCPU := "10"
		resourceQuotaCPU := "10"
		resourceQuotaMemory := "10"
		existingDeployment := astroplatformcore.Deployment{
			Id:                   deploymentID,
			Name:                 "test-deployment",
			ClusterId:            &clusterID,
			ClusterName:          &clusterName,
			Executor:             &executorCelery,
			WorkerQueues:         &expectedQList,
			DefaultTaskPodMemory: &defaultTaskPodMemory,
			DefaultTaskPodCpu:    &defaultTaskPodCPU,
			ResourceQuotaCpu:     &resourceQuotaCPU,
			ResourceQuotaMemory:  &resourceQuotaMemory,
		}

		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("transforms formattedDeployment to UpdateDeploymentInput if Kubernetes executor was requested with a queue on dedicated", func() {
		deploymentID = "test-deployment-id"
		deploymentFromFile = inspect.FormattedDeployment{}
		deploymentFromFile.Deployment.Configuration.ClusterName = "test-cluster"
		deploymentFromFile.Deployment.Configuration.Name = "test-deployment-modified"
		deploymentFromFile.Deployment.Configuration.Description = "test-description"
		deploymentFromFile.Deployment.Configuration.RunTimeVersion = "test-runtime-v"
		deploymentFromFile.Deployment.Configuration.SchedulerAU = 4
		deploymentFromFile.Deployment.Configuration.SchedulerCount = 2
		deploymentFromFile.Deployment.Configuration.Executor = deployment.KubeExecutor
		deploymentFromFile.Deployment.Configuration.DeploymentType = "DEDICATED"
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
		defaultTaskPodMemory = "10"
		defaultTaskPodCPU := "10"
		resourceQuotaCPU := "10"
		resourceQuotaMemory := "10"
		existingDeployment := astroplatformcore.Deployment{
			Id:                   deploymentID,
			Name:                 "test-deployment",
			ClusterId:            &clusterID,
			ClusterName:          &clusterName,
			Executor:             &executorCelery,
			WorkerQueues:         &expectedQList,
			DefaultTaskPodMemory: &defaultTaskPodMemory,
			DefaultTaskPodCpu:    &defaultTaskPodCPU,
			ResourceQuotaCpu:     &resourceQuotaCPU,
			ResourceQuotaMemory:  &resourceQuotaMemory,
		}

		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns correct update deployment input when multiple queues are requested", func() {
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
		err = createOrUpdateDeployment(&deploymentFromFile, clusterID, workspaceID, "update", &existingDeployment, existingPools, dagDeploy, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, mockCoreClient, mockPlatformCoreClient, false, 0*time.Second)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
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
		s.ErrorContains(err, "is not valid. It can be CeleryExecutor, KubernetesExecutor, or AstroExecutor")
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
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     &val2,
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
		s.ErrorContains(err, "missing required field: default queue is missing under deployment.worker_queues")
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
		clusterName, expectedClusterID, actualClusterID string
		actualNodePools                                 []astroplatformcore.NodePool
		err                                             error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedClusterID = "test-cluster-id"
	clusterName = "test-cluster"
	s.Run("returns a cluster id if cluster exists in organization", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgID, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(expectedClusterID, actualClusterID)
	})
	s.Run("returns error from api if listing cluster fails", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListClustersResponse{}, errTest).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgID, mockPlatformCoreClient)
		s.ErrorIs(err, errTest)
		s.Equal("", actualClusterID)
		s.Equal([]astroplatformcore.NodePool(nil), actualNodePools)
	})
	s.Run("returns an error if cluster does not exist in organization", func() {
		mockListClustersResponse = astroplatformcore.ListClustersResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.ClustersPaginated{
				Clusters: []astroplatformcore.Cluster{},
			},
		}
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		actualClusterID, actualNodePools, err = getClusterInfoFromName(clusterName, mockOrgID, mockPlatformCoreClient)
		s.ErrorIs(err, errNotFound)
		s.ErrorContains(err, "cluster_name: test-cluster does not exist in organization")
		s.Equal("", actualClusterID)
		s.Equal([]astroplatformcore.NodePool(nil), actualNodePools)
	})
}

func (s *Suite) TestGetWorkspaceIDFromName() {
	var (
		workspaceName, expectedWorkspaceID, actualWorkspaceID string
		err                                                   error
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkspaceID = "test-ws-id"
	workspaceName = "test-workspace"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	s.Run("returns a workspace id if workspace exists in organization", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, mockCoreClient)
		s.NoError(err)
		s.Equal(expectedWorkspaceID, actualWorkspaceID)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns error from api if listing workspace fails", func() {
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errTest).Once()
		actualWorkspaceID, err = getWorkspaceIDFromName(workspaceName, mockCoreClient)
		s.ErrorIs(err, errTest)
		s.Equal("", actualWorkspaceID)
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetNodePoolIDFromName() {
	var (
		workerType, expectedPoolID, actualPoolID, clusterID string
		existingPools                                       []astroplatformcore.NodePool
		err                                                 error
	)
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
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     &val2,
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
		actualEnvVars      []astroplatformcore.DeploymentEnvironmentVariableRequest
		deploymentFromFile inspect.FormattedDeployment
		err                error
	)

	s.Run("creates env vars if they were requested in a formatted deployment", func() {
		deploymentFromFile = inspect.FormattedDeployment{}
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     &val2,
			},
			{
				IsSecret:  true,
				Key:       "key-3",
				UpdatedAt: "",
			},
		}
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
			{
				IsSecret: true,
				Key:      "key-3",
				Value:    nil,
			},
		}
		deploymentFromFile.Deployment.EnvVars = list
		actualEnvVars = createEnvVarsRequest(&deploymentFromFile)
		s.NoError(err)
		s.Equal(expectedList, actualEnvVars)
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
		deploymentFromFile inspect.FormattedDeployment
		existingWQList     []astroplatformcore.WorkerQueue
		existingPools      []astroplatformcore.NodePool
		err                error
	)
	s.Run("returns list of queues for the requested deployment", func() {
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
		s.NoError(err)
	})
	s.Run("returns updated list of existing and queues being added", func() {
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
		s.NoError(err)
	})
	s.Run("returns updated list when multiple queue operations are requested", func() {
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
		s.NoError(err)
	})
	s.Run("returns one default queue regardless of any existing queues", func() {
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
		s.NoError(err)
	})
	s.Run("returns an error if unable to determine nodepool id", func() {
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
		s.ErrorContains(err, "worker_type: test-worker-4 does not exist in cluster: test-cluster")
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
		existingDeployments []astroplatformcore.Deployment
		deploymentToCreate  string
		expectedeployment   astroplatformcore.Deployment
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
	deploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &expectedeployment,
	}
	deploymentToCreate = "test-deployment-2"
	s.Run("returns the deployment id for the matching deployment name", func() {
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		actual, err := deploymentFromName(existingDeployments, deploymentToCreate, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(expectedeployment, actual)
	})
	s.Run("returns empty string if deployment name does not match", func() {
		deploymentToCreate = "test-d-2"
		expectedeployment = astroplatformcore.Deployment{}
		actual, err := deploymentFromName(existingDeployments, deploymentToCreate, mockPlatformCoreClient)
		s.NoError(err)
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
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     &val2,
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
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
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
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "",
				UpdatedAt: "",
				Value:     &val2,
			},
		}
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "update")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.environment_variables[1].key")
	})
	s.Run("returns an error if env var values are missing on update for non-secret env var", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
			},
		}
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "update")
		s.ErrorIs(err, errRequiredField)
		s.ErrorContains(err, "missing required field: deployment.environment_variables[0].value")
	})
	s.Run("returns nil if env var values are valid on update", func() {
		input.Deployment.Configuration.Name = "test-deployment"
		input.Deployment.Configuration.ClusterName = "test-cluster-id"
		list := []inspect.EnvironmentVariable{
			{
				IsSecret:  false,
				Key:       "key-1",
				UpdatedAt: "",
				Value:     &val1,
			},
			{
				IsSecret:  true,
				Key:       "key-2",
				UpdatedAt: "",
				Value:     &val2,
			},
			{
				IsSecret:  true,
				Key:       "key-3",
				UpdatedAt: "",
			},
		}
		input.Deployment.EnvVars = list
		err = checkEnvVars(&input, "update")
		s.NoError(err)
	})
}
