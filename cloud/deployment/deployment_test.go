package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	enableCiCdEnforcement  = true
	disableCiCdEnforcement = false
	errMock                = errors.New("mock error")
	limit                  = 1000
	clusterType            = []astrocore.ListClustersParamsType{astrocore.BRINGYOUROWNCLOUD, astrocore.HOSTED}
	clusterListParams      = &astrocore.ListClustersParams{
		Type:  &clusterType,
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
)

const (
	org              = "test-org-id"
	ws               = "test-ws-id"
	dagDeploy        = "disable"
	region           = "us-central1"
	mockOrgShortName = "test-org-short-name"
)

func TestList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockClient.AssertExpectations(t)
	})

	t.Run("success with all true", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, "").Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err := List("", true, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("success with hidden cluster information", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", org)

		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err = List(ws, false, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")
		assert.Contains(t, buf.String(), "N/A")

		mockClient.AssertExpectations(t)
	})
}

func TestGetDeployment(t *testing.T) {
	deploymentID := "test-id-wrong"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)

	t.Run("invalid deployment ID", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		_, err := GetDeployment(ws, deploymentID, "", mockClient, nil)
		assert.ErrorIs(t, err, errInvalidDeployment)
		mockClient.AssertExpectations(t)
	})
	deploymentName := "test-wrong"
	deploymentID = "test-id"
	t.Run("error after invalid deployment Name", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{Label: "test", ID: "test-id"}}, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, mockClient, nil)
		assert.ErrorIs(t, err, errInvalidDeployment)
		mockClient.AssertExpectations(t)
	})

	t.Run("correct deployment ID", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, "", mockClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})
	deploymentName = "test"
	t.Run("correct deployment Name", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{Label: "test"}}, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentName, deployment.Label)
		mockClient.AssertExpectations(t)
	})

	t.Run("two deployments with the same name", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{Label: "test", ID: "test-id"}, {Label: "test", ID: "test-id2"}}, nil).Once()

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

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentName, deployment.Label)
		mockClient.AssertExpectations(t)
	})

	t.Run("deployment name and deployment id", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, deploymentName, mockClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})

	t.Run("bad deployment call", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		_, err := GetDeployment(ws, deploymentID, "", mockClient, nil)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("create deployment error", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()
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
		}, nil).Once()
		// mock createDeployment
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool, apiKeyOnlyDeployments *bool) error {
			return errMock
		}

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

		_, err = GetDeployment(ws, "", "", mockClient, nil)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("get deployments after creation error", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()
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
		}, nil).Once()
		// mock createDeployment
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool, apiKeyOnlyDeployments *bool) error {
			return nil
		}
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, errMock).Once()
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

		_, err = GetDeployment(ws, "", "", mockClient, nil)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("test automatic deployment creation", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()
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
		}, nil).Once()
		// mock createDeployment
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool, apiKeyOnlyDeployments *bool) error {
			return nil
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
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

		deployment, err := GetDeployment(ws, "", "", mockClient, nil)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})
	t.Run("failed to validate resources", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		_, err := GetDeployment(ws, "", "", mockClient, nil)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}

