package cloud

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"

	"github.com/astronomer/astro-cli/cloud/deployment"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	clusterID        string
	cloudProvider    string
	region           string
	schedulerAU      int
	resourceQuotaCPU string
	workspaceName    string
	defaultTaskPodCPU    string
	defaultTaskPodMemory string

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
			WorkloadIdentity:       &mockWorkloadIdentity,
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
	hostedDedicatedDeploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
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
			CloudProvider:          (*astroplatformcore.DeploymentCloudProvider)(&cloudProvider),
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astroplatformcore.WorkerQueue{},
			RemoteExecution: &astroplatformcore.DeploymentRemoteExecution{
				Enabled: true,
			},
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

	af3ValidExecutors := append(af3OnlyValidExecutors, af2ValidExecutors...) //nolint:gocritic
	for _, executor := range af3ValidExecutors {
		t.Run(fmt.Sprintf("returns true if executor is %s isAirflow3=true", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "3.0-1", "standard")
			assert.True(t, actual)
		})
	}

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
