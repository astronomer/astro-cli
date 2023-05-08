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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var errMock = errors.New("mock error")

const (
	org       = "test-org-id"
	ws        = "test-ws-id"
	dagDeploy = "disable"
	region    = "us-central1"
)

type Suite struct {
	suite.Suite
}

func TestCloudDeploymentSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestList() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")

		mockClient.AssertExpectations(s.T())
	})

	s.Run("success with all true", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, "").Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err := List("", true, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")

		mockClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockClient, buf)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("success with hidden cluster information", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", org)

		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err = List(ws, false, mockClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")
		s.Contains(buf.String(), "N/A")

		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeployment() {
	deploymentID := "test-id-wrong"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)

	s.Run("invalid deployment ID", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		_, err := GetDeployment(ws, deploymentID, "", mockClient, nil)
		s.ErrorIs(err, errInvalidDeployment)
		mockClient.AssertExpectations(s.T())
	})
	deploymentName := "test-wrong"
	deploymentID = "test-id"
	s.Run("error after invalid deployment Name", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{Label: "test", ID: "test-id"}}, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, mockClient, nil)
		s.ErrorIs(err, errInvalidDeployment)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("correct deployment ID", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, "", mockClient, nil)
		s.NoError(err)
		s.Equal(deploymentID, deployment.ID)
		mockClient.AssertExpectations(s.T())
	})
	deploymentName = "test"
	s.Run("correct deployment Name", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{Label: "test"}}, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient, nil)
		s.NoError(err)
		s.Equal(deploymentName, deployment.Label)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("two deployments with the same name", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{Label: "test", ID: "test-id"}, {Label: "test", ID: "test-id2"}}, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient, nil)
		s.NoError(err)
		s.Equal(deploymentName, deployment.Label)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("deployment name and deployment id", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, deploymentName, mockClient, nil)
		s.NoError(err)
		s.Equal(deploymentID, deployment.ID)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("bad deployment call", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		_, err := GetDeployment(ws, deploymentID, "", mockClient, nil)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("create deployment error", func() {
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
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool) error {
			return errMock
		}

		// mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = GetDeployment(ws, "", "", mockClient, nil)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("get deployments after creation error", func() {
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
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool) error {
			return nil
		}
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, errMock).Once()
		// mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = GetDeployment(ws, "", "", mockClient, nil)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("test automatic deployment creation", func() {
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
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool) error {
			return nil
		}

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		// mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		deployment, err := GetDeployment(ws, "", "", mockClient, nil)
		s.NoError(err)
		s.Equal(deploymentID, deployment.ID)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("failed to validate resources", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, nil).Once()
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		_, err := GetDeployment(ws, "", "", mockClient, nil)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSelectRegion() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	s.Run("list regions failure", func() {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(nil, errMock).Once()

		_, err := selectRegion("gcp", "", mockCoreClient)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("region via selection", func() {
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
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectRegion("gcp", "", mockCoreClient)
		s.NoError(err)
		s.Equal(region, resp)
	})

	s.Run("region invalid selection", func() {
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
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = selectRegion("gcp", "", mockCoreClient)
		s.ErrorIs(err, ErrInvalidRegionKey)
	})

	s.Run("not able to find region", func() {
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
		s.Error(err)
		s.Contains(err.Error(), "unable to find specified Region")
	})
}

func (s *Suite) TestLogs() {
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
	s.Run("success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{{Raw: "test log line"}}}, nil).Once()

		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockClient)
		s.NoError(err)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("success without deployment", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}, {ID: "test-id-1"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{}}, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Logs("", ws, "", false, false, false, logCount, mockClient)
		s.NoError(err)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{}, errMock).Once()

		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockClient)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCreate() {
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
	s.Run("success with Celery Executor", func() {
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
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("success with cloud provider and region", func() {
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
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

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err = Create("", ws, "test-desc", "", "4.2.5", dagDeploy, "KubernetesExecutor", "gcp", region, "small", "enable", 10, 3, mockClient, mockCoreClient, false)
		s.NoError(err)
		ctx.SetContextKey("organization_product", "HYBRID")
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("select region", func() {
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
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
		defer testUtil.MockUserInput(s.T(), "1")()

		err = Create("test-name", ws, "test-desc", "", "4.2.5", dagDeploy, "KubernetesExecutor", "gcp", "", "small", "enable", 10, 3, mockClient, mockCoreClient, false)
		s.NoError(err)
		ctx.SetContextKey("organization_product", "HYBRID")
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("success with Kube Executor", func() {
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
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, "KubeExecutor", "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("success and wait for status", func() {
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
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Twice()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Twice()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		// setup wait for test
		sleepTime = 1
		tickNum = 2
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", Status: "UNHEALTHY"}}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", Status: "HEALTHY"}}, nil).Once()
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, true)
		s.NoError(err)

		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{}}, nil).Once()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		// timeout
		timeoutNum = 1
		err = Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, true)
		s.ErrorIs(err, errTimedOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when creating a deployment fails", func() {
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
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{}, errMock).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("failed to validate resources", func() {
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if cluster choice is not valid", func() {
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
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{}, errMock).Once()
		err := Create("test-name", ws, "test-desc", "invalid-cluster-id", "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("invalid resources", func() {
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
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 5, mockClient, mockCoreClient, false)
		s.NoError(err)
	})
	s.Run("list workspace failure", func() {
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

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("invalid workspace failure", func() {
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

		err := Create("", "test-invalid-id", "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 10, 3, mockClient, mockCoreClient, false)
		s.Error(err)
		s.Contains(err.Error(), "no workspaces with id")
		mockClient.AssertExpectations(s.T())
	})
	s.Run("success with hidden cluster information", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
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

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err = Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "gcp", region, "", "", 10, 3, mockClient, mockCoreClient, false)
		s.NoError(err)
		ctx.SetContextKey("organization_product", "HYBRID")
		mockClient.AssertExpectations(s.T())
	})
	s.Run("success with default config", func() {
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
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Once()
		mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", 0, 0, mockClient, mockCoreClient, false)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestValidateResources() {
	s.Run("invalid schedulerAU", func() {
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
		s.False(resp)
	})

	s.Run("invalid runtime version", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()

		resp, err := validateRuntimeVersion("4.2.4", mockClient)
		s.NoError(err)
		s.False(resp)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSelectCluster() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	orgID := "test-org-id"
	csID := "test-cluster-id"
	s.Run("list cluster failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errMock).Once()

		_, err := selectCluster("", orgID, mockClient)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("cluster id via selection", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectCluster("", orgID, mockClient)
		s.NoError(err)
		s.Equal(csID, resp)
	})

	s.Run("cluster id invalid selection", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()

		// mock os.Stdin
		input := []byte("4")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = selectCluster("", orgID, mockClient)
		s.ErrorIs(err, ErrInvalidDeploymentKey)
	})

	s.Run("not able to find cluster", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()

		_, err := selectCluster("test-invalid-id", orgID, mockClient)
		s.Error(err)
		s.Contains(err.Error(), "unable to find specified Cluster")
	})
}

func (s *Suite) TestUpdate() {
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

	s.Run("success", func() {
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
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, expectedQueue, false, mockClient)
		s.NoError(err)

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

		err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("successfully update schedulerSize and highAvailability", func() {
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
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
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "medium", "enable", 0, 0, expectedQueue, false, mockClient)
		s.NoError(err)

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

		err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "small", "disable", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("failed to validate resources", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		err := Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("success with hidden cluster information", func() {
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
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
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, expectedQueue, false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list deployments failure", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		err := Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid deployment id", func() {
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}, {ID: "test-id-2", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Twice()

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Update("", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.ErrorIs(err, ErrInvalidDeploymentKey)

		err = Update("test-invalid-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.ErrorIs(err, errInvalidDeployment)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid resources", func() {
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
		err := Update("test-id", "test-label", ws, "test-description", "", "", CeleryExecutor, "", "", 10, 5, []astro.WorkerQueue{}, true, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("cancel update", func() {
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
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Update("test-id", "test-label", ws, "test description", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("update deployment failure", func() {
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

		err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 0, 0, []astro.WorkerQueue{}, true, mockClient)
		s.ErrorIs(err, errMock)
		s.NotContains(err.Error(), astro.AstronomerConnectionErrMsg)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("update deployment to enable dag deploy", func() {
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

		err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)

		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("do not update deployment to enable dag deploy if already enabled", func() {
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
		err := Update("test-id", "", ws, "", "", "enable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)

		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("update deployment to disable dag deploy", func() {
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
		defer testUtil.MockUserInput(s.T(), "y")()
		err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.NoError(err)

		// force is false so we will confirm with the user
		defer testUtil.MockUserInput(s.T(), "n")()
		err = Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.NoError(err)

		// force is true so no confirmation is needed
		err = Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("do not update deployment to disable dag deploy if already disabled", func() {
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
		err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)

		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("update deployment to change executor to KubernetesExecutor", func() {
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

		defer testUtil.MockUserInput(s.T(), "y")()
		err := Update("test-id", "", ws, "", "", "", KubeExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("update deployment to change executor to CeleryExecutor", func() {
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

		defer testUtil.MockUserInput(s.T(), "y")()
		err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("do not update deployment if user says no to the executor change", func() {
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
		defer testUtil.MockUserInput(s.T(), "n")()
		err := Update("test-id", "", ws, "", "", "", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("do not update deployment with executor if deployment already has it", func() {
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
		err := Update("test-id", "", ws, "", "", "disable", CeleryExecutor, "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)

		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("do not update deployment with executor if user did not request it", func() {
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
		err := Update("test-id", "", ws, "", "", "disable", "", "", "", 5, 3, []astro.WorkerQueue{}, true, mockClient)

		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	s.Run("success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()

		// mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Delete("test-id", ws, "", false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list deployments failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", false, mockClient)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("invalid deployment id", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}, {ID: "test-id-2", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Twice()

		// mock os.Stdin
		input := []byte("0")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Delete("", ws, "", false, mockClient)
		s.ErrorIs(err, ErrInvalidDeploymentKey)

		err = Delete("test-invalid-id", ws, "", false, mockClient)
		s.ErrorIs(err, errInvalidDeployment)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("cancel update", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Once()

		// mock os.Stdin
		input := []byte("n")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = Delete("test-id", ws, "", false, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("delete deployment failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", org, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", true, mockClient)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeploymentURL() {
	deploymentID := "deployment-id"
	workspaceID := "workspace-id"

	s.Run("returns deploymentURL for dev environment", func() {
		testUtil.InitTestConfig(testUtil.CloudDevPlatform)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for stage environment", func() {
		testUtil.InitTestConfig(testUtil.CloudStagePlatform)
		expectedURL := "cloud.astronomer-stage.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for perf environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPerfPlatform)
		expectedURL := "cloud.astronomer-perf.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for cloud (prod) environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedURL := "cloud.astronomer.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for pr preview environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for local environment", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedURL := "localhost:5000/workspace-id/deployments/deployment-id/analytics"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns an error if getting current context fails", func() {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		expectedURL := ""
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.ErrorContains(err, "no context set")
		s.Equal(expectedURL, actualURL)
	})
}

func (s *Suite) TestMutateExecutor() {
	s.Run("returns true and updates executor from CE -> KE", func() {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		expectedSpec := astro.DeploymentCreateSpec{
			Executor: KubeExecutor,
		}
		actual, actualSpec := mutateExecutor(KubeExecutor, existingSpec, 2)
		s.True(actual) // we printed a warning
		s.Equal(expectedSpec, actualSpec)
	})
	s.Run("returns true and updates executor from KE -> CE", func() {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: KubeExecutor,
		}
		expectedSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		actual, actualSpec := mutateExecutor(CeleryExecutor, existingSpec, 2)
		s.True(actual) // we printed a warning
		s.Equal(expectedSpec, actualSpec)
	})
	s.Run("returns false and does not update executor if no executor change is requested", func() {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		actual, actualSpec := mutateExecutor("", existingSpec, 0)
		s.False(actual) // no warning was printed
		s.Equal(existingSpec, actualSpec)
	})
	s.Run("returns false and updates executor if user does not confirms change", func() {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		expectedSpec := astro.DeploymentCreateSpec{
			Executor: KubeExecutor,
		}
		actual, actualSpec := mutateExecutor(KubeExecutor, existingSpec, 1)
		s.False(actual) // no warning was printed
		s.Equal(expectedSpec, actualSpec)
	})
	s.Run("returns false and does not update executor if requested and existing executors are the same", func() {
		existingSpec := astro.DeploymentCreateSpec{
			Executor: CeleryExecutor,
		}
		actual, actualSpec := mutateExecutor(CeleryExecutor, existingSpec, 0)
		s.False(actual) // no warning was printed
		s.Equal(existingSpec, actualSpec)
	})
}

func (s *Suite) TestPrintWarning() {
	s.Run("when KubernetesExecutor is requested", func() {
		s.Run("returns true > 1 queues exist", func() {
			actual := printWarning(KubeExecutor, 3)
			s.True(actual)
		})
		s.Run("returns false if only 1 queue exists", func() {
			actual := printWarning(KubeExecutor, 1)
			s.False(actual)
		})
	})
	s.Run("returns true when CeleryExecutor is requested", func() {
		actual := printWarning(CeleryExecutor, 2)
		s.True(actual)
	})
	s.Run("returns false for any other executor is requested", func() {
		actual := printWarning("non-existent", 2)
		s.False(actual)
	})
}

func (s *Suite) TestUseSharedCluster() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	csID := "test-cluster-id"
	region := "us-central1"
	getSharedClusterParams := &astrocore.GetSharedClusterParams{
		Region:        region,
		CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(astrocore.SharedClusterCloudProviderGcp),
	}
	s.Run("returns a cluster id", func() {
		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(mockOKResponse, nil).Once()
		actual, err := useSharedCluster(astrocore.SharedClusterCloudProviderGcp, region, mockCoreClient)
		s.NoError(err)
		s.Equal(csID, actual)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns a 404 error if a cluster is not found in the region", func() {
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
		s.Error(err)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if calling api fails", func() {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, getSharedClusterParams).Return(nil, errMock).Once()
		_, err := useSharedCluster(astrocore.SharedClusterCloudProviderGcp, region, mockCoreClient)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUseSharedClusterOrSelectCluster() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	orgID := "test-org-id"
	csID := "test-cluster-id"

	s.Run("uses shared cluster if cloud provider and region are provided", func() {
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
		actual, err := useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, "", "", nil, mockCoreClient)
		s.NoError(err)
		s.Equal(csID, actual)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns error if using shared cluster fails", func() {
		cloudProvider := "gcp"
		region := "us-central1"
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, mock.Anything).Return(nil, errMock).Once()
		_, err := useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, "", "", nil, mockCoreClient)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("uses select cluster if cloud provider and region are not provided", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()
		defer testUtil.MockUserInput(s.T(), "1")()
		actual, err := useSharedClusterOrSelectDedicatedCluster("", "", orgID, "", mockClient, nil)
		s.NoError(err)
		s.Equal(csID, actual)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns error if selecting cluster fails", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errMock).Once()
		_, err := useSharedClusterOrSelectDedicatedCluster("", "", orgID, "", mockClient, nil)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})
}
