package deployment

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errMock = errors.New("mock error")

func TestList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ws := "test-ws-id"
	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockClient, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "test-id-1")
		assert.Contains(t, buf.String(), "test-id-2")

		mockClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, errMock).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}

func TestGetDeployment(t *testing.T) {
	ws := "test-ws-id"
	deploymentID := "test-id-wrong"
	t.Run("invalid deployment ID", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		_, err := GetDeployment(ws, deploymentID, "", mockClient)
		assert.ErrorIs(t, err, errInvalidDeployment)
		mockClient.AssertExpectations(t)
	})
	deploymentName := "test-wrong"
	deploymentID = "test-id"
	t.Run("auto select after invalid deployment Name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{Label: "test", ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})

	t.Run("correct deployment ID", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})
	deploymentName = "test"
	t.Run("correct deployment Name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{Label: "test"}}, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentName, deployment.Label)
		mockClient.AssertExpectations(t)
	})

	t.Run("two deployments with the same name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{Label: "test", ID: "test-id"}, {Label: "test", ID: "test-id2"}}, nil).Once()

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

		deployment, err := GetDeployment(ws, "", deploymentName, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentName, deployment.Label)
		mockClient.AssertExpectations(t)
	})

	t.Run("deployment name and deployment id", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, deploymentName, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})

	t.Run("bad deployment call", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, errMock).Once()

		_, err := GetDeployment(ws, deploymentID, "", mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("create deployment error", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, nil).Once()

		// mock createDeployment
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion string, schedulerAU, schedulerReplicas, workerAU int, client astro.Client, waitForStatus bool) error {
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

		_, err = GetDeployment(ws, "", "", mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("get deployments after creation error", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, nil).Once()

		// mock createDeployment
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion string, schedulerAU, schedulerReplicas, workerAU int, client astro.Client, waitForStatus bool) error {
			return nil
		}
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, errMock).Once()
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

		_, err = GetDeployment(ws, "", "", mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("test automatic deployment creation", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, nil).Once()
		// mock createDeployment
		createDeployment = func(label, workspaceID, description, clusterID, runtimeVersion string, schedulerAU, schedulerReplicas, workerAU int, client astro.Client, waitForStatus bool) error {
			return nil
		}
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
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

		deployment, err := GetDeployment(ws, "", "", mockClient)
		assert.NoError(t, err)
		assert.Equal(t, deploymentID, deployment.ID)
		mockClient.AssertExpectations(t)
	})
}

func TestLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ws := "test-ws-id"
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
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{{Raw: "test log line"}}}, nil).Once()

		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockClient)
		assert.NoError(t, err)

		mockClient.AssertExpectations(t)
	})

	t.Run("success without deployment", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}, {ID: "test-id-1"}}, nil).Once()
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
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()
		mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{}, errMock).Once()

		err := Logs(deploymentID, ws, "", true, true, true, logCount, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ws := "test-ws-id"
	csID := "test-cluster-id"

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id"}}, nil).Once()

		// mock os.Stdin
		input := []byte("test-name")
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

		err = Create("", ws, "test-desc", csID, "4.2.5", 10, 3, 15, mockClient, false)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("success and wait for status", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Twice()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Twice()
		mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Twice()
		mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Twice()

		// mock os.Stdin
		input := []byte("test-name")
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

		// setup wait for test
		sleepTime = 1
		tickNum = 2
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id", Status: "UNHEALTHY"}}, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id", Status: "HEALTHY"}}, nil).Once()
		err = Create("", ws, "test-desc", csID, "4.2.5", 10, 3, 15, mockClient, true)
		assert.NoError(t, err)

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{}}, nil).Once()

		// mock os.Stdin
		input = []byte("test-name")
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

		// timeout
		timeoutNum = 1
		err = Create("", ws, "test-desc", csID, "4.2.5", 10, 3, 15, mockClient, true)
		assert.ErrorIs(t, err, errTimedOut)
		mockClient.AssertExpectations(t)
	})

	t.Run("failed to validate resources", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errMock).Once()

		err := Create("", ws, "test-desc", csID, "4.2.5", 10, 3, 15, mockClient, false)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid resources", func(t *testing.T) {
		err := Create("", ws, "test-desc", csID, "4.2.5", 10, 5, 15, nil, false)
		assert.NoError(t, err)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()

		err := Create("", ws, "test-desc", csID, "4.2.5", 10, 3, 15, mockClient, false)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()

		err := Create("", "test-invalid-id", "test-desc", csID, "4.2.5", 10, 3, 15, mockClient, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no workspaces with id")
		mockClient.AssertExpectations(t)
	})
}

func TestValidateResources(t *testing.T) {
	t.Run("invalid workerAU", func(t *testing.T) {
		resp := validateResources(0, 3, 3)
		assert.False(t, resp)
	})

	t.Run("invalid schedulerAU", func(t *testing.T) {
		resp := validateResources(10, 0, 3)
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

	orgID := "test-org-id"
	csID := "test-cluster-id"
	t.Run("list cluster failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{}, errMock).Once()

		_, err := selectCluster("", orgID, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("cluster id via selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()

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

		resp, err := selectCluster("", orgID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, csID, resp)
	})

	t.Run("cluster id invalid selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()

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

		_, err = selectCluster("", orgID, mockClient)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)
	})

	t.Run("not able to find cluster", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListClusters", orgID).Return([]astro.Cluster{{ID: csID}}, nil).Once()

		_, err := selectCluster("test-invalid-id", orgID, mockClient)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to find specified Cluster")
	})
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ws := "test-ws-id"
	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Workers: astro.Workers{AU: 10}, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{deploymentResp}, nil).Twice()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Twice()

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

		err = Update("test-id", "", ws, "", "", 0, 0, 0, []astro.WorkerQueue{}, false, mockClient)
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

		err = Update("test-id", "test-label", ws, "test description", "", 5, 3, 10, []astro.WorkerQueue{}, false, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("list deployments failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, errMock).Once()

		err := Update("test-id", "test-label", ws, "test description", "", 5, 3, 10, []astro.WorkerQueue{}, false, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid deployment id", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}, {ID: "test-id-2", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Twice()

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

		err = Update("", "test-label", ws, "test description", "", 5, 3, 10, []astro.WorkerQueue{}, false, mockClient)
		assert.ErrorIs(t, err, ErrInvalidDeploymentKey)

		err = Update("test-invalid-id", "test-label", ws, "test description", "", 5, 3, 10, []astro.WorkerQueue{}, false, mockClient)
		assert.ErrorIs(t, err, errInvalidDeployment)
		mockClient.AssertExpectations(t)
	})

	t.Run("cancel update", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Once()

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

		err = Update("test-id", "test-label", ws, "test description", "", 5, 3, 10, []astro.WorkerQueue{}, false, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("update deployment failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errMock).Once()

		err := Update("test-id", "", ws, "", "", 0, 0, 0, []astro.WorkerQueue{}, true, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ws := "test-ws-id"
	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Workers: astro.Workers{AU: 10}, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{deploymentResp}, nil).Once()
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
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", false, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("invalid deployment id", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}, {ID: "test-id-2", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Twice()

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
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{{ID: "test-id", RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"}}}, nil).Once()

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
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{deploymentResp}, nil).Once()
		mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{}, errMock).Once()

		err := Delete("test-id", ws, "", true, mockClient)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})
}