func TestIsDeploymentHosted(t *testing.T) {
	t.Run("if deployment type is hosted", func(t *testing.T) {
		out := IsDeploymentHosted("HOSTED_SHARED")
		assert.Equal(t, out, true)
	})

	t.Run("if deployment type is hybrid", func(t *testing.T) {
		out := IsDeploymentHosted("")
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
	deploymentID := "test-id"
	logCount := 1
	logLevels := []string{"WARN", "ERROR", "INFO"}
	mockInput := map[string]interface{}{
		"deploymentId":  deploymentID,
		"logCountLimit": logCount,
		"start":         "-24hrs",
		"logLevels":     logLevels,
	}
	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{{Raw: "test log line"}}}, nil).Once()

		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockClient)
		assert.NoError(t, err)

		mockClient.AssertExpectations(t)
	})

	t.Run("success without deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}, {ID: "test-id-1"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{}}, nil).Once()

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

		err = Logs("", ws, "", false, false, false, logCount, mockClient)
		assert.NoError(t, err)

		mockClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{}, errMock).Once()

		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	csID := "test-cluster-id"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient := new(astro_mocks.Client)

	deploymentCreateInput := astro.CreateDeploymentInput{
		WorkspaceID:           ws,
		ClusterID:             csID,
		Label:                 "test-name",
		Description:           "test-desc",
		RuntimeReleaseVersion: "4.2.5",
		DagDeployEnabled:      false,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
			Scheduler: astro.Scheduler{
				AU:       10,
				Replicas: 3,
			},
		},
	}
	t.Run("success with Celery Executor", func(t *testing.T) {
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
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("success with enabling ci-cd enforcement", func(t *testing.T) {
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
		deploymentCreateInput.APIKeyOnlyDeployments = true
		defer func() { deploymentCreateInput.APIKeyOnlyDeployments = false }()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, "CeleryExecutor", "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &enableCiCdEnforcement)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("success with cloud provider and region", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
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
			DefaultSchedulerSize: astro.MachineUnit{
				Size: "small",
			},
		}, nil).Times(2)
		getSharedClusterParams := &astrocore.GetSharedClusterParams{
			Region:        region,
			CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(astrocore.SharedClusterCloudProviderGcp),
		}
		deploymentCreateInput1 := astro.CreateDeploymentInput{
			WorkspaceID:           ws,
			ClusterID:             csID,
			Label:                 "test-name",
			Description:           "test-desc",
			RuntimeReleaseVersion: "4.2.5",
			DagDeployEnabled:      false,
			IsHighAvailability:    true,
			SchedulerSize:         "small",
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: "KubernetesExecutor",
				Scheduler: astro.Scheduler{
					AU:       10,
					Replicas: 3,
				},
			},
		}
		defer func() {
			deploymentCreateInput1.DeploymentSpec.Executor = CeleryExecutor
		}()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput1).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err = Create("", ws, "test-desc", "", "4.2.5", dagDeploy, "KubernetesExecutor", "gcp", region, "small", "enable", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HYBRID")
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("select region", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
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
			DefaultSchedulerSize: astro.MachineUnit{
				Size: "small",
			},
		}, nil).Times(2)
		getSharedClusterParams := &astrocore.GetSharedClusterParams{
			Region:        region,
			CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(astrocore.SharedClusterCloudProviderGcp),
		}
		deploymentCreateInput1 := astro.CreateDeploymentInput{
			WorkspaceID:           ws,
			ClusterID:             csID,
			Label:                 "test-name",
			Description:           "test-desc",
			RuntimeReleaseVersion: "4.2.5",
			DagDeployEnabled:      false,
			IsHighAvailability:    true,
			SchedulerSize:         "small",
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: "KubernetesExecutor",
				Scheduler: astro.Scheduler{
					AU:       10,
					Replicas: 3,
				},
			},
		}
		defer func() {
			deploymentCreateInput1.DeploymentSpec.Executor = CeleryExecutor
		}()

		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput1).Return(astro.Deployment{ID: "test-id"}, nil).Once()

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

		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()
		defer testUtil.MockUserInput(t, "1")()

		err = Create("test-name", ws, "test-desc", "", "4.2.5", dagDeploy, "KubernetesExecutor", "gcp", "", "small", "enable", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HYBRID")
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("success with Kube Executor", func(t *testing.T) {
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
		deploymentCreateInput.DeploymentSpec.Executor = "KubeExecutor"
		defer func() { deploymentCreateInput.DeploymentSpec.Executor = CeleryExecutor }()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, "KubeExecutor", "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("success and wait for status", func(t *testing.T) {
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
		}, nil).Times(4)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Twice()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, clusterListParams).Return(&mockListClustersResponse, nil).Twice()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Twice()

		defer testUtil.MockUserInput(t, "test-name")()

		// setup wait for test
		sleepTime = 1
		tickNum = 2
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", Status: "UNHEALTHY"}}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", Status: "HEALTHY"}}, nil).Once()
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, true, &disableCiCdEnforcement)
		assert.NoError(t, err)

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{}}, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		// timeout
		timeoutNum = 1
		err = Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, true, &disableCiCdEnforcement)
		assert.ErrorIs(t, err, errTimedOut)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when creating a deployment fails", func(t *testing.T) {
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
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{}, errMock).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("failed to validate resources", func(t *testing.T) {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if cluster choice is not valid", func(t *testing.T) {
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
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, clusterListParams).Return(&astrocore.ListClustersResponse{}, errMock).Once()
		err := Create("test-name", ws, "test-desc", "invalid-cluster-id", "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
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
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 5, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
	})
	t.Run("list workspace failure", func(t *testing.T) {
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
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
	t.Run("invalid workspace failure", func(t *testing.T) {
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
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()

		err := Create("", "test-invalid-id", "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no workspaces with id")
		mockClient.AssertExpectations(t)
	})
	t.Run("success with hidden cluster information", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", org)

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
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		getSharedClusterParams := &astrocore.GetSharedClusterParams{
			Region:        region,
			CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(astrocore.SharedClusterCloudProviderGcp),
		}
		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err = Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "gcp", region, "", "", 10, 3, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HYBRID")
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("success with default config", func(t *testing.T) {
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
		deploymentCreateInput := astro.CreateDeploymentInput{
			WorkspaceID:           ws,
			ClusterID:             csID,
			Label:                 "test-name",
			Description:           "test-desc",
			RuntimeReleaseVersion: "4.2.5",
			DagDeployEnabled:      false,
			DeploymentSpec: astro.DeploymentCreateSpec{
				Executor: CeleryExecutor,
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 1,
				},
			},
		}
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(t, "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 0, 0, mockClient, mockCoreClient, false, &disableCiCdEnforcement)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestValidateResources(t *testing.T) {
	t.Run("invalid schedulerAU", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
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
		}, nil).Once()
		resp := validateResources(0, 3, astro.DeploymentConfig{
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
		})
		assert.False(t, resp)
	})

	t.Run("invalid runtime version", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()

		resp, err := validateRuntimeVersion("4.2.4", mockClient)
		assert.NoError(t, err)
		assert.False(t, resp)
		mockClient.AssertExpectations(t)
	})
}

