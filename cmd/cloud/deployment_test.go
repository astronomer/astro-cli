package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroiamcore_mocks "github.com/astronomer/astro-cli/astro-client-iam-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockTokenID              = "ck05r3bor07h40d02y2hw4n4t"
	csID                     = "test-cluster-id"
	testCluster              = "test-cluster"
	mockListClustersResponse = astroplatformcore.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.ClustersPaginated{
			Clusters: []astroplatformcore.Cluster{
				{
					Id:        csID,
					Name:      testCluster,
					NodePools: &nodePools,
				},
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
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
	mockWorkloadIdentity             = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	mockCoreDeploymentCreateResponse = []astroplatformcore.Deployment{
		{
			Name:             "test-deployment-label",
			Status:           "HEALTHY",
			WorkloadIdentity: &mockWorkloadIdentity,
			ClusterId:        &clusterID,
			ClusterName:      &testCluster,
			Id:               "test-id-1",
		},
	}
	mockUpdateDeploymentResponse = astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Name:          "test-deployment-label",
			Id:            "test-id-1",
			CloudProvider: (*astroplatformcore.DeploymentCloudProvider)(&cloudProvider),
			Type:          &hybridType,
			ClusterId:     &clusterID,
			Region:        &region,
			ClusterName:   &cluster.Name,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	executorCelery       = astroplatformcore.DeploymentExecutorCELERY
	highAvailabilityTest = true
	developmentModeTest  = true
	ResourceQuotaMemory  = "1"
	schedulerTestSize    = astroplatformcore.DeploymentSchedulerSizeSMALL
	deploymentResponse   = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                     "test-id-1",
			RuntimeVersion:         "4.2.5",
			Namespace:              "test-name",
			WorkspaceId:            "workspace-id",
			WebServerUrl:           "test-url",
			IsDagDeployEnabled:     false,
			Description:            &description,
			Name:                   "test-deployment-label",
			Status:                 "HEALTHY",
			Type:                   &hybridType,
			SchedulerAu:            &schedulerAU,
			ClusterId:              &csID,
			ClusterName:            &testCluster,
			Executor:               &executorCelery,
			IsHighAvailability:     &highAvailabilityTest,
			IsDevelopmentMode:      &developmentModeTest,
			ResourceQuotaCpu:       &resourceQuotaCPU,
			ResourceQuotaMemory:    &ResourceQuotaMemory,
			SchedulerSize:          &schedulerTestSize,
			Region:                 &region,
			WorkspaceName:          &workspaceName,
			CloudProvider:          (*astroplatformcore.DeploymentCloudProvider)(&cloudProvider),
			DefaultTaskPodCpu:      &defaultTaskPodCPU,
			DefaultTaskPodMemory:   &defaultTaskPodMemory,
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astroplatformcore.WorkerQueue{},
		},
	}
	hostedDeploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                     "test-id-1",
			RuntimeVersion:         "4.2.5",
			Namespace:              "test-name",
			WorkspaceId:            "workspace-id",
			WebServerUrl:           "test-url",
			IsDagDeployEnabled:     false,
			Description:            &description,
			Name:                   "test-deployment-label",
			Status:                 "HEALTHY",
			Type:                   &standardType,
			ClusterId:              &csID,
			ClusterName:            &testCluster,
			Executor:               &executorCelery,
			IsHighAvailability:     &highAvailabilityTest,
			IsDevelopmentMode:      &developmentModeTest,
			ResourceQuotaCpu:       &resourceQuotaCPU,
			ResourceQuotaMemory:    &ResourceQuotaMemory,
			SchedulerSize:          &schedulerTestSize,
			Region:                 &region,
			WorkspaceName:          &workspaceName,
			CloudProvider:          (*astroplatformcore.DeploymentCloudProvider)(&cloudProvider),
			DefaultTaskPodCpu:      &defaultTaskPodCPU,
			DefaultTaskPodMemory:   &defaultTaskPodMemory,
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astroplatformcore.WorkerQueue{},
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
	standardType               = astroplatformcore.DeploymentTypeSTANDARD
	hybridType                 = astroplatformcore.DeploymentTypeHYBRID
	testRegion                 = "region"
	testProvider               = "provider"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:                "test-id-1",
			Name:              "test",
			Status:            "HEALTHY",
			Type:              &standardType,
			Region:            &testRegion,
			CloudProvider:     (*astroplatformcore.DeploymentCloudProvider)(&testProvider),
			WorkspaceName:     &workspaceName,
			IsDevelopmentMode: &developmentModeTest,
		},
		{
			Id:            "test-id-2",
			Name:          "test-2",
			Status:        "HEALTHY",
			Type:          &hybridType,
			ClusterName:   &testCluster,
			WorkspaceName: &workspaceName,
		},
	}
	mockGetDeploymentLogsResponse = astrocore.GetDeploymentLogsResponse{
		JSON200: &astrocore.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
			Results: []astrocore.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrocore.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceScheduler,
				},
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	mockGetDeploymentLogsMultipleComponentsResponse = astrocore.GetDeploymentLogsResponse{
		JSON200: &astrocore.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
			Results: []astrocore.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrocore.DeploymentLogEntrySourceWebserver,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceTriggerer,
				},
				{
					Raw:       "test log line 3",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 4",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceWorker,
				},
			},
			SearchId: "search-id",
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
	mockCreateDeploymentResponse = astroplatformcore.CreateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Id:            "test-id",
			CloudProvider: (*astroplatformcore.DeploymentCloudProvider)(&cloudProvider),
			Type:          &hybridType,
			Region:        &region,
			ClusterName:   &cluster.Name,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	nodePools = []astroplatformcore.NodePool{
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
	cluster = astroplatformcore.Cluster{
		Id:        "test-cluster-id",
		Name:      "test-cluster",
		NodePools: &nodePools,
	}
	mockGetClusterResponse = astroplatformcore.GetClusterResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &cluster,
	}
)

func execDeploymentCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newDeploymentRootCmd(os.Stdout)
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "deployment")
}

func TestDeploymentList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	platformCoreClient = mockPlatformCoreClient

	cmdArgs := []string{"list", "-a"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-id-2")
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeploymentLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Times(3)
	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient

	cmdArgs := []string{"logs", "test-id-1", "-w"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "-e"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockPlatformCoreClient.AssertExpectations(t)
	mockCoreClient.AssertExpectations(t)
}

func TestDeploymentLogsMultipleComponents(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsMultipleComponentsResponse, nil).Times(3)
	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient

	cmdArgs := []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-w"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-e"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockPlatformCoreClient.AssertExpectations(t)
	mockCoreClient.AssertExpectations(t)
}

func TestDeploymentCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	ws := "workspace-id"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	mockResponse := &airflowversions.Response{
		RuntimeVersions: map[string]airflowversions.RuntimeVersion{
			"4.2.5": {Metadata: airflowversions.RuntimeVersionMetadata{AirflowVersion: "2.2.5", Channel: "stable"}, Migrations: airflowversions.RuntimeVersionMigrations{}},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})
	t.Run("creates a deployment when dag-deploy is disabled", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable"}
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("creates a deployment when dag deploy is enabled", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "enable"}
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("creates a deployment when executor is specified", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubernetesExecutor"}
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("creates a deployment with default executor", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable"}
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if dag-deploy flag has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "some-value"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if cluster-type flag has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--cluster-type", "some-value"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if type flag has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--type", "some-value"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if cicd-enforcement flag has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--cicd-enforcement", "some-value"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if executor has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubeExecutor"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "KubeExecutor is not a valid executor")
	})
	t.Run("creates a deployment from file", func(t *testing.T) {
		filePath := "./test-deployment.yaml"
		data := `
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
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)

		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml"}
		astroCoreClient = mockCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if creating a deployment from file fails", func(t *testing.T) {
		cmdArgs := []string{"create", "--deployment-file", "test-file-name.json"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "open test-file-name.json: no such file or directory")
	})
	t.Run("returns an error if from-file is specified with any other flags", func(t *testing.T) {
		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml", "--description", "fail"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorIs(t, err, errFlag)
	})
	t.Run("creates a deployment with cloud provider and region", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("organization_short_name", "test-org")
		ctx.SetContextKey("workspace", ws)

		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--cloud-provider", "gcp", "--region", "us-central1",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error with incorrect high-availability value", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization_short_name", "test-org")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "gcp", "--region", "us-east1", "--high-availability", "some-value",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --high-availability value")
	})
	t.Run("returns an error with incorrect development-mode value", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization_short_name", "test-org")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "gcp", "--region", "us-east1", "--development-mode", "some-value",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --development-mode value")
	})
	t.Run("returns an error if cloud provider is not valid", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "azure",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "azure is not a valid cloud provider. It can only be gcp")
	})
	t.Run("creates a hosted dedicated deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		ctx.SetContextKey("organization_short_name", "test-org")
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated",
		}

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "1")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if incorrect cluster type is passed for a hosted dedicated deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		astroCoreClient = mockCoreClient
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "wrong-value",
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --type value")
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("creates an extra large deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		extraLarge := astroplatformcore.DeploymentSchedulerSizeEXTRALARGE
		mockCreateDeploymentResponse.JSON200.SchedulerSize = &extraLarge
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		ctx.SetContextKey("organization_short_name", "test-org")
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--scheduler-size", "extra-large",
		}

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "1")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestDeploymentUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	ws := "test-ws-id"

	t.Run("updates the deployment successfully", func(t *testing.T) {
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		cmdArgs := []string{"update", "test-id-1", "--name", "test", "--workspace-id", ws, "--force"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("updates the deployment successfully to enable ci-cd enforcement", func(t *testing.T) {
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd", "enable"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("updates the deployment successfully to disable ci-cd enforcement", func(t *testing.T) {
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd", "disable"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if ci-cd enforcement has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--cicd-enforcement", "some-value"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if cluster-type enforcement has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--cluster-type", "some-value"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if type enforcement has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--type", "some-value"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if dag-deploy has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--dag-deploy", "some-value"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if executor has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--executor", "KubeExecutor"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "KubeExecutor is not a valid executor")
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Workspace = ""
		err = ctx.SetContext()
		assert.NoError(t, err)
		defer testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"update", "-n", "doesnotexist"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to find a valid Workspace")
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("updates a deployment from file", func(t *testing.T) {
		filePath := "./test-deployment.yaml"
		data := `
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
		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		cmdArgs := []string{"update", "--deployment-file", "test-deployment.yaml"}
		astroCoreClient = mockCoreClient
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if updating a deployment from file fails", func(t *testing.T) {
		cmdArgs := []string{"update", "--deployment-file", "test-file-name.json"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "open test-file-name.json: no such file or directory")
	})
	t.Run("returns an error if from-file is specified with any other flags", func(t *testing.T) {
		cmdArgs := []string{"update", "--deployment-file", "test-deployment.yaml", "--description", "fail"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorIs(t, err, errFlag)
	})
	t.Run("updates a deployment with small scheduler size", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)

		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Times(1)

		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "small", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error with incorrect high-availability value", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--high-availability", "some-value", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --high-availability value")
	})
	t.Run("returns an error with incorrect development-mode value", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--development-mode", "some-value", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --development-mode value")
	})
	t.Run("returns an error if cluster-id is provided with implicit standard deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "flag --cluster-id cannot be used to create a standard deployment")
	})
	t.Run("returns an error if cluster-id is provided with explicit standard deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--type", standard}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "flag --cluster-id cannot be used to create a standard deployment")
	})

	t.Run("updates a deployment with extra large scheduler size", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)

		astroCoreClient = mockCoreClient
		platformCoreClient = mockPlatformCoreClient

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Times(1)

		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "extra_large", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	mockDeleteDeploymentResponse := astroplatformcore.DeleteDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	platformCoreClient = mockPlatformCoreClient

	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockPlatformCoreClient.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, nil).Times(1)

	cmdArgs := []string{"delete", "test-id-1", "--force"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeploymentVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

	platformCoreClient = mockPlatformCoreClient

	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	value := "test-value-1"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &value,
			IsSecret: false,
		},
	}

	cmdArgs := []string{"variable", "list", "--deployment-id", "test-id-1"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	mockPlatformCoreClient.AssertExpectations(t)
}

func TestDeploymentVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient
	value := "test-value-1"
	value2 := "test-value-2"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &value,
			IsSecret: false,
		},
		{
			Key:      "test-key-2",
			Value:    &value2,
			IsSecret: false,
		},
	}

	mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
	mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

	cmdArgs := []string{"variable", "create", "test-key-3=test-value-3", "--deployment-id", "test-id-1", "--key", "test-key-2", "--value", "test-value-2"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2")
	mockPlatformCoreClient.AssertExpectations(t)
	mockCoreClient.AssertExpectations(t)
}

func TestDeploymentVariableUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient

	value := "test-value-1"
	value2 := "test-value-2"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &value,
			IsSecret: false,
		},
		{
			Key:      "test-key-2",
			Value:    &value2,
			IsSecret: false,
		},
	}

	mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
	mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
	valueUpdate := "test-value-update"
	valueUpdate2 := "test-value-2-update"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &valueUpdate,
			IsSecret: false,
		},
		{
			Key:      "test-key-2",
			Value:    &valueUpdate2,
			IsSecret: false,
		},
	}
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

	cmdArgs := []string{"variable", "update", "test-key-2=test-value-2-update", "--deployment-id", "test-id-1", "--key", "test-key-1", "--value", "test-value-update"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-update")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2-update")
	mockPlatformCoreClient.AssertExpectations(t)
	mockCoreClient.AssertExpectations(t)
}

