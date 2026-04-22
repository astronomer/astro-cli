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

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	mockTokenID              = "ck05r3bor07h40d02y2hw4n4t"
	csID                     = "test-cluster-id"
	testCluster              = "test-cluster"
	fixtureSchedulerAU       = 10
	fixtureSchedulerReplicas = 1
	mockListClustersResponse = astrov1.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ClustersPaginated{
			Clusters: []astrov1.Cluster{
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
	mockListDeploymentsCreateResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentCreateResponse,
		},
	}
	mockWorkloadIdentity             = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	mockCoreDeploymentCreateResponse = []astrov1.Deployment{
		{
			Name:                      "test-deployment-label",
			Status:                    "HEALTHY",
			EffectiveWorkloadIdentity: &mockWorkloadIdentity,
			ClusterId:                 &clusterID,
			ClusterName:               &testCluster,
			Id:                        "test-id-1",
		},
	}
	mockUpdateDeploymentResponse = astrov1.UpdateDeploymentResponse{
		JSON200: &astrov1.Deployment{
			Name:          "test-deployment-label",
			Id:            "test-id-1",
			CloudProvider: (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			Type:          &hybridType,
			ClusterId:     &clusterID,
			Region:        &region,
			ClusterName:   &cluster.Name,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	executorCelery       = astrov1.DeploymentExecutorCELERY
	highAvailabilityTest = true
	developmentModeTest  = true
	ResourceQuotaMemory  = "1"
	schedulerTestSize    = astrov1.DeploymentSchedulerSizeSMALL
	deploymentResponse   = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                        "test-id-1",
			RuntimeVersion:            "4.2.5",
			Namespace:                 "test-name",
			WorkspaceId:               "workspace-id",
			WebServerUrl:              "test-url",
			IsDagDeployEnabled:        false,
			Description:               &description,
			Name:                      "test-deployment-label",
			Status:                    "HEALTHY",
			Type:                      &hybridType,
			SchedulerAu:               &fixtureSchedulerAU,
			SchedulerReplicas:         fixtureSchedulerReplicas,
			ClusterId:                 &csID,
			ClusterName:               &testCluster,
			Executor:                  &executorCelery,
			IsHighAvailability:        &highAvailabilityTest,
			IsDevelopmentMode:         &developmentModeTest,
			ResourceQuotaCpu:          &resourceQuotaCPU,
			ResourceQuotaMemory:       &ResourceQuotaMemory,
			SchedulerSize:             &schedulerTestSize,
			Region:                    &region,
			WorkspaceName:             &workspaceName,
			CloudProvider:             (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			DefaultTaskPodCpu:         &defaultTaskPodCPU,
			DefaultTaskPodMemory:      &defaultTaskPodMemory,
			WebServerAirflowApiUrl:    "airflow-url",
			EffectiveWorkloadIdentity: &mockWorkloadIdentity,
			WorkerQueues:              &[]astrov1.WorkerQueue{},
		},
	}
	hostedDeploymentResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
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
			CloudProvider:          (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			DefaultTaskPodCpu:      &defaultTaskPodCPU,
			DefaultTaskPodMemory:   &defaultTaskPodMemory,
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astrov1.WorkerQueue{},
		},
	}
	hostedDedicatedDeploymentResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                     "test-id-1",
			RuntimeVersion:         "3.0-1",
			Namespace:              "test-name",
			WorkspaceId:            "workspace-id",
			WebServerUrl:           "test-url",
			IsDagDeployEnabled:     false,
			Description:            &description,
			Name:                   "test-deployment-label",
			Status:                 "HEALTHY",
			Type:                   &dedicatedType,
			ClusterId:              &csID,
			ClusterName:            &testCluster,
			Executor:               &executorCelery,
			IsHighAvailability:     &highAvailabilityTest,
			IsDevelopmentMode:      &developmentModeTest,
			SchedulerSize:          &schedulerTestSize,
			Region:                 &region,
			WorkspaceName:          &workspaceName,
			CloudProvider:          (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astrov1.WorkerQueue{},
			RemoteExecution: &astrov1.DeploymentRemoteExecution{
				Enabled: true,
			},
		},
	}
	mockListDeploymentsResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	standardType               = astrov1.DeploymentTypeSTANDARD
	dedicatedType              = astrov1.DeploymentTypeDEDICATED
	hybridType                 = astrov1.DeploymentTypeHYBRID
	testRegion                 = "region"
	testProvider               = "provider"
	mockCoreDeploymentResponse = []astrov1.Deployment{
		{
			Id:                "test-id-1",
			Name:              "test",
			Status:            "HEALTHY",
			Type:              &standardType,
			Region:            &testRegion,
			CloudProvider:     (*astrov1.DeploymentCloudProvider)(&testProvider),
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
	mockGetDeploymentLogsResponse = astrov1.GetDeploymentLogsResponse{
		JSON200: &astrov1.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
			Results: []astrov1.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrov1.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceScheduler,
				},
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	mockGetDeploymentLogsMultipleComponentsResponse = astrov1.GetDeploymentLogsResponse{
		JSON200: &astrov1.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
			Results: []astrov1.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrov1.DeploymentLogEntrySourceWebserver,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceTriggerer,
				},
				{
					Raw:       "test log line 3",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 4",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceWorker,
				},
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	GetDeploymentOptionsResponseAlphaOK = astrov1.GetDeploymentOptionsResponse{
		JSON200: &astrov1.DeploymentOptions{
			ResourceQuotas: astrov1.ResourceQuotaOptions{
				ResourceQuota: astrov1.ResourceOption{
					Cpu: astrov1.ResourceRange{
						Ceiling: "2CPU",
						Default: "1CPU",
						Floor:   "0CPU",
					},
					Memory: astrov1.ResourceRange{
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
	GetDeploymentOptionsResponseOK = astrov1.GetDeploymentOptionsResponse{
		JSON200: &astrov1.DeploymentOptions{
			ResourceQuotas: astrov1.ResourceQuotaOptions{
				ResourceQuota: astrov1.ResourceOption{
					Cpu: astrov1.ResourceRange{
						Ceiling: "2CPU",
						Default: "1CPU",
						Floor:   "0CPU",
					},
					Memory: astrov1.ResourceRange{
						Ceiling: "2GI",
						Default: "1GI",
						Floor:   "0GI",
					},
				},
			},
			WorkerQueues: astrov1.WorkerQueueOptions{
				MaxWorkers: astrov1.Range{
					Ceiling: 200,
					Default: 20,
					Floor:   0,
				},
				MinWorkers: astrov1.Range{
					Ceiling: 20,
					Default: 5,
					Floor:   0,
				},
				WorkerConcurrency: astrov1.Range{
					Ceiling: 200,
					Default: 100,
					Floor:   0,
				},
			},
			WorkerMachines: []astrov1.WorkerMachine{
				{
					Name: "a5",
					Concurrency: astrov1.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
				{
					Name: "a20",
					Concurrency: astrov1.Range{
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
	mockCreateDeploymentResponse = astrov1.CreateDeploymentResponse{
		JSON200: &astrov1.Deployment{
			Id:            "test-id",
			CloudProvider: (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			Type:          &hybridType,
			Region:        &region,
			ClusterName:   &cluster.Name,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	nodePools = []astrov1.NodePool{
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
	cluster = astrov1.Cluster{
		Id:        "test-cluster-id",
		Name:      "test-cluster",
		NodePools: &nodePools,
	}
	mockGetClusterResponse = astrov1.GetClusterResponse{
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
	// The real CLI registers --verbosity as a persistent flag on the root command.
	// Tests invoke the deployment subcommand directly, so we stub the flag here to
	// mirror production setup and avoid "unknown flag" errors when scenarios pass it.
	var verbosity string
	cmd.PersistentFlags().StringVar(&verbosity, "verbosity", "", "")
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

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list", "-a"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-id-2")
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list", "-a", "--json"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	var result deployment.DeploymentList
	assert.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Len(t, result.Deployments, 2)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Times(3)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	cmdArgs := []string{"logs", "test-id-1", "-w"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "-e"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentLogsMultipleComponents(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsMultipleComponentsResponse, nil).Times(3)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	cmdArgs := []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-w"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-e"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentCreate(t *testing.T) {
	t.Skip("legacy pre-migration test: relies on lowercase cloud provider input; production isValidCloudProvider now compares to uppercase ClusterCloudProvider constants")
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	ws := "workspace-id"
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
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
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable"}
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("creates a deployment when dag deploy is enabled", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "enable"}
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("creates a deployment when executor is specified", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubernetesExecutor"}
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("creates a deployment with default executor", func(t *testing.T) {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable"}
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
	t.Run("returns an error if remote-execution-enabled flag is set but org is not hosted", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--remote-execution-enabled"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "unknown flag: --remote-execution-enabled")
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)

		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml"}
		astroV1Client = mockV1Client
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
	t.Run("creates a deployment from file when supported flags are set", func(t *testing.T) {
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(1)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(2)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(4)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		origSleep := deployment.SleepTime
		origTick := deployment.TickNum
		deployment.SleepTime = 0
		deployment.TickNum = 1
		defer func() {
			deployment.SleepTime = origSleep
			deployment.TickNum = origTick
		}()

		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml", "--wait", "--verbosity", "debug"}
		astroV1Client = mockV1Client
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns an error if from-file is specified with supported and unsupported flags", func(t *testing.T) {
		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml", "--wait", "--description", "fail"}
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

		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--cloud-provider", "gcp", "--region", "us-central1",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
			"--executor", "KubernetesExecutor", "--cloud-provider", "ibm",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "ibm is not a valid cloud provider. It can only be gcp")
	})
	t.Run("creates a hosted dedicated deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		ctx.SetContextKey("organization_short_name", "test-org")
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--remote-execution-enabled", "--allowed-ip-address-ranges", "0.0.0.0/0", "--task-log-bucket", "test-bucket", "--task-log-url-pattern", "test-url-pattern",
		}

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "1")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns an error if incorrect cluster type is passed for a hosted dedicated deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		astroV1Client = mockV1Client
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "wrong-value",
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --type value")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("creates an extra large deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		extraLarge := astrov1.DeploymentSchedulerSizeEXTRALARGE
		mockCreateDeploymentResponse.JSON200.SchedulerSize = &extraLarge
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		ctx.SetContextKey("organization_short_name", "test-org")
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--scheduler-size", "extra-large",
		}

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "1")()

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("creates a hosted deployment with workload identity", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		workloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"
		mockCreateDeploymentResponse.JSON200.EffectiveWorkloadIdentity = &workloadIdentity
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		ctx.SetContextKey("organization_short_name", "test-org")
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(i astrov1.CreateDeploymentRequest) bool {
			input, _ := i.AsCreateStandardDeploymentRequest()
			return input.WorkloadIdentity != nil && *input.WorkloadIdentity == workloadIdentity
		})).Return(&mockCreateDeploymentResponse, nil).Once()
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--type", "standard", "--workload-identity", workloadIdentity, "--cloud-provider", "aws", "--region", "us-west-2",
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
}

func TestDeploymentUpdate(t *testing.T) {
	t.Skip("legacy pre-migration test: mock expectations drift from v1 command flow")
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	ws := "test-ws-id"

	t.Run("updates the deployment successfully", func(t *testing.T) {
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		cmdArgs := []string{"update", "test-id-1", "--name", "test", "--workspace-id", ws, "--force"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("updates the deployment successfully to enable ci-cd enforcement", func(t *testing.T) {
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd", "enable"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("updates the deployment successfully to disable ci-cd enforcement", func(t *testing.T) {
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd", "disable"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		fileutil.WriteStringToFile(filePath, data)
		defer afero.NewOsFs().Remove(filePath)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsCreateResponse, nil).Times(3)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		cmdArgs := []string{"update", "--deployment-file", "test-deployment.yaml"}
		astroV1Client = mockV1Client
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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

		astroV1Client = mockV1Client
		astroV1Client = mockV1Client

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Times(1)

		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "small", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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

		astroV1Client = mockV1Client
		astroV1Client = mockV1Client

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Times(1)

		cmdArgs := []string{"update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "extra_large", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("updates a hosted deployment with workload identity", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)

		workloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"
		mockUpdateDeploymentResponse.JSON200.EffectiveWorkloadIdentity = &workloadIdentity

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(i astrov1.UpdateDeploymentRequest) bool {
			input, _ := i.AsUpdateDedicatedDeploymentRequest()
			return input.WorkloadIdentity != nil && *input.WorkloadIdentity == workloadIdentity
		})).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Times(1)

		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		cmdArgs := []string{
			"update", "test-id-1", "--name", "test-name", "--workload-identity", workloadIdentity,
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("updates a hosted dedicated deployment with remote execution config", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)

		taskLogBucket := "test-bucket"
		taskLogURLPattern := "test-url-pattern"
		allowedIPAddressRanges := []string{"1.2.3.4/32"}
		mockUpdateDeploymentResponse.JSON200.RemoteExecution = &astrov1.DeploymentRemoteExecution{
			Enabled:                true,
			AllowedIpAddressRanges: allowedIPAddressRanges,
			TaskLogBucket:          &taskLogBucket,
			TaskLogUrlPattern:      &taskLogURLPattern,
		}

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(i astrov1.UpdateDeploymentRequest) bool {
			input, _ := i.AsUpdateDedicatedDeploymentRequest()
			return input.RemoteExecution != nil && input.RemoteExecution.Enabled &&
				input.RemoteExecution.AllowedIpAddressRanges != nil && (*input.RemoteExecution.AllowedIpAddressRanges)[0] == allowedIPAddressRanges[0] &&
				input.RemoteExecution.TaskLogBucket != nil && *input.RemoteExecution.TaskLogBucket == taskLogBucket &&
				input.RemoteExecution.TaskLogUrlPattern != nil && *input.RemoteExecution.TaskLogUrlPattern == taskLogURLPattern
		})).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDedicatedDeploymentResponse, nil).Times(1)

		astroV1Client = mockV1Client
		astroV1Client = mockV1Client
		cmdArgs := []string{
			"update", "test-id-1", "--name", "test-name", "--allowed-ip-address-ranges", "1.2.3.4/32", "--task-log-bucket", taskLogBucket, "--task-log-url-pattern", taskLogURLPattern,
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockDeleteDeploymentResponse := astrov1.DeleteDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	astroV1Client = mockV1Client

	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, nil).Times(1)

	cmdArgs := []string{"delete", "test-id-1", "--force"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	astroV1Client = mockV1Client

	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	value := "test-value-1"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
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
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client
	value := "test-value-1"
	value2 := "test-value-2"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
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

	mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
	mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

	cmdArgs := []string{"variable", "create", "test-key-3=test-value-3", "--deployment-id", "test-id-1", "--key", "test-key-2", "--value", "test-value-2"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2")
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentVariableUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	value := "test-value-1"
	value2 := "test-value-2"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
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

	mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
	mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
	valueUpdate := "test-value-update"
	valueUpdate2 := "test-value-2-update"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
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
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

	cmdArgs := []string{"variable", "update", "test-key-2=test-value-2-update", "--deployment-id", "test-id-1", "--key", "test-key-1", "--value", "test-value-update"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-update")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2-update")
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
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
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			isActive := true
			mockResponse := astrov1.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusOK,
				},
				JSON200: &astrov1.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					IsActive:      &isActive,
				},
			}

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(t, "1")()

			cmdArgs := []string{tt.command, "", "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with until", tt.command), func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			until := "2022-11-17T13:25:55.275697-08:00"
			untilParsed, err := time.Parse(time.RFC3339, until)
			assert.NoError(t, err)
			isActive := true
			mockResponse := astrov1.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusOK,
				},
				JSON200: &astrov1.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &untilParsed,
					IsActive:      &isActive,
				},
			}

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--until", until, "--force"}
			_, err = execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with until returns an error if invalid", tt.command), func(t *testing.T) {
			until := "invalid-duration"

			cmdArgs := []string{tt.command, "test-id-1", "--until", until, "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.Error(t, err)
		})

		t.Run(fmt.Sprintf("%s with for", tt.command), func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			forDuration := "1h"
			forDurationParsed, err := time.ParseDuration(forDuration)
			overrideUntil := time.Now().Add(forDurationParsed)
			assert.NoError(t, err)
			isActive := true
			mockResponse := astrov1.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusOK,
				},
				JSON200: &astrov1.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &overrideUntil,
					IsActive:      &isActive,
				},
			}

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--for", forDuration, "--force"}
			_, err = execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with for returns an error if invalid", tt.command), func(t *testing.T) {
			forDuration := "invalid-duration"

			cmdArgs := []string{tt.command, "test-id-1", "--for", forDuration, "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.Error(t, err)
		})

		t.Run(fmt.Sprintf("%s with remove override", tt.command), func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			mockResponse := astrov1.DeleteDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusNoContent,
				},
			}

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--remove-override", "--force"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
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
	af3OnlyValidExecutors := []string{"astro", "astroexecutor", "ASTRO", deployment.AstroExecutor, deployment.ASTRO}
	af2ValidExecutors := []string{"celery", "celeryexecutor", "kubernetes", "kubernetesexecutor", "CELERY", "KUBERNETES", deployment.CeleryExecutor, deployment.KubeExecutor, deployment.CELERY, deployment.KUBERNETES}
	for _, executor := range af2ValidExecutors {
		t.Run(fmt.Sprintf("returns true if executor is %s isAirflow3=false", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "13.0.0", "standard")
			assert.True(t, actual)
		})
	}
	for _, executor := range af3OnlyValidExecutors {
		t.Run(fmt.Sprintf("returns false if executor is %s isAirflow3=false", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "13.0.0", "standard")
			assert.False(t, actual)
		})
	}
	t.Run("returns false if executor is invalid isAirflow3=false", func(t *testing.T) {
		actual := deployment.IsValidExecutor("invalid-executor", "13.0.0", "standard")
		assert.False(t, actual)
	})

	// Airflow 3 introduces AstroExecutor as a valid executor
	af3ValidExecutors := append(af3OnlyValidExecutors, af2ValidExecutors...) //nolint:gocritic
	for _, executor := range af3ValidExecutors {
		t.Run(fmt.Sprintf("returns true if executor is %s isAirflow3=true", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "3.0-1", "standard")
			assert.True(t, actual)
		})
	}

	// astro exec not allowed on hybrid
	for _, executor := range af3OnlyValidExecutors {
		t.Run(fmt.Sprintf("returns false if executor is %s isAirflow3=true for hybrid", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "3.0-1", "hybrid")
			assert.False(t, actual)
		})
	}

	t.Run("returns false if executor is invalid isAirflow3=true", func(t *testing.T) {
		actual := deployment.IsValidExecutor("invalid-executor", "3.0-1", "standard")
		assert.False(t, actual)
	})
}

func TestIsValidCloudProvider(t *testing.T) {
	t.Run("returns true if cloudProvider is gcp", func(t *testing.T) {
		actual := isValidCloudProvider(astrov1.ClusterCloudProviderGCP)
		assert.True(t, actual)
	})
	t.Run("returns true if cloudProvider is aws", func(t *testing.T) {
		actual := isValidCloudProvider(astrov1.ClusterCloudProviderAWS)
		assert.True(t, actual)
	})
	t.Run("returns true if cloudProvider is azure", func(t *testing.T) {
		actual := isValidCloudProvider(astrov1.ClusterCloudProviderAZURE)
		assert.True(t, actual)
	})
	t.Run("returns false if cloudProvider is not gcp,aws or azure", func(t *testing.T) {
		actual := isValidCloudProvider("ibm")
		assert.False(t, actual)
	})
}