func TestSelectCluster(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	csID := "test-cluster-id"
	t.Run("list cluster failure", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&astrocore.ListClustersResponse{}, errMock).Once()

		_, err := selectCluster("", mockOrgShortName, mockCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("cluster id via selection", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()

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

		resp, err := selectCluster("", mockOrgShortName, mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, csID, resp)
	})

	t.Run("cluster id invalid selection", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()

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

		_, err = selectCluster("", mockOrgShortName, mockCoreClient)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
	})

	t.Run("not able to find cluster", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()

		_, err := selectCluster("test-invalid-id", mockOrgShortName, mockCoreClient)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to find specified Cluster")
	})
}

func TestUpdate(t *testing.T) {
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
		expectedQueue := []astro.WorkerQueue{
			{
				Name:       "test-queue",
				IsDefault:  false,
				NodePoolID: "test-node-pool-id",
			},
		}
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

		err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, expectedQueue, false, nil, mockClient)
		assert.NoError(t, err)

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

		err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("successfully update schedulerSize and highAvailability", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
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
		expectedQueue := []astro.WorkerQueue{
			{
				Name:       "test-queue",
				IsDefault:  false,
				NodePoolID: "test-node-pool-id",
			},
		}
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

		err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "medium", "enable", 0, 0, expectedQueue, false, nil, mockClient)
		assert.NoError(t, err)

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

		err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "small", "disable", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("failed to validate resources", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		err := Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("success with hidden cluster information", func(t *testing.T) {
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

		err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, expectedQueue, false, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		err := Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
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

		err = Update("", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)

		err = Update("test-invalid-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.ErrorIs(t, err, errInvalidDeployment)
		mockClient.AssertExpectations(t)
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
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		err := Update("test-id", "test-label", ws, "test-description", "", "", CeleryExecutor, "", "", 10, 5, []astro.WorkerQueue{}, true, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, []astro.WorkerQueue{}, true, nil, mockClient)
		assert.ErrorIs(t, err, errMock)
		assert.NotContains(t, err.Error(), astro.AstronomerConnectionErrMsg)
		mockClient.AssertExpectations(t)
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

		err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		// force is false so we will confirm with the user
		defer testUtil.MockUserInput(t, "y")()
		err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.NoError(t, err)

		// force is false so we will confirm with the user
		defer testUtil.MockUserInput(t, "n")()
		err = Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.NoError(t, err)

		// force is true so no confirmation is needed
		err = Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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
		err := Update("test-id", "", ws, "", "", "", KubeExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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
		err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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
		err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, nil, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		err := Update("test-id", "", ws, "", "", "disable", "", "", "", 5, 3, []astro.WorkerQueue{}, true, nil, mockClient)

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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

		err = Delete("test-id", ws, "", false, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", false, mockClient)
		assert.ErrorIs(t, err, errMock)
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

		err = Delete("", ws, "", false, mockClient)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)

		err = Delete("test-invalid-id", ws, "", false, mockClient)
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

		err = Delete("test-id", ws, "", false, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("delete deployment failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", true, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}

func TestGetDeploymentURL(t *testing.T) {
	deploymentID := "deployment-id"
	workspaceID := "workspace-id"

	t.Run("returns deploymentURL for dev environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudDevPlatform)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for stage environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudStagePlatform)
		expectedURL := "cloud.astronomer-stage.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for perf environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPerfPlatform)
		expectedURL := "cloud.astronomer-perf.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for cloud (prod) environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedURL := "cloud.astronomer.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for pr preview environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns deploymentURL for local environment", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedURL := "localhost:5000/workspace-id/deployments/deployment-id/analytics"
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

func TestMutateExecutor(t *testing.T) {
	t.Run("returns true and updates executor from CE -> KE", func(t *testing.T) {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		expectedSpec := astro.DeploymentCreateSpec{
			Executor: KubeExecutor,
		}
		actual, actualSpec := mutateExecutor(KubeExecutor, existingSpec, 2)
		assert.True(t, actual) // we printed a warning
		assert.Equal(t, expectedSpec, actualSpec)
	})
	t.Run("returns true and updates executor from KE -> CE", func(t *testing.T) {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: KubeExecutor,
		}
		expectedSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		actual, actualSpec := mutateExecutor(CeleryExecutor, existingSpec, 2)
		assert.True(t, actual) // we printed a warning
		assert.Equal(t, expectedSpec, actualSpec)
	})
	t.Run("returns false and does not update executor if no executor change is requested", func(t *testing.T) {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		actual, actualSpec := mutateExecutor("", existingSpec, 0)
		assert.False(t, actual) // no warning was printed
		assert.Equal(t, existingSpec, actualSpec)
	})
	t.Run("returns false and updates executor if user does not confirms change", func(t *testing.T) {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		expectedSpec := astro.DeploymentCreateSpec{
			Executor: KubeExecutor,
		}
		actual, actualSpec := mutateExecutor(KubeExecutor, existingSpec, 1)
		assert.False(t, actual) // no warning was printed
		assert.Equal(t, expectedSpec, actualSpec)
	})
	t.Run("returns false and does not update executor if requested and existing executors are the same", func(t *testing.T) {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		actual, actualSpec := mutateExecutor(CeleryExecutor, existingSpec, 0)
		assert.False(t, actual) // no warning was printed
		assert.Equal(t, existingSpec, actualSpec)
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

func TestUseSharedCluster(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	csID := "test-cluster-id"
	region := "us-central1"
	getSharedClusterParams := &astrocore.GetSharedClusterParams{
		Region:        region,
		CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(astrocore.SharedClusterCloudProviderGcp),
	}
	t.Run("returns a cluster id", func(t *testing.T) {
		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()
		actual, err := useSharedCluster(astrocore.SharedClusterCloudProviderGcp, region, mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, csID, actual)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns a 404 error if a cluster is not found in the region", func(t *testing.T) {
		errorBody, _ := json.Marshal(astrocore.Error{
			Message: "Unable to find shared cluster",
		})
		mockErrorResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 404,
			},
			Body:    errorBody,
			JSON200: nil,
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockErrorResponse, nil).Once()
		_, err := useSharedCluster(astrocore.SharedClusterCloudProviderGcp, region, mockCoreClient)
		assert.Error(t, err)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if calling api fails", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(nil, errMock).Once()
		_, err := useSharedCluster(astrocore.SharedClusterCloudProviderGcp, region, mockCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestUseSharedClusterOrSelectCluster(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	csID := "test-cluster-id"

	t.Run("uses shared cluster if cloud provider and region are provided", func(t *testing.T) {
		cloudProvider := "gcp"
		region := "us-central1"
		getSharedClusterParams := &astrocore.GetSharedClusterParams{
			Region:        region,
			CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(cloudProvider),
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()
		actual, err := useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, "", "", mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, csID, actual)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns error if using shared cluster fails", func(t *testing.T) {
		cloudProvider := "gcp"
		region := "us-central1"
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, mock.Anything).Return(nil, errMock).Once()
		_, err := useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, "", "", mockCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("uses select cluster if cloud provider and region are not provided", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&mockListClustersResponse, nil).Once()
		defer testUtil.MockUserInput(t, "1")()
		actual, err := useSharedClusterOrSelectDedicatedCluster("", "", mockOrgShortName, "", mockCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, csID, actual)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns error if selecting cluster fails", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mockOrgShortName, clusterListParams).Return(&astrocore.ListClustersResponse{}, errMock).Once()
		_, err := useSharedClusterOrSelectDedicatedCluster("", "", mockOrgShortName, "", mockCoreClient)
		assert.ErrorIs(t, err, errMock)
		mockCoreClient.AssertExpectations(t)
	})
}
