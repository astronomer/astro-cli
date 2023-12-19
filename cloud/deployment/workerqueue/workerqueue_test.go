package workerqueue

import (
	"bytes"
	"errors"
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
	errGetDeployment           = errors.New("test get deployment error")
	errUpdateDeployment        = errors.New("test deployment update error")
	errDeploymentConfigOptions = errors.New("test get deployment error")
)

var (
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient         = new(astrocore_mocks.ClientWithResponsesInterface)
	testPoolID             = "test-pool-id"
	testPoolID1            = "test-pool-id-1"
	standardType           = astroplatformcore.DeploymentTypeSTANDARD
	dedicatedType          = astroplatformcore.DeploymentTypeDEDICATED
	hybridType             = astroplatformcore.DeploymentTypeHYBRID
	schedulerAU            = 0
	clusterID              = "cluster-id"
	executorCelery         = astroplatformcore.DeploymentExecutorCELERY
	executorKubernetes     = astroplatformcore.DeploymentExecutorKUBERNETES
	highAvailability       = true
	resourceQuotaCpu       = "1cpu"
	ResourceQuotaMemory    = "1"
	schedulerSize          = astroplatformcore.DeploymentSchedulerSizeSMALL
	deploymentResponse     = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                  "test-deployment-id",
			RuntimeVersion:      "4.2.5",
			Namespace:           "test-deployment-label",
			WorkspaceId:         "workspace-id",
			WebServerUrl:        "test-url",
			DagDeployEnabled:    false,
			Name:                "test-deployment-label",
			Status:              "HEALTHY",
			Type:                &hybridType,
			SchedulerAu:         &schedulerAU,
			ClusterId:           &clusterID,
			Executor:            &executorCelery,
			IsHighAvailability:  &highAvailability,
			ResourceQuotaCpu:    &resourceQuotaCpu,
			ResourceQuotaMemory: &ResourceQuotaMemory,
			SchedulerSize:       &schedulerSize,
			WorkerQueues:        &[]astroplatformcore.WorkerQueue{},
		},
	}
	nodePools = []astroplatformcore.NodePool{
		{
			Id:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "test-instance-type",
		},
		{
			Id:               "test-pool-id-1",
			IsDefault:        false,
			NodeInstanceType: "test-instance-type-1",
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
	mockListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	testRegion                 = "region"
	testProvider               = "provider"
	testCluster                = "cluster"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:            "test-deployment-id",
			Name:          "test-deployment-label",
			Status:        "HEALTHY",
			Type:          &standardType,
			Region:        &testRegion,
			CloudProvider: &testProvider,
		},
		{
			Id:          "test-id-2",
			Name:        "test",
			Status:      "HEALTHY",
			Type:        &hybridType,
			ClusterName: &testCluster,
		},
	}
	cloudProvider                = "test-provider"
	mockUpdateDeploymentResponse = astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Id:            "test-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	GetDeploymentOptionsResponseOK = astrocore.GetDeploymentOptionsResponse{
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
	GetDeploymentOptionsPlatformResponseOK = astroplatformcore.GetDeploymentOptionsResponse{
		JSON200: &astroplatformcore.DeploymentOptions{
			ResourceQuotas: astroplatformcore.ResourceQuotaOptions{},
			Executors:      []string{},
			WorkerQueues: astroplatformcore.WorkerQueueOptions{
				MaxWorkers: astroplatformcore.Range{
					Floor:   20,
					Ceiling: 200,
					Default: 125,
				},
				MinWorkers: astroplatformcore.Range{
					Floor:   0,
					Ceiling: 20,
					Default: 5,
				},
				WorkerConcurrency: astroplatformcore.Range{
					Floor:   175,
					Ceiling: 275,
					Default: 180,
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
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	emptyListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{},
		},
	}
)

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedWorkerQueue := astroplatformcore.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		defer testUtil.MockUserInput(t, "test-worker-queue")()
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "test-instance-type-1", -1, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "test-invalid-deployment-id")()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a node pool fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue would update an existing queue with the same name", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "test-queue-1", NodePoolId: &testPoolID}}
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-1", createAction, "test-instance-type-1", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errCannotUpdateExistingQueue)
		assert.ErrorContains(t, err, "worker queue already exists: use worker queue update test-queue-1 instead")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when update deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "y")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateDeployment).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	// CE executor
	deploymentResponse.JSON200.Executor = &executorCelery
	t.Run("happy path creates a new worker queue for a deployment when no worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "y")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "test-instance-type-1", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("happy path creates a new worker queue for a deployment when worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "2")()
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "test-queue-1", NodePoolId: &testPoolID}}
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "", -1, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, errWorkerQueueDefaultOptions).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue input is not valid", func(t *testing.T) {
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{}
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	// kube executor
	deploymentResponse.JSON200.Executor = &executorKubernetes
	t.Run("prompts user for a name if one was not provided", func(t *testing.T) {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "bigQ")()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)

		assert.ErrorIs(t, err, ErrNotSupported)
		assert.ErrorContains(t, err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested input is not valid", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.ErrorContains(t, err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns a queue already exists error when request is to create a new queue", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "default", createAction, "test-instance-type-1", -1, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errCannotUpdateExistingQueue)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestCreateHostedShared(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedOutMessage := "worker queue test-worker-queue for test-deployment-label in test-ws-id workspace created\n"
	out := new(bytes.Buffer)
	// hosted deployments
	deploymentResponse.JSON200.Executor = &executorCelery
	deploymentResponse.JSON200.Type = &standardType
	deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{}
	t.Run("for hosted shared deployments", func(t *testing.T) {
		deploymentResponse.JSON200.Type = &standardType
		defer testUtil.MockUserInput(t, "test-worker-queue")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "a5", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("select machine for hosted shared deployments", func(t *testing.T) {
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-worker-queue", createAction, "", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)

	})
	t.Run("failed to select astro machines for hosted shared deployments", func(t *testing.T) {
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "4")()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-worker-queue", createAction, "", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidAstroMachine)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)

	})
	t.Run("failed to get deployment config options for hosted shared deployments", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "test-worker-queue")()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, errDeploymentConfigOptions).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "a5", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errDeploymentConfigOptions)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	expectedWorkerQueue := astroplatformcore.WorkerQueue{
		Name:              "test-queue-1",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}
	t.Run("updates the queue if user replies yes", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		deploymentResponse.JSON200.Type = &hybridType

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorContains(t, err, "use worker queue create test-queue-1 instead")

		deploymentResponse.JSON200.Type = &standardType

		err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "a5", -1, 20, 5, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorContains(t, err, "use worker queue create test-queue-1 instead")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("cancels update if user does not confirm", func(t *testing.T) {
		expectedOutMessage := "Canceling worker queue update\n"
		deploymentResponse.JSON200.Type = &hybridType
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type-1", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
		defer testUtil.MockUserInput(t, "1")()
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", -1, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "test-invalid-deployment-id")()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, deployment.ErrInvalidDeploymentKey).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a node pool fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		// mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if user makes incorrect choice when selecting a queue to update", func(t *testing.T) {
		expectedOutMessage := "invalid worker queue: 4 selected"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "4")()
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", 0, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.Contains(t, err.Error(), expectedOutMessage)
		mockPlatformCoreClient.AssertExpectations(t)

	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when updating requested queue would create a new queue", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-2", updateAction, "test-instance-type-1", 0, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errCannotCreateNewQueue)
		assert.ErrorContains(t, err, "worker queue does not exist: use worker queue create test-queue-2 instead")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when update deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateDeployment).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	deploymentResponse.JSON200.Executor = &executorCelery
	t.Run("happy path update existing worker queue for a deployment Celery", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, errWorkerQueueDefaultOptions).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue input is not valid", func(t *testing.T) {
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{}
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)

		deploymentResponse.JSON200.Type = &dedicatedType

		err = CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "a5", 25, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	// kube executor
	deploymentResponse.JSON200.Executor = &executorKubernetes
	t.Run("happy path update existing worker queue for a deployment", func(t *testing.T) {

		expectedOutMessage := "worker queue default for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		// mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}
		deploymentResponse.JSON200.Type = &hybridType
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type", -1, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("update existing worker queue with a new worker type", func(t *testing.T) {
		expectedOutMessage := "worker queue default for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type-1", -1, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested input is not valid", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", updateAction, "test-instance-type-1", -1, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.ErrorContains(t, err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	testID := "test-wq-id"
	astroMachine := "A5"
	workerQueueList := []astroplatformcore.WorkerQueue{
		{
			Id:                testID,
			Name:              "default",
			IsDefault:         true,
			MaxWorkerCount:    130,
			MinWorkerCount:    12,
			WorkerConcurrency: 110,
			NodePoolId:        &testPoolID,
			PodCpu:            "huge",
			PodMemory:         "lots",
			AstroMachine:      &astroMachine,
		},
		{
			Id:                "test-wq-id-1",
			Name:              "test-worker-queue-1",
			IsDefault:         false,
			MaxWorkerCount:    175,
			MinWorkerCount:    8,
			WorkerConcurrency: 150,
			NodePoolId:        &testPoolID1,
			AstroMachine:      &astroMachine,
		},
	}
	deploymentResponse.JSON200.WorkerQueues = &workerQueueList
	expectedOutMessage := "worker queue test-worker-queue-1 for test-deployment-label in test-ws-id workspace deleted\n"
	t.Run("happy path worker queue gets deleted", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())

		out = new(bytes.Buffer)
		// starndard type
		deploymentResponse.JSON200.Type = &standardType
		err = Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())

		out = new(bytes.Buffer)
		// dedicated type
		deploymentResponse.JSON200.Type = &dedicatedType
		err = Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())

		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		defer testUtil.MockUserInput(t, "2")()

		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)

		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prompts user for confirmation if --force was not provided", func(t *testing.T) {
		t.Run("deletes the queue if user replies yes", func(t *testing.T) {
			defer testUtil.MockUserInput(t, "y")()
			out := new(bytes.Buffer)
			mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
			mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
			mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockPlatformCoreClient, mockCoreClient, out)
			assert.NoError(t, err)
			mockCoreClient.AssertExpectations(t)
			mockPlatformCoreClient.AssertExpectations(t)
		})
		t.Run("cancels deletion if user does not confirm", func(t *testing.T) {
			defer testUtil.MockUserInput(t, "n")()
			expectedOutMessage = "Canceling worker queue deletion\n"
			out := new(bytes.Buffer)
			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockPlatformCoreClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockCoreClient.AssertExpectations(t)
			mockPlatformCoreClient.AssertExpectations(t)
		})
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		out := new(bytes.Buffer)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Times(1)
		// mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an errors if queue selection fails", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "3")()

		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.NotContains(t, out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if user chooses to delete default queue", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Delete("test-ws-id", "test-deployment-id", "", "default", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errCannotDeleteDefaultQueue)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if trying to delete a queue that does not exist", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Delete("test-ws-id", "test-deployment-id", "", "test-non-existent-queue", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errQueueDoesNotExist)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if deployment update fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateDeployment).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("when no deployments exists in the workspace", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Times(1)
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestSetWorkerQueueValues(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astroplatformcore.WorkerQueueOptions{
		MaxWorkers: astroplatformcore.Range{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		MinWorkers: astroplatformcore.Range{
			Floor:   1,
			Ceiling: 20,
			Default: 5,
		},
		WorkerConcurrency: astroplatformcore.Range{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	mockWorkerQueue := &astroplatformcore.WorkerQueueRequest{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}
	t.Run("sets user provided min worker count for queue", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(0, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.MinWorkerCount, actualQueue.MinWorkerCount)
	})
	t.Run("sets user provided max worker count for queue", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.MaxWorkerCount, actualQueue.MaxWorkerCount)
	})
	t.Run("sets user provided worker concurrency for queue", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.WorkerConcurrency, actualQueue.WorkerConcurrency)
	})
	t.Run("sets default min worker count for queue if user did not provide it", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(-1, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, int(mockWorkerQueueDefaultOptions.MinWorkers.Default), actualQueue.MinWorkerCount)
	})
	t.Run("sets default max worker count for queue if user did not provide it", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 0, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, int(mockWorkerQueueDefaultOptions.MaxWorkers.Default), actualQueue.MaxWorkerCount)
	})
	t.Run("sets default worker concurrency for queue if user did not provide it", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 150, 0, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, int(mockWorkerQueueDefaultOptions.WorkerConcurrency.Default), actualQueue.WorkerConcurrency)
	})
}

func TestIsCeleryWorkerQueueInputValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astroplatformcore.WorkerQueueOptions{
		MaxWorkers: astroplatformcore.Range{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		MinWorkers: astroplatformcore.Range{
			Floor:   0,
			Ceiling: 20,
			Default: 5,
		},
		WorkerConcurrency: astroplatformcore.Range{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	requestedWorkerQueue := &astroplatformcore.HybridWorkerQueueRequest{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolId:        "",
	}

	t.Run("happy path when min or max worker count and worker concurrency are within default floor and ceiling", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.NoError(t, err)
	})
	t.Run("returns an error when min worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	t.Run("returns an error when max worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	t.Run("returns an error when worker concurrency is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 350
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: worker concurrency must be between 175 and 275")
	})
}

func TestIsHostedCeleryWorkerQueueInputValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astroplatformcore.WorkerQueueOptions{
		MinWorkers: astroplatformcore.Range{
			Floor:   0,
			Ceiling: 20,
			Default: 5,
		},
		MaxWorkers: astroplatformcore.Range{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		WorkerConcurrency: astroplatformcore.Range{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}

	mockMachineOptions := &astroplatformcore.WorkerMachine{
		Name: "a5",
		Concurrency: astroplatformcore.Range{
			Default: 5,
			Ceiling: 15,
		},
	}
	requestedWorkerQueue := &astroplatformcore.WorkerQueueRequest{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}

	t.Run("happy path when min or max worker count and worker concurrency are within default floor and ceiling", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 10
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.NoError(t, err)
	})
	t.Run("returns an error when min worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	t.Run("returns an error when max worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	t.Run("returns an error when worker concurrency is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 20
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: worker concurrency must be between 1 and 15")
	})
}

func TestIsKubernetesWorkerQueueInputValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	requestedWorkerQueue := &astroplatformcore.HybridWorkerQueueRequest{
		Name:              "default",
		MinWorkerCount:    -1,
		MaxWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolId:        "test-pool-id",
	}

	t.Run("returns nil when queue input is valid", func(t *testing.T) {
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.NoError(t, err)
	})
	t.Run("returns an error when queue name is not default", func(t *testing.T) {
		requestedWorkerQueue.Name = "test-queue"
		defer func() {
			requestedWorkerQueue = &astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
	})
	t.Run("returns an error when min_worker_count is in input", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		defer func() {
			requestedWorkerQueue = &astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support minimum worker count in the request. It can only be used with CeleryExecutor")
	})
	t.Run("returns an error when max_worker_count is in input", func(t *testing.T) {
		requestedWorkerQueue.MaxWorkerCount = 25
		defer func() {
			requestedWorkerQueue = &astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support maximum worker count in the request. It can only be used with CeleryExecutor")
	})
	t.Run("returns an error when worker_concurrency is in input", func(t *testing.T) {
		requestedWorkerQueue.WorkerConcurrency = 350
		defer func() {
			requestedWorkerQueue = &astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support worker concurrency in the request. It can only be used with CeleryExecutor")
	})
}

func TestQueueExists(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	testID := "test-nodepool-id"
	testID1 := "test-nodepool-id-1"
	testWQID := "test-wq-id-1"
	testWQID10 := "test-wq-id-10"
	existingQueues := []astroplatformcore.WorkerQueue{
		{
			Id:                "test-wq-id",
			Name:              "test-default-queue",
			IsDefault:         true,
			MaxWorkerCount:    130,
			MinWorkerCount:    12,
			WorkerConcurrency: 110,
			NodePoolId:        &testID,
		},
		{
			Id:                testWQID,
			Name:              "test-queue-1",
			IsDefault:         false,
			MaxWorkerCount:    175,
			MinWorkerCount:    8,
			WorkerConcurrency: 150,
			NodePoolId:        &testID1,
		},
	}
	t.Run("returns true if queue with same name exists in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astroplatformcore.WorkerQueueRequest{Name: "test-default-queue"}, &astroplatformcore.HybridWorkerQueueRequest{Name: "test-default-queue"})
		assert.True(t, actual)
	})
	t.Run("returns true if queue with same id exists in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astroplatformcore.WorkerQueueRequest{Id: &testWQID}, &astroplatformcore.HybridWorkerQueueRequest{Id: &testWQID})
		fmt.Println(actual)
		assert.True(t, actual)
	})
	t.Run("returns false if queue with same name does not exist in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astroplatformcore.WorkerQueueRequest{Name: "test-default-queues"}, &astroplatformcore.HybridWorkerQueueRequest{Name: "test-default-queues"})
		assert.False(t, actual)
	})
	t.Run("returns false if queue with same id exists in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astroplatformcore.WorkerQueueRequest{Id: &testWQID10}, &astroplatformcore.HybridWorkerQueueRequest{Id: &testWQID10})
		assert.False(t, actual)
	})
}