func TestDeploymentHibernateAndWakeUp(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	tests := []struct {
		IsHibernating bool
		command       string
	}{
		{true, "hibernate"},
		{false, "wake-up"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
			platformCoreClient = mockPlatformCoreClient

			isActive := true
			mockResponse := astroplatformcore.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: astrocore.HTTPStatus200,
				},
				JSON200: &astroplatformcore.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					IsActive:      &isActive,
				},
			}

			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockPlatformCoreClient.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(t, "1")()

			cmdArgs := []string{tt.command, "", "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockPlatformCoreClient.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with until", tt.command), func(t *testing.T) {
			mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
			platformCoreClient = mockPlatformCoreClient

			until := "2022-11-17T13:25:55.275697-08:00"
			untilParsed, err := time.Parse(time.RFC3339, until)
			assert.NoError(t, err)
			isActive := true
			mockResponse := astroplatformcore.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: astrocore.HTTPStatus200,
				},
				JSON200: &astroplatformcore.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &untilParsed,
					IsActive:      &isActive,
				},
			}

			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockPlatformCoreClient.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--until", until, "--force"}
			_, err = execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockPlatformCoreClient.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with until returns an error if invalid", tt.command), func(t *testing.T) {
			until := "invalid-duration"

			cmdArgs := []string{tt.command, "test-id-1", "--until", until, "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.Error(t, err)
		})

		t.Run(fmt.Sprintf("%s with for", tt.command), func(t *testing.T) {
			mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
			platformCoreClient = mockPlatformCoreClient

			forDuration := "1h"
			forDurationParsed, err := time.ParseDuration(forDuration)
			overrideUntil := time.Now().Add(forDurationParsed)
			assert.NoError(t, err)
			isActive := true
			mockResponse := astroplatformcore.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: astrocore.HTTPStatus200,
				},
				JSON200: &astroplatformcore.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &overrideUntil,
					IsActive:      &isActive,
				},
			}

			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockPlatformCoreClient.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--for", forDuration, "--force"}
			_, err = execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockPlatformCoreClient.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with for returns an error if invalid", tt.command), func(t *testing.T) {
			forDuration := "invalid-duration"

			cmdArgs := []string{tt.command, "test-id-1", "--for", forDuration, "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.Error(t, err)
		})

		t.Run(fmt.Sprintf("%s with remove override", tt.command), func(t *testing.T) {
			mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
			platformCoreClient = mockPlatformCoreClient

			mockResponse := astroplatformcore.DeleteDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: astrocore.HTTPStatus204,
				},
			}

			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockPlatformCoreClient.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--remove-override", "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockPlatformCoreClient.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s returns an error when getting workspace fails", tt.command), func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, err := config.GetCurrentContext()
			assert.NoError(t, err)
			ctx.Workspace = ""
			err = ctx.SetContext()
			assert.NoError(t, err)
			defer testUtil.InitTestConfig(testUtil.LocalPlatform)
			expectedOut := "Usage:\n"
			cmdArgs := []string{tt.command, "-n", "doesnotexist"}
			resp, err := execDeploymentCmd(cmdArgs...)
			assert.ErrorContains(t, err, "failed to find a valid workspace")
			assert.Contains(t, resp, expectedOut)
		})
	}
}

