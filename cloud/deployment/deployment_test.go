package deployment

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	hybridQueueList         = []astroplatformcore.HybridWorkerQueueRequest{}
	workerQueueRequest      = []astroplatformcore.WorkerQueueRequest{}
	newEnvironmentVariables = []astroplatformcore.DeploymentEnvironmentVariableRequest{}
	errMock                 = errors.New("mock error")
	errCreateFailed         = errors.New("failed to create deployment")
	nodePools               = []astroplatformcore.NodePool{
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
	mockListClustersResponse = astroplatformcore.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.ClustersPaginated{
			Clusters: []astroplatformcore.Cluster{
				{
					Id:        "test-cluster-id",
					Name:      "test-cluster",
					NodePools: &nodePools,
				},
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
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
	standardType               = astroplatformcore.DeploymentTypeSTANDARD
	dedicatedType              = astroplatformcore.DeploymentTypeDEDICATED
	hybridType                 = astroplatformcore.DeploymentTypeHYBRID
	testRegion                 = "region"
	testProvider               = "provider"
	testCluster                = "cluster"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:            "test-id-1",
			Name:          "test-1",
			Status:        "HEALTHY",
			Type:          &standardType,
			Region:        &testRegion,
			CloudProvider: &testProvider,
			WorkspaceName: &workspace1.Name,
		},
		{
			Id:            "test-id-2",
			Name:          "test",
			Status:        "HEALTHY",
			Type:          &hybridType,
			ClusterName:   &testCluster,
			WorkspaceName: &workspace1.Name,
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
	emptyListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{},
		},
	}
	schedulerAU         = 0
	clusterID           = "cluster-id"
	executorCelery      = astroplatformcore.DeploymentExecutorCELERY
	executorKubernetes  = astroplatformcore.DeploymentExecutorKUBERNETES
	highAvailability    = true
	resourceQuotaCPU    = "1cpu"
	ResourceQuotaMemory = "1"
	schedulerSize       = astroplatformcore.DeploymentSchedulerSizeSMALL
	deploymentResponse  = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                  "test-id-1",
			RuntimeVersion:      "4.2.5",
			Namespace:           "test-name",
			WorkspaceId:         ws,
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			Name:                "test",
			Status:              "HEALTHY",
			Type:                &hybridType,
			SchedulerAu:         &schedulerAU,
			ClusterId:           &clusterID,
			Executor:            &executorCelery,
			IsHighAvailability:  &highAvailability,
			ResourceQuotaCpu:    &resourceQuotaCPU,
			ResourceQuotaMemory: &ResourceQuotaMemory,
			SchedulerSize:       &schedulerSize,
			WorkerQueues:        &[]astroplatformcore.WorkerQueue{},
		},
	}
	GetDeploymentOptionsResponseOK = astrocore.GetDeploymentOptionsResponse{
		JSON200: &astrocore.DeploymentOptions{
			DefaultValues: astrocore.DefaultValueOptions{
				WorkerMachineName: "test-machine",
			},
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
			WorkerMachines: []astrocore.WorkerMachine{
				{
					Concurrency: astrocore.Range{
						Ceiling: 1,
						Default: 1,
						Floor:   1,
					},
					Name: "test-machine",
					Spec: astrocore.MachineSpec{},
				},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	workspaceTestDescription = "test workspace"
	workspace1               = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &workspaceTestDescription,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "workspace-id",
		OrganizationId:               "test-org-id",
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
)

const (
	org              = "test-org-id"
	ws               = "workspace-id"
	dagDeploy        = "disable"
	region           = "us-central1"
	mockOrgShortName = "test-org-short-name"
)

var (
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient         = new(astrocore_mocks.ClientWithResponsesInterface)
)

func TestList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("success", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with no deployments in a workspace", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with all true", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List("", true, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with hidden namespace information", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")
		assert.Contains(t, buf.String(), "N/A")
		assert.Contains(t, buf.String(), "region")
		assert.Contains(t, buf.String(), "cluster")

		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestGetDeployment(t *testing.T) {
	deploymentID := "test-id-wrong"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("invalid deployment ID", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, deploymentID, "", false, mockPlatformCoreClient, nil)
		assert.ErrorIs(t, err, errInvalidDeployment)

		mockPlatformCoreClient.AssertExpectations(t)
	})
	deploymentName := "test-wrong"
	deploymentID = "test-id-1"
	t.Run("error after invalid deployment Name", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, false, mockPlatformCoreClient, nil)
		assert.ErrorIs(t, err, errInvalidDeployment)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("no deployments in workspace", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, true, mockPlatformCoreClient, nil)
		assert.ErrorIs(t, err, errInvalidDeployment)

		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("correct deployment ID", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, "", false, mockPlatformCoreClient, nil)

		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.Id)

		mockPlatformCoreClient.AssertExpectations(t)
	})
	deploymentName = "test"
	t.Run("correct deployment Name", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, false, mockPlatformCoreClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentName, deployment.Name)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("two deployments with the same name", func(t *testing.T) {
		mockCoreDeploymentResponse = []astroplatformcore.Deployment{
			{
				Id:            "test-id-1",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &standardType,
				Region:        &testRegion,
				CloudProvider: &testProvider,
				WorkspaceName: &workspace1.Name,
			},
			{
				Id:            "test-id-2",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &hybridType,
				ClusterName:   &testCluster,
				WorkspaceName: &workspace1.Name,
			},
		}

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		deployment, err := GetDeployment(ws, "", deploymentName, false, mockPlatformCoreClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentName, deployment.Name)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("deployment name and deployment id", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, mockPlatformCoreClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.Id)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("create deployment error", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, cicdEnforcement,
			defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory string,
			deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int,
			platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
		) error {
			return errMock
		}

		// Mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = GetDeployment(ws, "", "", false, mockPlatformCoreClient, nil)
		assert.ErrorIs(t, err, errMock)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("get deployments after creation error", func(t *testing.T) {
		// Assuming org, ws, and errMock are predefined
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory string,
			deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int,
			platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
		) error {
			return nil
		}

		// Second ListDeployments call with an error to simulate an error after deployment creation
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Once()

		// Mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = GetDeployment(ws, "", "", false, mockPlatformCoreClient, nil)
		assert.ErrorIs(t, err, errMock)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("test automatic deployment creation", func(t *testing.T) {
		// Assuming org, ws, and deploymentID are predefined
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, cicdEnforcement,
			defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory string,
			deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int,
			platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
		) error {
			return nil
		}

		mockCoreDeploymentResponse = []astroplatformcore.Deployment{
			{
				Id:            "test-id-1",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &standardType,
				Region:        &testRegion,
				CloudProvider: &testProvider,
			},
		}

		mockListOneDeploymentsResponse := astroplatformcore.ListDeploymentsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.DeploymentsPaginated{
				Deployments: mockCoreDeploymentResponse,
			},
		}

		// Second ListDeployments call after successful deployment creation
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListOneDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		deployment, err := GetDeployment(ws, "", "", false, mockPlatformCoreClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.Id)

		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestCoreGetDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	deploymentID := "test-id-1"
	t.Run("success", func(t *testing.T) {
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := CoreGetDeployment(org, deploymentID, mockPlatformCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.Id)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success from context", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_short_name", org)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := CoreGetDeployment("", deploymentID, mockPlatformCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.Id)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("context error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)

		_, err := CoreGetDeployment("", deploymentID, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
	})

	t.Run("error in api response", func(t *testing.T) {
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, errMock).Once()

		_, err := CoreGetDeployment(org, deploymentID, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestIsDeploymentDedicated(t *testing.T) {
	t.Run("if deployment type is dedicated", func(t *testing.T) {
		out := IsDeploymentDedicated(astroplatformcore.DeploymentTypeDEDICATED)
		assert.Equal(t, out, true)
	})

	t.Run("if deployment type is hybrid", func(t *testing.T) {
		out := IsDeploymentDedicated(astroplatformcore.DeploymentTypeHYBRID)
		assert.Equal(t, out, false)
	})
}

func TestSelectRegion(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("list regions failure", func(t *testing.T) {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(nil, errMock).Once()

		_, err := selectRegion("gcp", "", mockCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("region via selection", func(t *testing.T) {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectRegion("gcp", "", mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, region, resp)
	})

	t.Run("region via selection aws", func(t *testing.T) {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderAws) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectRegion("aws", "", mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, region, resp)
	})

	t.Run("region invalid selection", func(t *testing.T) {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		// mock os.Stdin
		input := []byte("4")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = selectRegion("gcp", "", mockCoreClient)
		assert.ErrorIs(t, err, ErrInvalidRegionKey)
	})

	t.Run("not able to find region", func(t *testing.T) {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		_, err := selectRegion("gcp", "test-invalid-region", mockCoreClient)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to find specified Region")
	})
}

func TestLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	deploymentID := "test-id-1"
	logCount := 2
	mockGetDeploymentLogsResponse := astrocore.GetDeploymentLogsResponse{
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

	t.Run("success", func(t *testing.T) {
		// Mock ListDeployments
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()

		err := Logs(deploymentID, ws, "", true, false, false, logCount, mockPlatformCoreClient, mockCoreClient)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("success without deployment", func(t *testing.T) {
		// Mock ListDeployments
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		// Mock GetDeploymentWithResponse for the first deployment in the list
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock GetDeploymentLogsWithResponse using the reusable variable
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()

		// Mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Logs("", ws, "", false, false, false, 1, mockPlatformCoreClient, mockCoreClient)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		// Mock ListDeployments
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		// Mock GetDeploymentWithResponse for the first deployment in the list
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock GetDeploymentLogsWithResponse to return an error
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, errMock).Once()

		err := Logs(deploymentID, ws, "", false, false, false, logCount, mockPlatformCoreClient, mockCoreClient)
		assert.ErrorIs(t, err, errMock)

		mockPlatformCoreClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("query for more than one log level error", func(t *testing.T) {
		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockPlatformCoreClient, mockCoreClient)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot query for more than one log level at a time")
	})
}

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	csID := "test-cluster-id"
	var (
		cloudProvider                = "test-provider"
		mockCreateDeploymentResponse = astroplatformcore.CreateDeploymentResponse{
			JSON200: &astroplatformcore.Deployment{
				Id:            "test-id",
				CloudProvider: &cloudProvider,
				Type:          &hybridType,
				ClusterName:   &cluster.Name,
				Region:        &cluster.Region,
			},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}
	)
	GetClusterOptionsResponseOK := astrocore.GetClusterOptionsResponse{
		JSON200: &[]astrocore.ClusterOptions{
			{
				DatabaseInstances:       []astrocore.ProviderInstanceType{},
				DefaultDatabaseInstance: astrocore.ProviderInstanceType{},
				DefaultNodeInstance:     astrocore.ProviderInstanceType{},

				DefaultPodSubnetRange:      nil,
				DefaultRegion:              astrocore.ProviderRegion{Name: "us-west-2"},
				DefaultServicePeeringRange: nil,
				DefaultServiceSubnetRange:  nil,
				DefaultVpcSubnetRange:      "10.0.0.0/16",
				NodeCountDefault:           1,
				NodeCountMax:               3,
				NodeCountMin:               1,
				NodeInstances:              []astrocore.ProviderInstanceType{},
				Regions:                    []astrocore.ProviderRegion{{Name: "us-west-1"}, {Name: "us-west-2"}},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	t.Run("success with Celery Executor", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "1")()

		// Call the Create function
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("success with Celery Executor and differen schedulers", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(3)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(3)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "1")()

		// Call the Create function
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", SmallScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", MediumScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", LargeScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("success with enabling ci-cd enforcement", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "test-name")()

		// Call the Create function with ci-cd enforcement enabled
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "enable", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("success with cloud provider and region", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "test-name")()

		// Call the Create function with deployment type as STANDARD, cloud provider, and region set
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with standard/dedicated type different scheduler sizes", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(6)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(6)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(3)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "test-name")()

		// Call the Create function with deployment type as STANDARD, cloud provider, and region set
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", SmallScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, azureCloud, "us-west-2", MediumScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, gcpCloud, "us-west-2", LargeScheduler, "enable", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Call the Create function with deployment type as DEDICATED, cloud provider, and region set
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", SmallScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, azureCloud, "us-west-2", MediumScheduler, "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, gcpCloud, "us-west-2", LargeScheduler, "enable", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("success with region selection", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetClusterOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for region selection
		defer testUtil.MockUserInput(t, "1")()

		// Call the Create function with region selection
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("success with Kube Executor", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Mock user input for deployment name and executor type
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "2")()

		// Call the Create function with Kube Executor
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("success and wait for status with Dedicated Deployment", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(t, "test-name")()
		defer testUtil.MockUserInput(t, "y")()

		// setup wait for test
		sleepTime = 1
		tickNum = 2

		// Call the Create function with Dedicated Deployment and wait for status
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, true)
		assert.NoError(t, err)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when creating a Hybrid deployment fails", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, errCreateFailed).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "test-name")()

		// Call the Create function with Hybrid Deployment that returns an error during creation
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create deployment")

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("failed to get workspaces", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, errMock).Once()

		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("failed to get default options", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Once()

		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster choice is not valid", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Mock invalid user input for cluster choice
		defer testUtil.MockUserInput(t, "invalid-cluster-choice")()

		// Call the Create function and expect an error due to invalid cluster choice
		err := Create("test-name", ws, "test-desc", "", "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid Cluster selected")

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("invalid hybrid resources", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Call the Create function and expect an error due to invalid cluster choice
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 10, 10, mockPlatformCoreClient, mockCoreClient, false)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidResourceRequest)

		// Assert expectations
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("invalid workspace failure", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		err := Create("test-name", "wrong-workspace", "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.ErrorContains(t, err, "no workspaces with id")
		mockCoreClient.AssertExpectations(t)
	})
}

func TestSelectCluster(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	csID := "test-cluster-id"
	t.Run("list cluster failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListClustersResponse{}, errMock).Once()

		_, err := selectCluster("", mockOrgShortName, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("cluster id via selection", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectCluster("", mockOrgShortName, mockPlatformCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, csID, resp)
	})

	t.Run("cluster id invalid selection", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// mock os.Stdin
		input := []byte("4")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = selectCluster("", mockOrgShortName, mockPlatformCoreClient)
		assert.ErrorIs(t, err, ErrInvalidClusterKey)
	})

	t.Run("not able to find cluster", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		_, err := selectCluster("test-invalid-id", mockOrgShortName, mockPlatformCoreClient)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to find specified Cluster")
	})
}

func TestCanCiCdDeploy(t *testing.T) {
	permissions := []string{}
	mockClaims := util.CustomClaims{
		Permissions: permissions,
	}

	parseToken = func(astroAPIToken string) (*util.CustomClaims, error) {
		return &mockClaims, nil
	}

	canDeploy := CanCiCdDeploy("bearer token")
	assert.Equal(t, canDeploy, false)

	parseToken = func(astroAPIToken string) (*util.CustomClaims, error) {
		return nil, errMock
	}
	canDeploy = CanCiCdDeploy("bearer token")
	assert.Equal(t, canDeploy, false)

	permissions = []string{
		"workspaceId:workspace-id",
		"organizationId:org-ID",
		"orgShortName:org-short-name",
	}
	mockClaims = util.CustomClaims{
		Permissions: permissions,
	}

	parseToken = func(astroAPIToken string) (*util.CustomClaims, error) {
		return &mockClaims, nil
	}

	canDeploy = CanCiCdDeploy("bearer token")
	assert.Equal(t, canDeploy, true)
}

func TestUpdate(t *testing.T) { //nolint
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cloudProvider := "test-provider"
	astroMachine := "test-machine"
	nodeID := "test-id"
	varValue := "VALUE"
	deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{
		{
			AstroMachine:      &astroMachine,
			Id:                "queue-id",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			Name:              "worker-name",
			WorkerConcurrency: 20,
			NodePoolId:        &nodeID,
		},
	}
	deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
		{
			IsSecret: false,
			Key:      "KEY",
			Value:    &varValue,
		},
	}
	deploymentResponse.JSON200.Executor = &executorCelery
	mockUpdateDeploymentResponse := astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Id:            "test-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
			Region:        &cluster.Region,
			ClusterName:   &cluster.Name,
		},

		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	t.Run("update success", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(6)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Twice()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "1")()

		// success with hybrid type
		err := Update("", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, true, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()
		defer testUtil.MockUserInput(t, "y")()

		// kubernetes executor
		err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()
		defer testUtil.MockUserInput(t, "y")()

		// success with standard type
		err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "y")()

		// kubernetes executor
		err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// change type to dedicatd
		deploymentResponse.JSON200.Type = &dedicatedType
		// mock os.Stdin
		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()

		// success with dedicated type
		err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "y")()

		// kubernetes executor
		err = Update("test-id-1", "", ws, "", "", "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("successfully update schedulerSize and highAvailability and CICDEnforement", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(4)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(4)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(4)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		// mock os.Stdin
		// Mock user input for deployment name

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "1")()
		// success with standard type with name
		err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "disable", "", "", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// change type to dedicatd
		deploymentResponse.JSON200.Type = &dedicatedType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "1")()

		// success with dedicated type
		err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "enable", "disable", "", "", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// success with hybrid type with id
		deploymentResponse.JSON200.Type = &hybridType
		err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "y")()

		// success with hybrid type with id
		deploymentResponse.JSON200.Executor = &executorKubernetes
		err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		deploymentResponse.JSON200.Executor = &executorKubernetes
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
		deploymentResponse.JSON200.Executor = &executorCelery
	})
	t.Run("failed to validate resources", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		deploymentResponse.JSON200.Type = &hybridType
		err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "10Gi", "2CPU", "10Gi", 100, 100, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "invaild resource request")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("invalid deployment id", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)
		// list deployment error
		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)

		// invalid id
		err = Update("invalid-id", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "the Deployment specified was not found in this workspace.")
		// invalid name
		err = Update("", "", ws, "update", "invalid-name", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "the Deployment specified was not found in this workspace.")

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "0")()

		// invalid selection
		err = Update("", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "invalid Deployment selected")

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("cancel update", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		deploymentResponse.JSON200.IsDagDeployEnabled = true
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(t, "n")()

		err := Update("test-id-1", "", ws, "update", "", "disable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("update deployment failure", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errMock).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		assert.NotContains(t, err.Error(), organization.AstronomerConnectionErrMsg)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("do not update deployment to enable dag deploy if already enabled", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		deploymentResponse.JSON200.IsDagDeployEnabled = true
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("throw warning to enable dag deploy if ci-cd enforcement is enabled", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		canCiCdDeploy = func(astroAPIToken string) bool {
			return false
		}

		defer testUtil.MockUserInput(t, "n")()
		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "enable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("do not update deployment to disable dag deploy if already disabled", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "disable", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("update deployment to change executor to KubernetesExecutor", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		defer testUtil.MockUserInput(t, "y")()

		err := Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		defer testUtil.MockUserInput(t, "y")()
		// test update with standard type
		err = Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "disable", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// change type to standard
		deploymentResponse.JSON200.Type = &dedicatedType

		defer testUtil.MockUserInput(t, "y")()
		// test update with standard type
		err = Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "disable", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)

		assert.NoError(t, err)
		assert.Equal(t, (*[]astroplatformcore.WorkerQueueRequest)(nil), dedicatedDeploymentRequest.WorkerQueues)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("update deployment to change executor to CeleryExecutor", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		deploymentResponse.JSON200.Executor = &executorKubernetes

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		defer testUtil.MockUserInput(t, "y")()
		// test update with standard type
		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "disable", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		// change type to standard
		deploymentResponse.JSON200.Type = &dedicatedType

		defer testUtil.MockUserInput(t, "y")()
		// test update with standard type
		err = Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "disable", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		defer testUtil.MockUserInput(t, "y")()

		// test update with hybrid type
		deploymentResponse.JSON200.Type = &hybridType
		err = Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)

		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("do not update deployment if user says no to the executor change", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		deploymentResponse.JSON200.Executor = &executorKubernetes

		defer testUtil.MockUserInput(t, "n")()

		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "disable", "2CPU", "2Gi", "2CPU", "2Gi", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockDeleteDeploymentResponse := astroplatformcore.DeleteDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	t.Run("success", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, nil).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(t, "y")()

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("no deployments in a workspace", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Times(1)

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("invalid deployment id", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(t, "0")()

		err := Delete("", ws, "", false, mockPlatformCoreClient)
		assert.ErrorContains(t, err, "invalid Deployment selected")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("cancel update", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "n")()

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("delete deployment failure", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, errMock).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(t, "y")()

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestGetDeploymentURL(t *testing.T) {
	deploymentID := "deployment-id"
	workspaceID := "workspace-id"

	t.Run("returns deploymentURL for dev environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudDevPlatform)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for stage environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudStagePlatform)
		expectedURL := "cloud.astronomer-stage.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for perf environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPerfPlatform)
		expectedURL := "cloud.astronomer-perf.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for cloud (prod) environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedURL := "cloud.astronomer.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for pr preview environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for local environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedURL := "localhost:5000/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns an error if getting current context fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		expectedURL := ""
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.ErrorContains(t, err, "no context set")
		assert.Equal(t, expectedURL, actualURL)
	})
}

func TestPrintWarning(t *testing.T) {
	t.Run("when KubernetesExecutor is requested", func(t *testing.T) {
		t.Run("returns true > 1 queues exist", func(t *testing.T) {
			actual := printWarning(KubeExecutor, 3)
			assert.True(t, actual)
		})
		t.Run("returns false if only 1 queue exists", func(t *testing.T) {
			actual := printWarning(KubeExecutor, 1)
			assert.False(t, actual)
		})
	})
	t.Run("returns true when CeleryExecutor is requested", func(t *testing.T) {
		actual := printWarning(CeleryExecutor, 2)
		assert.True(t, actual)
	})
	t.Run("returns false for any other executor is requested", func(t *testing.T) {
		actual := printWarning("non-existent", 2)
		assert.False(t, actual)
	})
}

func TestGetPlatformDeploymentOptions(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("function success", func(t *testing.T) {
		GetDeploymentOptionsResponse := astroplatformcore.GetDeploymentOptionsResponse{
			JSON200: &astroplatformcore.DeploymentOptions{},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}

		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponse, nil).Times(1)

		_, err := GetPlatformDeploymentOptions("", astroplatformcore.GetDeploymentOptionsParams{}, mockPlatformCoreClient)
		assert.NoError(t, err)
	})

	t.Run("function failure", func(t *testing.T) {
		GetDeploymentOptionsResponse := astroplatformcore.GetDeploymentOptionsResponse{
			JSON200: &astroplatformcore.DeploymentOptions{},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}

		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponse, errMock).Times(1)

		_, err := GetPlatformDeploymentOptions("", astroplatformcore.GetDeploymentOptionsParams{}, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
	})
}