func TestSelectNodePool(t *testing.T) {
	var (
		workerType, nodePoolID string
		poolList               []astroplatformcore.NodePool
		out                    *bytes.Buffer
	)

	out = new(bytes.Buffer)
	poolList = []astroplatformcore.NodePool{
		{
			Id:               "test-default-pool",
			IsDefault:        true,
			NodeInstanceType: "test-instance",
		},
		{
			Id:               "test-non-default-pool",
			IsDefault:        false,
			NodeInstanceType: "test-instance-1",
		},
		{
			Id:               "test-non-default-pool-1",
			IsDefault:        false,
			NodeInstanceType: "test-instance-2",
		},
	}
	t.Run("prompts user to pick a node pool if worker type was not requested", func(t *testing.T) {
		workerType = ""

		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.NoError(t, err)
		assert.Equal(t, poolList[1].Id, nodePoolID)
	})
	t.Run("returns the pool that matches worker type that the user requested", func(t *testing.T) {
		var err error
		workerType = "test-instance-2"
		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.NoError(t, err)
		assert.Equal(t, poolList[2].Id, nodePoolID)
	})
	t.Run("returns an error when user chooses a value not in the list", func(t *testing.T) {
		workerType = ""

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
	})
	t.Run("returns an error when user chooses a workerType that does not exist in any node pools", func(t *testing.T) {
		workerType = "non-existent"

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
	})
}