func TestIsValidExecutor(t *testing.T) {
	t.Run("returns true for Kubernetes Executor", func(t *testing.T) {
		actual := isValidExecutor(deployment.KubeExecutor)
		assert.True(t, actual)
	})
	t.Run("returns true for Celery Executor", func(t *testing.T) {
		actual := isValidExecutor(deployment.CeleryExecutor)
		assert.True(t, actual)
	})
	t.Run("returns true if executor is CELERY", func(t *testing.T) {
		actual := isValidExecutor(deployment.CELERY)
		assert.True(t, actual)
	})
	t.Run("returns true if executor is KUBERNETES", func(t *testing.T) {
		actual := isValidExecutor(deployment.KUBERNETES)
		assert.True(t, actual)
	})
	t.Run("returns true if executor is celery", func(t *testing.T) {
		actual := isValidExecutor("celery")
		assert.True(t, actual)
	})
	t.Run("returns true if executor is kubernetes", func(t *testing.T) {
		actual := isValidExecutor("kubernetes")
		assert.True(t, actual)
	})
	t.Run("returns true if executor is celery", func(t *testing.T) {
		actual := isValidExecutor("celeryexecutor")
		assert.True(t, actual)
	})
	t.Run("returns true if executor is kubernetes", func(t *testing.T) {
		actual := isValidExecutor("kubernetesexecutor")
		assert.True(t, actual)
	})
	t.Run("returns true when no Executor is requested", func(t *testing.T) {
		actual := isValidExecutor("")
		assert.True(t, actual)
	})
	t.Run("returns false for any invalid executor", func(t *testing.T) {
		actual := isValidExecutor("KubeExecutor")
		assert.False(t, actual)
	})
}

func TestIsValidCloudProvider(t *testing.T) {
	t.Run("returns true if cloudProvider is gcp", func(t *testing.T) {
		actual := isValidCloudProvider("gcp")
		assert.True(t, actual)
	})
	t.Run("returns true if cloudProvider is aws", func(t *testing.T) {
		actual := isValidCloudProvider("aws")
		assert.True(t, actual)
	})
	t.Run("returns false if cloudProvider is not gcp", func(t *testing.T) {
		actual := isValidCloudProvider("azure")
		assert.False(t, actual)
	})
}

