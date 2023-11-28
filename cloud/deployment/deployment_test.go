package deployment

import (
	"bytes"
	"fmt"

	// "encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	enableCiCdEnforcement  = true
	disableCiCdEnforcement = false
	errMock                = errors.New("mock error")
	limit                  = 1000
	clusterType            = []astrocore.ListClustersParamsTypes{astrocore.BRINGYOUROWNCLOUD, astrocore.HOSTED}
	clusterListParams      = &astrocore.ListClustersParams{
		Types: &clusterType,
		Limit: &limit,
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
	hybridType                 = astroplatformcore.DeploymentTypeHYBRID
	testRegion                 = "region"
	testProvider               = "provider"
	testCluster                = "cluster"
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
			Status:      "HEALTHY",
			Type:        &hybridType,
			ClusterName: &testCluster,
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
	deploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:               "test-id-1",
			RuntimeVersion:   "4.2.5",
			Namespace:        "test-name",
			WorkspaceId:      ws,
			WebServerUrl:     "test-url",
			DagDeployEnabled: false,
			Name:             "test",
			Status:           "HEALTHY",
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("success", func(t *testing.T) {
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with no deployments in a workspace", func(t *testing.T) {
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.NoError(t, err)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with all true", func(t *testing.T) {
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List("", true, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)

		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("success with hidden namespace information", func(t *testing.T) {
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
			},
			{
				Id:          "test-id-2",
				Name:        "test",
				Status:      "HEALTHY",
				Type:        &hybridType,
				ClusterName: &testCluster,
			},
		}

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

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
			corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
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
			corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
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
			corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
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

	// t.Run("failed to validate resources", func(t *testing.T) {
	// 	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()
	// 	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

	// 	_, err := GetDeployment(ws, "", "", false, mockPlatformCoreClient, nil)
	// 	assert.ErrorIs(t, err, errMock)
	// 	mockClient.AssertExpectations(t)
	// })
}

func TestCoreGetDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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

	// t.Run("no deployments in workspace", func(t *testing.T) {
	// 	emptyDeploymentResponse := astroplatformcore.GetDeploymentResponse{
	// 		HTTPResponse: &http.Response{
	// 			StatusCode: 200,
	// 		},
	// 		JSON200: &astroplatformcore.Deployment{},
	// 	}
	// 	// mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&emptyListDeploymentsResponse, nil).Once()
	// 	mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyDeploymentResponse, nil).Once()
	// 	_, err := CoreGetDeployment(org, deploymentID, mockPlatformCoreClient)
	// 	assert.ErrorIs(t, err, ErrNoDeploymentExists)
	// 	mockCoreClient.AssertExpectations(t)
	// })

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

// func TestIsDeploymentHosted(t *testing.T) {
// 	t.Run("if deployment type is hosted", func(t *testing.T) {
// 		out := IsDeploymentHosted("HOSTED_SHARED")
// 		assert.Equal(t, out, true)
// 	})

// 	t.Run("if deployment type is hybrid", func(t *testing.T) {
// 		out := IsDeploymentHosted("")
// 		assert.Equal(t, out, false)
// 	})
// }

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	deploymentID := "test-id-1"
	logCount := 2
	var mockGetDeploymentLogsResponse = astrocore.GetDeploymentLogsResponse{
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	csID := "test-cluster-id"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient := new(astro_mocks.Client)

	var (
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
		// deploymentID                   = "test-deployment-id"
		GetDeploymentOptionsResponseOK = astrocore.GetDeploymentOptionsResponse{
			JSON200: &astrocore.DeploymentOptions{
				DefaultValues: astrocore.DefaultValueOptions{},
				Executors:     []string{},
			},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}
		cloudProvider                = "test-provider"
		hybridType                   = astroplatformcore.DeploymentTypeHYBRID
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
	)
	var GetClusterOptionsResponseOK = astrocore.GetClusterOptionsResponse{
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
		// mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

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
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "enable", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
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
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, errors.New("failed to create deployment")).Once()
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
		mockClient.AssertExpectations(t)
	})
	t.Run("failed to get default options", func(t *testing.T) {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Once()

		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster choice is not valid", func(t *testing.T) {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		// mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

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
		mockClient.AssertExpectations(t)
	})
	// t.Run("success with hidden namespace information", func(t *testing.T) {
	// 	testUtil.InitTestConfig(testUtil.CloudPlatform)
	// 	ctx, err := context.GetCurrentContext()
	// 	assert.NoError(t, err)
	// 	ctx.SetContextKey("organization_product", "HOSTED")
	// 	ctx.SetContextKey("organization", org)
	// 	ctx.SetContextKey("organization_short_name", org)
	// 	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
	// 		Components: astro.Components{
	// 			Scheduler: astro.SchedulerConfig{
	// 				AU: astro.AuConfig{
	// 					Default: 5,
	// 					Limit:   24,
	// 				},
	// 				Replicas: astro.ReplicasConfig{
	// 					Default: 1,
	// 					Minimum: 1,
	// 					Limit:   4,
	// 				},
	// 			},
	// 		},
	// 		RuntimeReleases: []astro.RuntimeRelease{
	// 			{
	// 				Version: "4.2.5",
	// 			},
	// 		},
	// 	}, nil).Times(2)
	// 	mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	// 	mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
	// 	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

	// 	getSharedClusterParams := &astrocore.GetSharedClusterParams{
	// 		Region:        region,
	// 		CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(astrocore.SharedClusterCloudProviderGcp),
	// 	}
	// 	mockOKResponse := &astrocore.GetSharedClusterResponse{
	// 		HTTPResponse: &http.Response{
	// 			StatusCode: 200,
	// 		},
	// 		JSON200: &astrocore.SharedCluster{Id: csID},
	// 	}
	// 	mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()

	// 	defer testUtil.MockUserInput(t, "test-name")()

	// 	// err = Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "gcp", region, "", "", "standard", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
	// 	// assert.NoError(t, err)
	// 	// ctx.SetContextKey("organization_product", "HYBRID")
	// 	// mockClient.AssertExpectations(t)
	// 	// mockCoreClient.AssertExpectations(t)
	// // })
	// t.Run("success with default config", func(t *testing.T) {
	// 	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
	// 		Components: astro.Components{
	// 			Scheduler: astro.SchedulerConfig{
	// 				AU: astro.AuConfig{
	// 					Default: 5,
	// 					Limit:   24,
	// 				},
	// 				Replicas: astro.ReplicasConfig{
	// 					Default: 1,
	// 					Minimum: 1,
	// 					Limit:   4,
	// 				},
	// 			},
	// 		},
	// 		RuntimeReleases: []astro.RuntimeRelease{
	// 			{
	// 				Version: "4.2.5",
	// 			},
	// 		},
	// 	}, nil).Times(2)
	// 	deploymentCreateInput := astro.CreateDeploymentInput{
	// 		WorkspaceID:           ws,
	// 		ClusterID:             csID,
	// 		Label:                 "test-name",
	// 		Description:           "test-desc",
	// 		RuntimeReleaseVersion: "4.2.5",
	// 		DagDeployEnabled:      false,
	// 		DeploymentSpec: astro.DeploymentCreateSpec{
	// 			Executor: CeleryExecutor,
	// 			Scheduler: astro.Scheduler{
	// 				AU:       5,
	// 				Replicas: 1,
	// 			},
	// 		},
	// 	}
	// 	mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	// 	mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, clusterListParams).Return(&mockListClustersResponse, nil).Once()
	// 	mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
	// 	mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

	// 	defer testUtil.MockUserInput(t, "test-name")()

	// 	// err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", 0, 0, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
	// 	// assert.NoError(t, err)
	// 	// mockClient.AssertExpectations(t)
	// 	// mockCoreClient.AssertExpectations(t)
	// })
}

// func TestValidateResources(t *testing.T) {
// 	t.Run("invalid schedulerAU", func(t *testing.T) {
// 		mockClient := new(astro_mocks.Client)
// 		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
// 			Components: astro.Components{
// 				Scheduler: astro.SchedulerConfig{
// 					AU: astro.AuConfig{
// 						Default: 5,
// 						Limit:   24,
// 					},
// 					Replicas: astro.ReplicasConfig{
// 						Default: 1,
// 						Minimum: 1,
// 						Limit:   4,
// 					},
// 				},
// 			},
// 		}, nil).Once()
// 		resp := validateResources(0, 3, astro.DeploymentConfig{
// 			Components: astro.Components{
// 				Scheduler: astro.SchedulerConfig{
// 					AU: astro.AuConfig{
// 						Default: 5,
// 						Limit:   24,
// 					},
// 					Replicas: astro.ReplicasConfig{
// 						Default: 1,
// 						Minimum: 1,
// 						Limit:   4,
// 					},
// 				},
// 			},
// 		})
// 		assert.False(t, resp)
// 	})

// 	t.Run("invalid runtime version", func(t *testing.T) {
// 		mockClient := new(astro_mocks.Client)
// 		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()

// 		resp, err := validateRuntimeVersion("4.2.4", mockClient)
// 		assert.NoError(t, err)
// 		assert.False(t, resp)
// 		mockClient.AssertExpectations(t)
// 	})
// }

func TestSelectCluster(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)

	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Executor: CeleryExecutor, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
		Cluster: astro.Cluster{
			NodePools: []astro.NodePool{
				{
					ID:               "test-node-pool-id",
					IsDefault:        true,
					NodeInstanceType: "test-default-node-pool",
					CreatedAt:        time.Time{},
				},
			},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:         "test-queue-id",
				Name:       "default",
				IsDefault:  true,
				NodePoolID: "test-default-node-pool",
			},
			{
				Name:       "test-queue",
				IsDefault:  false,
				NodePoolID: "test-node-pool-id",
			},
		},
	}
	deploymentUpdateInput := astro.UpdateDeploymentInput{
		ID:          "test-id",
		ClusterID:   "",
		Label:       "",
		Description: "",
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  CeleryExecutor,
			Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				Name:       "test-queue",
				IsDefault:  false,
				NodePoolID: "test-node-pool-id",
			},
		},
	}
	deploymentUpdateInput2 := astro.UpdateDeploymentInput{
		ID:          "test-id",
		ClusterID:   "",
		Label:       "test-label",
		Description: "test description",
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  CeleryExecutor,
			Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
		},
		WorkerQueues: nil,
	}

	t.Run("success", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Times(2)
		// expectedQueue := []astro.WorkerQueue{
		// 	{
		// 		Name:       "test-queue",
		// 		IsDefault:  false,
		// 		NodePoolID: "test-node-pool-id",
		// 	},
		// }
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Twice()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput2).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		// mock os.Stdin
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

		// err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, expectedQueue, false, nil, mockClient)
		// assert.NoError(t, err)

		// mock os.Stdin
		input = []byte("y")
		r, w, err = os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin = os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		// err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("successfully update schedulerSize and highAvailability", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", org)
		ctx.SetContextKey("workspace", ws)
		deploymentUpdateInput1 := astro.UpdateDeploymentInput{
			ID:          "test-id",
			ClusterID:   "",
			Label:       "",
			Description: "",
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  CeleryExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			IsHighAvailability: true,
			SchedulerSize:      "medium",
			WorkerQueues: []astro.WorkerQueue{
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}
		deploymentUpdateInput2 := astro.UpdateDeploymentInput{
			ID:                 "test-id",
			ClusterID:          "",
			Label:              "test-label",
			Description:        "test description",
			IsHighAvailability: false,
			SchedulerSize:      "small",
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  CeleryExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			WorkerQueues: nil,
		}
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Times(2)
		// expectedQueue := []astro.WorkerQueue{
		// 	{
		// 		Name:       "test-queue",
		// 		IsDefault:  false,
		// 		NodePoolID: "test-node-pool-id",
		// 	},
		// }

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Twice()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput1).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput2).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		// mock os.Stdin
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

		// err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "medium", "enable", 0, 0, expectedQueue, false, nil, mockClient)
		// assert.NoError(t, err)

		// mock os.Stdin
		input = []byte("y")
		r, w, err = os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin = os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		// err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "small", "disable", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("successfully update deployment for larger scheduler", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", org)
		ctx.SetContextKey("workspace", ws)
		deploymentUpdateInput1 := astro.UpdateDeploymentInput{
			ID:          "test-id",
			ClusterID:   "",
			Label:       "",
			Description: "",
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  CeleryExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			IsHighAvailability: true,
			SchedulerSize:      "medium",
			WorkerQueues: []astro.WorkerQueue{
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}
		deploymentResponse := astro.Deployment{
			ID:             "test-id",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			Type:           "HOSTED_DEDICATED",
			DeploymentSpec: astro.DeploymentSpec{Executor: CeleryExecutor, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
			Cluster: astro.Cluster{
				NodePools: []astro.NodePool{
					{
						ID:               "test-node-pool-id",
						IsDefault:        true,
						NodeInstanceType: "test-default-node-pool",
						CreatedAt:        time.Time{},
					},
				},
			},
			SchedulerSize: "large",
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-queue-id",
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-default-node-pool",
				},
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		expectedQueue := []astro.WorkerQueue{
			{
				Name:       "test-queue",
				IsDefault:  false,
				NodePoolID: "test-node-pool-id",
			},
		}
		fmt.Println(expectedQueue)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResponse}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput1).Return(astro.Deployment{ID: "test-id"}, nil).Times(1)

		// mock os.Stdin
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

		// err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "medium", "enable", 0, 0, expectedQueue, false, nil, mockClient)
		// assert.NoError(t, err)
	})

	t.Run("failed to validate resources", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		// err := Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.ErrorIs(t, err, errMock)
		// mockClient.AssertExpectations(t)
	})

	t.Run("success with hidden namespace information", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", org)

		expectedQueue := []astro.WorkerQueue{
			{
				Name:       "test-queue",
				IsDefault:  false,
				NodePoolID: "test-node-pool-id",
			},
		}
		fmt.Println(expectedQueue)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		// mock os.Stdin
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

		// err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, expectedQueue, false, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		// err := Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.ErrorIs(t, err, errMock)
		// mockClient.AssertExpectations(t)
	})

	t.Run("invalid deployment id", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}, {ID: "test-id-2", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Twice()

		// mock os.Stdin
		input := []byte("0")
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

		// err = Update("", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.ErrorIs(t, err, ErrInvalidDeploymentKey)

		// err = Update("test-invalid-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.ErrorIs(t, err, errInvalidDeployment)
		// mockClient.AssertExpectations(t)
	})

	t.Run("invalid resources", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Times(1)
		// mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		// err := Update("test-id", "test-label", ws, "test-description", "", "", CeleryExecutor, "", "", 10, 5, []astro.WorkerQueue{}, true, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("cancel update", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Once()

		// mock os.Stdin
		input := []byte("n")
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

		// err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("update deployment failure", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errMock).Once()

		// err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, []astro.WorkerQueue{}, true, nil, mockClient)
		// assert.ErrorIs(t, err, errMock)
		// assert.NotContains(t, err.Error(), astro.AstronomerConnectionErrMsg)
		// mockClient.AssertExpectations(t)
	})

	t.Run("update deployment to enable dag deploy", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentUpdateInput := astro.UpdateDeploymentInput{
			ID:               "test-id",
			ClusterID:        "",
			Label:            "",
			Description:      "",
			DagDeployEnabled: true,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  CeleryExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			WorkerQueues: nil,
		}
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		// err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("do not update deployment to enable dag deploy if already enabled", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			DagDeployEnabled: true,
			DeploymentSpec: astro.DeploymentSpec{
				Executor: CeleryExecutor,
			},
		}

		// mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		// err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("throw warning to enable dag deploy if ci-cd enforcement is enabled", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			DagDeployEnabled: true,
			DeploymentSpec: astro.DeploymentSpec{
				Executor: CeleryExecutor,
			},
			APIKeyOnlyDeployments: true,
		}

		canCiCdDeploy = func(astroAPIToken string) bool {
			return false
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		// err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		// assert.NoError(t, err)

		// mockClient.AssertExpectations(t)
	})

	t.Run("update deployment to disable dag deploy", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Times(3)
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			Label:            "test-deployment",
			RuntimeRelease:   astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec:   astro.DeploymentSpec{Executor: CeleryExecutor, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
			DagDeployEnabled: true,
			Cluster: astro.Cluster{
				NodePools: []astro.NodePool{
					{
						ID:               "test-node-pool-id",
						IsDefault:        true,
						NodeInstanceType: "test-default-node-pool",
						CreatedAt:        time.Time{},
					},
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-queue-id",
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-default-node-pool",
				},
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}

		deploymentUpdateInput := astro.UpdateDeploymentInput{
			ID:               "test-id",
			ClusterID:        "",
			Label:            "test-deployment",
			Description:      "",
			DagDeployEnabled: false,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  CeleryExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			WorkerQueues: nil,
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Times(3)
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Twice()

		// // force is false so we will confirm with the user
		// defer testUtil.MockUserInput(t, "y")()
		// err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.NoError(t, err)

		// // force is false so we will confirm with the user
		// defer testUtil.MockUserInput(t, "n")()
		// err = Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.NoError(t, err)

		// // force is true so no confirmation is needed
		// err = Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("do not update deployment to disable dag deploy if already disabled", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			DagDeployEnabled: false,
			DeploymentSpec: astro.DeploymentSpec{
				Executor: CeleryExecutor,
			},
		}

		// mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		// err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("update deployment to change executor to KubernetesExecutor", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			RuntimeRelease:   astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec:   astro.DeploymentSpec{Executor: CeleryExecutor, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
			DagDeployEnabled: true,
			Cluster: astro.Cluster{
				NodePools: []astro.NodePool{
					{
						ID:               "test-node-pool-id",
						IsDefault:        true,
						NodeInstanceType: "test-default-node-pool",
						CreatedAt:        time.Time{},
					},
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-queue-id",
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-default-node-pool",
				},
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}

		deploymentUpdateInput := astro.UpdateDeploymentInput{
			ID:               "test-id",
			ClusterID:        "",
			Label:            "",
			Description:      "",
			DagDeployEnabled: false,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  KubeExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			WorkerQueues: nil,
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		defer testUtil.MockUserInput(t, "y")()
		// err := Update("test-id", "", ws, "", "", "", KubeExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("update deployment to change executor to CeleryExecutor", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			RuntimeRelease:   astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec:   astro.DeploymentSpec{Executor: KubeExecutor, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
			DagDeployEnabled: true,
			Cluster: astro.Cluster{
				NodePools: []astro.NodePool{
					{
						ID:               "test-node-pool-id",
						IsDefault:        true,
						NodeInstanceType: "test-default-node-pool",
						CreatedAt:        time.Time{},
					},
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-queue-id",
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-default-node-pool",
				},
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}

		deploymentUpdateInput := astro.UpdateDeploymentInput{
			ID:               "test-id",
			ClusterID:        "",
			Label:            "",
			Description:      "",
			DagDeployEnabled: false,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor:  CeleryExecutor,
				Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
			},
			WorkerQueues: nil,
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		defer testUtil.MockUserInput(t, "y")()
		// err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("do not update deployment if user says no to the executor change", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID:               "test-id",
			Label:            "test-deployment",
			RuntimeRelease:   astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec:   astro.DeploymentSpec{Executor: KubeExecutor, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
			DagDeployEnabled: true,
			Cluster: astro.Cluster{
				NodePools: []astro.NodePool{
					{
						ID:               "test-node-pool-id",
						IsDefault:        true,
						NodeInstanceType: "test-default-node-pool",
						CreatedAt:        time.Time{},
					},
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-queue-id",
					Name:       "default",
					IsDefault:  true,
					NodePoolID: "test-default-node-pool",
				},
				{
					Name:       "test-queue",
					IsDefault:  false,
					NodePoolID: "test-node-pool-id",
				},
			},
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		// err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("do not update deployment with executor if deployment already has it", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID: "test-id",
			DeploymentSpec: astro.DeploymentSpec{
				Executor: KubeExecutor,
			},
		}

		// mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		// err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})

	t.Run("do not update deployment with executor if user did not request it", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		deploymentResp = astro.Deployment{
			ID: "test-id",
			DeploymentSpec: astro.DeploymentSpec{
				Executor: KubeExecutor,
			},
		}

		// mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		// err := Update("test-id", "", ws, "", "", "disable", "", "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		// assert.NoError(t, err)
		// mockClient.AssertExpectations(t)
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		// mock os.Stdin
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

		err = Delete("test-id", ws, "", false, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", false, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("no deployments in a workspace", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()

		err := Delete("test-id", ws, "", true, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid deployment id", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}, {ID: "test-id-2", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Twice()

		// mock os.Stdin
		input := []byte("0")
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

		err = Delete("", ws, "", false, mockPlatformCoreClient)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)

		err = Delete("test-invalid-id", ws, "", false, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errInvalidDeployment)
		mockClient.AssertExpectations(t)
	})

	t.Run("cancel update", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Once()

		// mock os.Stdin
		input := []byte("n")
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

		err = Delete("test-id", ws, "", false, mockPlatformCoreClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("delete deployment failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", true, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
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

func TestValidateHostedResources_ValidInput(t *testing.T) {
	// Test with valid input values
	defaultTaskPodCpu := "2"
	defaultTaskPodMemory := "4Gi"
	resourceQuotaCpu := "8"
	resourceQuotaMemory := "16Gi"

	configOption := astrocore.DeploymentOptions{
		ResourceQuotas: astrocore.ResourceQuotaOptions{
			DefaultPodSize: astrocore.ResourceOption{
				Cpu:    astrocore.ResourceRange{Floor: "1", Ceiling: "4"},
				Memory: astrocore.ResourceRange{Floor: "2Gi", Ceiling: "8Gi"},
			},
			ResourceQuota: astrocore.ResourceOption{
				Cpu:    astrocore.ResourceRange{Floor: "2", Ceiling: "16"},
				Memory: astrocore.ResourceRange{Floor: "4Gi", Ceiling: "32Gi"},
			},
		},
	}

	result := validateHostedResources(defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, configOption)

	assert.True(t, result, "Expected validation to pass with valid input values")
}

func TestValidateHostedResources_InvalidDefaultTaskPodCpu(t *testing.T) {
	// Test with invalid Default Task Pod CPU
	defaultTaskPodCpu := "5"
	defaultTaskPodMemory := "4Gi"
	resourceQuotaCpu := "8"
	resourceQuotaMemory := "16Gi"

	configOption := astrocore.DeploymentOptions{
		ResourceQuotas: astrocore.ResourceQuotaOptions{
			DefaultPodSize: astrocore.ResourceOption{
				Cpu:    astrocore.ResourceRange{Floor: "1", Ceiling: "4"},
				Memory: astrocore.ResourceRange{Floor: "2Gi", Ceiling: "8Gi"},
			},
			ResourceQuota: astrocore.ResourceOption{
				Cpu:    astrocore.ResourceRange{Floor: "2", Ceiling: "16"},
				Memory: astrocore.ResourceRange{Floor: "4Gi", Ceiling: "32Gi"},
			},
		},
	}

	result := validateHostedResources(defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, configOption)

	assert.False(t, result, "Expected validation to fail with invalid Default Task Pod CPU")
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