func TestSelectQueue(t *testing.T) {
	var (
		out           *bytes.Buffer
		queueList     []astroplatformcore.WorkerQueue
		queueToDelete string
		err           error
	)
	out = new(bytes.Buffer)
	queueList = []astroplatformcore.WorkerQueue{
		{
			Id:        "queue-1",
			Name:      "default",
			IsDefault: true,
		},
		{
			Id:        "queue-2",
			Name:      "my-queue-2",
			IsDefault: false,
		},
		{
			Id:        "queue-3",
			Name:      "my-queue-3",
			IsDefault: false,
		},
	}

	t.Run("user can select a queue to delete", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "2")()
		queueToDelete, err = selectQueue(&queueList, out)
		assert.NoError(t, err)
		assert.Equal(t, "my-queue-2", queueToDelete)
	})
	t.Run("errors if user makes an invalid choice", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "4")()
		queueToDelete, err = selectQueue(&queueList, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.Equal(t, "", queueToDelete)
	})
}

func TestUpdateQueueList(t *testing.T) {
	id1 := "q-1"
	id2 := "q-2"
	id3 := "q-3"
	id4 := "q-4"
	deploymentCelery := astroplatformcore.DeploymentExecutorCELERY
	existingQs := []astroplatformcore.WorkerQueueRequest{
		{
			Id:                &id1,
			Name:              "test-q",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 16,
		},
		{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    2,
			WorkerConcurrency: 18,
		},
	}
	t.Run("updates min, max, concurrency and node pool when queue exists", func(t *testing.T) {
		updatedQ := astroplatformcore.WorkerQueueRequest{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQ, &deploymentCelery, 3, 16, 20)
		assert.Equal(t, updatedQ, updatedQueueList[1])
	})
	t.Run("does not update id or isDefault when queue exists", func(t *testing.T) {
		updatedQRequest := astroplatformcore.WorkerQueueRequest{
			Id:                &id3,
			Name:              "test-q-1",
			IsDefault:         true,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQ := astroplatformcore.WorkerQueueRequest{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest, &deploymentCelery, 3, 16, 20)
		assert.Equal(t, updatedQ, updatedQueueList[1])
	})
	t.Run("does not change any queues if queue to update does not exist", func(t *testing.T) {
		updatedQRequest := astroplatformcore.WorkerQueueRequest{
			Id:                &id4,
			Name:              "test-q-does-not-exist",
			IsDefault:         true,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest, &deploymentCelery, 0, 0, 0)
		assert.Equal(t, existingQs, updatedQueueList)
	})
	t.Run("does not change any queues if user did not request min, max, concurrency", func(t *testing.T) {
		updatedQRequest := astroplatformcore.WorkerQueueRequest{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    5,
			WorkerConcurrency: 18,
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest, &deploymentCelery, -1, 0, 0)
		assert.Equal(t, existingQs, updatedQueueList)
	})
}

func TestGetQueueName(t *testing.T) {
	var (
		out                      *bytes.Buffer
		queueList                []astroplatformcore.WorkerQueue
		existingDeployment       astroplatformcore.Deployment
		err                      error
		expectedName, actualName string
	)
	out = new(bytes.Buffer)
	queueList = []astroplatformcore.WorkerQueue{
		{
			Id:        "queue-1",
			Name:      "default",
			IsDefault: true,
		},
		{
			Id:        "queue-2",
			Name:      "my-queue-2",
			IsDefault: false,
		},
		{
			Id:        "queue-3",
			Name:      "my-queue-3",
			IsDefault: false,
		},
	}
	existingDeployment.WorkerQueues = &queueList
	t.Run("prompts a user for a queue name when creating", func(t *testing.T) {
		expectedName = "queue-name"
		defer testUtil.MockUserInput(t, "queue-name")()
		actualName, err = getQueueName("", createAction, &existingDeployment, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedName, actualName)
	})
	t.Run("user selects a queue name when updating", func(t *testing.T) {
		expectedName = "my-queue-2"
		defer testUtil.MockUserInput(t, "2")()
		actualName, err = getQueueName("", updateAction, &existingDeployment, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedName, actualName)
	})
	t.Run("returns an error if user makes invalid queue choice when updating", func(t *testing.T) {
		expectedName = "my-queue-2"
		defer testUtil.MockUserInput(t, "8")()
		actualName, err = getQueueName("", updateAction, &existingDeployment, out)
		assert.Error(t, err)
	})
}

func TestSanitizeExistingQueues(t *testing.T) {
	testWorker := "test-worker"
	testWorker1 := "test-worker-1"
	var existingQs, actualQs []astroplatformcore.WorkerQueue
	existingQs = []astroplatformcore.WorkerQueue{
		{
			Id:                "q-1",
			Name:              "default",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 16,
			NodePoolId:        &testWorker,
		},
		{
			Id:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    2,
			WorkerConcurrency: 18,
			NodePoolId:        &testWorker1,
		},
	}
	t.Run("updates existing queues for CeleryExecutor", func(t *testing.T) {
		testWorker := "test-worker"
		testWorker1 := "test-worker-1"
		expectedQs := []astroplatformcore.WorkerQueue{
			{
				Id:                "q-1",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
				WorkerConcurrency: 16,
				NodePoolId:        &testWorker,
			},
			{
				Id:                "q-2",
				Name:              "test-q-1",
				IsDefault:         false,
				MaxWorkerCount:    15,
				MinWorkerCount:    2,
				WorkerConcurrency: 18,
				NodePoolId:        &testWorker1,
			},
		}
		actualQs = sanitizeExistingQueues(existingQs, deployment.CeleryExecutor)
		assert.Equal(t, expectedQs, actualQs)
	})
	t.Run("updates existing queues for KubernetesExecutor", func(t *testing.T) {
		existingQs = []astroplatformcore.WorkerQueue{existingQs[0]}
		expectedQs := []astroplatformcore.WorkerQueue{
			{
				Id:                "q-1",
				Name:              "default",
				IsDefault:         true,
				NodePoolId:        &testWorker,
				WorkerConcurrency: 16,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
			},
		}
		actualQs = sanitizeExistingQueues(existingQs, deployment.KubeExecutor)
		assert.Equal(t, expectedQs, actualQs)
	})
}