var (
	tokenValue       = "token"
	mockDeploymentID = "ck05r3bor07h40d02y2hw4n4v"
	deploymentRole   = "DEPLOYMENT_ADMIN"
	deploymentUser1  = astrocore.User{
		CreatedAt:      time.Now(),
		FullName:       "user 1",
		Id:             "user1-id",
		DeploymentRole: &deploymentRole,
		Username:       "user@1.com",
	}
	deploymentUsers = []astrocore.User{
		deploymentUser1,
	}

	deploymentTeams = []astrocore.Team{
		team1,
	}
	ListDeploymentUsersResponseOK = astrocore.ListDeploymentUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      deploymentUsers,
		},
	}
	ListDeploymentUsersResponseError = astrocore.ListDeploymentUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateDeploymentUserRoleResponseOK = astrocore.MutateDeploymentUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UserRole{
			Role: "DEPLOYMENT_ADMIN",
		},
	}
	MutateDeploymentUserRoleResponseError = astrocore.MutateDeploymentUserRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteDeploymentUserResponseOK = astrocore.DeleteDeploymentUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentUserResponseError = astrocore.DeleteDeploymentUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	ListDeploymentTeamsResponseOK = astrocore.ListDeploymentTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      deploymentTeams,
		},
	}
	ListDeploymentTeamsResponseError = astrocore.ListDeploymentTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorBodyList,
		JSON200: nil,
	}
	MutateDeploymentTeamRoleResponseOK = astrocore.MutateDeploymentTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamRole{
			Role: "DEPLOYMENT_ADMIN",
		},
	}
	MutateDeploymentTeamRoleResponseError = astrocore.MutateDeploymentTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    teamRequestErrorBodyUpdate,
		JSON200: nil,
	}
	DeleteDeploymentTeamResponseOK = astrocore.DeleteDeploymentTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentTeamResponseError = astrocore.DeleteDeploymentTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: teamRequestErrorDelete,
	}

	tokenDeploymentRole = astrocore.ApiTokenRole{
		EntityType: "DEPLOYMENT",
		EntityId:   deploymentID,
		Role:       "DEPLOYMENT_ADMIN",
	}

	tokenDeploymentRole2 = astrocore.ApiTokenRole{
		EntityType: "DEPLOYMENT",
		EntityId:   deploymentID,
		Role:       "custom role",
	}

	deploymentAPIToken1 = astrocore.ApiToken{Id: "token1", Name: tokenName1, Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{tokenDeploymentRole}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	deploymentAPITokens = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "Type 2", Roles: []astrocore.ApiTokenRole{tokenDeploymentRole2}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}

	ListDeploymentAPITokensResponseOK = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			ApiTokens:  deploymentAPITokens,
		},
	}
	apiTokenRequestErrorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list api tokens",
	})
	ListDeploymentAPITokensResponseError = astrocore.ListDeploymentApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    apiTokenRequestErrorBodyList,
		JSON200: nil,
	}
	CreateDeploymentAPITokenRoleResponseOK = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}
	apiTokenRequestErrorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create api token",
	})
	CreateDeploymentAPITokenRoleResponseError = astrocore.CreateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    apiTokenRequestErrorBodyCreate,
		JSON200: nil,
	}
	MutateDeploymentAPITokenRoleResponseOK = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}
	apiTokenRequestErrorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update api token",
	})
	MutateDeploymentAPITokenRoleResponseError = astrocore.UpdateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    apiTokenRequestErrorBodyUpdate,
		JSON200: nil,
	}
	DeleteDeploymentAPITokenResponseOK = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	apiTokenRequestErrorDelete, _ = json.Marshal(astrocore.Error{
		Message: "failed to delete api token",
	})
	DeleteDeploymentAPITokenResponseError = astrocore.DeleteDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: apiTokenRequestErrorDelete,
	}

	RotateDeploymentAPITokenResponseOK = astrocore.RotateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}

	apiTokenRequestErrorRotate, _ = json.Marshal(astrocore.Error{
		Message: "failed to rotate api token",
	})
	RotateDeploymentAPITokenResponseError = astrocore.RotateDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: apiTokenRequestErrorRotate,
	}
	GetDeploymentAPITokenWithResponseOK = astrocore.GetDeploymentApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &deploymentAPIToken1,
	}
)

