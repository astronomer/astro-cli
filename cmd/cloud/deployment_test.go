package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
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
	workloadIdentity                 = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	mockCoreDeploymentCreateResponse = []astroplatformcore.Deployment{
		{
			Name:             "test-deployment-label",
			Status:           "HEALTHY",
			WorkloadIdentity: &workloadIdentity,
			ClusterId:        &clusterID,
			ClusterName:      &testCluster,
			Id:               "deployment-id",
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
	executorCelery       = astroplatformcore.DeploymentExecutorCELERY
	executorKubernetes   = astroplatformcore.DeploymentExecutorKUBERNETES
	highAvailabilityTest = true
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
			DagDeployEnabled:       false,
			Name:                   "test",
			Status:                 "HEALTHY",
			Type:                   &hybridType,
			SchedulerAu:            &schedulerAU,
			ClusterId:              &csID,
			ClusterName:            &testCluster,
			Executor:               &executorCelery,
			IsHighAvailability:     &highAvailabilityTest,
			ResourceQuotaCpu:       &resourceQuotaCpu,
			ResourceQuotaMemory:    &ResourceQuotaMemory,
			SchedulerSize:          &schedulerTestSize,
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
	dedicatedType              = astroplatformcore.DeploymentTypeDEDICATED
	hybridType                 = astroplatformcore.DeploymentTypeHYBRID
	testRegion                 = "region"
	testProvider               = "provider"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:            "test-id-1",
			Name:          "test",
			Status:        "HEALTHY",
			Type:          &standardType,
			Region:        &testRegion,
			CloudProvider: &testProvider,
		},
		{
			Id:          "test-id-2",
			Name:        "test-2",
			Status:      "HEALTHY",
			Type:        &hybridType,
			ClusterName: &testCluster,
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
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newDeploymentRootCmd(os.Stdout)
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "deployment")
}

func TestDeploymentList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Times(3)
	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient

	cmdArgs := []string{"logs", "test-id-1", "-w", "", ""}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "", "-e", ""}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "", "", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockPlatformCoreClient.AssertExpectations(t)
	mockCoreClient.AssertExpectations(t)
}

func TestDeploymentCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ws := "workspace-id"
	csID := "test-cluster-id"
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
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
			"create", "--name", "test-name", "--workspace-id", ws, "--cluster-type", "dedicated",
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
			"create", "--name", "test-name", "--workspace-id", ws, "--cluster-type", "wrong-value",
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --cluster-type value")
		mockCoreClient.AssertExpectations(t)
	})
}

func TestDeploymentUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd", "some-value"}
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
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Workspace = ""
		err = ctx.SetContext()
		assert.NoError(t, err)
		defer testUtil.InitTestConfig(testUtil.CloudPlatform)
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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

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
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

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
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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

func TestIsValidExecutor(t *testing.T) {
	t.Run("returns true for Kubernetes Executor", func(t *testing.T) {
		actual := isValidExecutor(deployment.KubeExecutor)
		assert.True(t, actual)
	})
	t.Run("returns true for Celery Executor", func(t *testing.T) {
		actual := isValidExecutor(deployment.CeleryExecutor)
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