func TestDeploymentUserList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"user", "list", "-h"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("any errors from api are returned and users are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list users")
	})
	t.Run("any context errors from api are returned and users are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestDeploymentUserUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"user", "update", "-h"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("valid email with valid role updates user", func(t *testing.T) {
		expectedOut := "The deployment user user@1.com role was successfully updated to DEPLOYMENT_ADMIN"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "update", "user@1.com", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The deployment user user@1.com role was successfully updated to DEPLOYMENT_ADMIN"
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "update", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestDeploymentUserAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"user", "add", "-h"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("valid email with valid role adds user", func(t *testing.T) {
		expectedOut := "The user user@1.com was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any errors from api are returned and user is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user", "--deployment-id", mockDeploymentID)
	})

	t.Run("any context errors from api are returned and role is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "add", "user@1.com", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The user user@1.com was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"
		mockClient.On("MutateDeploymentUserRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentUserRoleResponseOK, nil).Once()

		cmdArgs := []string{"user", "add", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestDeploymentUserRemove(t *testing.T) {
	expectedHelp := "Remove a user from an Astro Deployment"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints remove help", func(t *testing.T) {
		cmdArgs := []string{"user", "remove", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("valid email removes user", func(t *testing.T) {
		expectedOut := "The user user@1.com was successfully removed from the deployment"
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentUserResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any errors from api are returned and user is not removed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentUserResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update user")
	})
	t.Run("any context errors from api are returned and the user is not removed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentUserResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"user", "remove", "user@1.com", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no email is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentUsersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentUsersResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The user user@1.com was successfully removed from the deployment"
		mockClient.On("DeleteDeploymentUserWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentUserResponseOK, nil).Once()

		cmdArgs := []string{"user", "remove", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestDeploymentTeamList(t *testing.T) {
	expectedHelp := "List all the teams in an Astro Deployment"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"team", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("any errors from api are returned and teams are not listed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to list teams")
	})
	t.Run("any context errors from api are returned and teams are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestDeploymentTeamUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"team", "update", "-h"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("valid id with valid role updates team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("The deployment team %s role was successfully updated to DEPLOYMENT_ADMIN", team1.Id)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any errors from api are returned and role is not updated", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("any context errors from api are returned and role is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "update", team1.Id, "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("command asks for input when no team id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("The deployment team %s role was successfully updated to DEPLOYMENT_ADMIN", team1.Id)
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "update", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestDeploymentTeamAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints add help", func(t *testing.T) {
		cmdArgs := []string{"team", "add", "-h"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("valid id with valid role adds team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("The team %s was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n", team1.Id)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})

	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("any errors from api are returned and team is not added", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("any context errors from api are returned and role is not added", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "add", team1.Id, "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("command asks for input when no team id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("The team %s was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n", team1.Id)
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "add", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestDeploymentTeamRemove(t *testing.T) {
	expectedHelp := "Remove a team from an Astro Deployment"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints remove help", func(t *testing.T) {
		cmdArgs := []string{"team", "remove", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("valid id removes team", func(t *testing.T) {
		expectedOut := fmt.Sprintf("Astro Team %s was successfully removed from deployment %s\n", team1.Name, mockDeploymentID)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id, "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("any errors from api are returned and team is not removed", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseError, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to delete team")
	})
	t.Run("any context errors from api are returned and the team is not removed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseOK, nil).Once()
		astroCoreClient = mockClient
		cmdArgs := []string{"team", "remove", team1.Id, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("command asks for input when no id is passed in as an arg", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro Team %s was successfully removed from deployment %s", team1.Name, mockDeploymentID)
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseOK, nil).Once()
		astroCoreClient = mockClient

		cmdArgs := []string{"team", "remove", "--deployment-id", mockDeploymentID}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOut)
	})
}

func TestDeploymentTokenRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"token", "-h"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestDeploymentTokenList(t *testing.T) {
	expectedHelp := "List all the API tokens in an Astro Deployment"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})

	t.Run("any errors from api are returned and tokens are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to list api tokens")
	})
	t.Run("any context errors from api are returned and tokens are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("tokens are listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestDeploymentTokenCreate(t *testing.T) {
	expectedHelp := "Create an API token in an Astro Deployment"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})

	t.Run("any errors from api are returned and token is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenRoleResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to create api token")
	})
	t.Run("any context errors from api are returned and token is not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenRoleResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenRoleResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenRoleResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("Token 1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--role", "DEPLOYMENT_ADMIN", "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is created with no role provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenRoleResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "create", "--name", "Token 1", "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestDeploymentTokenUpdate(t *testing.T) {
	expectedHelp := "Update a Deployment API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	tokenID = ""

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})

	t.Run("any errors from api are returned and token is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentAPITokenRoleResponseError, nil)
		astroCoreClient = mockClient

		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		astroCoreIamClient = mockIamClient

		cmdArgs := []string{"token", "update", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to update api token")
	})
	t.Run("any context errors from api are returned and token is not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentAPITokenRoleResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentAPITokenRoleResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("token is updated with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentAPITokenRoleResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("token is updated with id provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentAPITokenWithResponseOK, nil)
		mockClient.On("UpdateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentAPITokenRoleResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "update", mockTokenID, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestDeploymentTokenRotate(t *testing.T) {
	tokenID = ""
	expectedHelp := "Rotate a Deployment API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "rotate", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})

	t.Run("any errors from api are returned and token is not rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to rotate api token")
	})
	t.Run("any context errors from api are returned and token is not rotated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is rotated with name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--force", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("token is rotated with no ID or name provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--force", "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is rotated with and confirmed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("y")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("token is rotated with id provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentAPITokenWithResponseOK, nil)
		mockClient.On("RotateDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "rotate", mockTokenID, "--force", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestDeploymentTokenDelete(t *testing.T) {
	tokenID = ""
	expectedHelp := "Delete a Deployment API token"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"token", "delete", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("will error if deployment id flag is not provided", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", apiToken1.Id}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "flag --deployment-id is required")
	})
	t.Run("any errors from api are returned and token is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseError, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--force", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to delete api token")
	})
	t.Run("any context errors from api are returned and token is not deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("token is deleted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--force", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("token is deleted with no ID provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--force", "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is delete with and confirmed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		// mock os.Stdin
		expectedInput := []byte("y")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", "--name", tokenName1, "--deployment-id", mockDeploymentID}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("token is deleted with id provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentAPITokenWithResponseOK, nil)
		mockClient.On("DeleteDeploymentApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		astroCoreClient = mockClient
		cmdArgs := []string{"token", "delete", mockTokenID, "--force", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestAddWorkspaceTokenDeploymentRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Create", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()

		defer testUtil.MockUserInput(t, "1")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient

		cmdArgs := []string{"token", "workspace-token", "add", "--deployment-id", mockDeploymentID, "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "DEPLOYMENT_ADMIN")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "workspace-token", "add", "token-id", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestUpdateWorkspaceTokenDeploymentRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("happy path Update", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		defer testUtil.MockUserInput(t, "1")()

		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "workspace-token", "update", "--deployment-id", mockDeploymentID, "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "DEPLOYMENT_ADMIN")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "workspace-token", "update", "token-id", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestAddOrganizationTokenDeploymentRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Create", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()

		defer testUtil.MockUserInput(t, "1")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient

		cmdArgs := []string{"token", "organization-token", "add", "--deployment-id", mockDeploymentID, "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "DEPLOYMENT_ADMIN")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "organization-token", "add", "token-id", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestUpdateOrganizationTokenDeploymentRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("happy path Update", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		defer testUtil.MockUserInput(t, "1")()

		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "organization-token", "update", "--deployment-id", mockDeploymentID, "--role", "DEPLOYMENT_ADMIN"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "DEPLOYMENT_ADMIN")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "organization-token", "update", "token-id", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestRemoveWorkspaceTokenDeploymentRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		defer testUtil.MockUserInput(t, "1")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "workspace-token", "remove", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateWorkspaceApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "workspace-token", "remove", "token-id", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestRemoveOrganizationTokenDeploymentRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		defer testUtil.MockUserInput(t, "1")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "organization-token", "remove", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("happy path with token id passed in", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "organization-token", "remove", "token-id", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestListOrganizationTokenDeploymentRoles(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "1")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "organization-token", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}

func TestListWorkspaceTokenDeploymentRoles(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path", func(t *testing.T) {
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "1")()
		astroCoreClient = mockClient
		astroCoreIamClient = mockIamClient
		cmdArgs := []string{"token", "workspace-token", "list", "--deployment-id", mockDeploymentID}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
}
