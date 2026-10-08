package deployment

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	errMock                 = errors.New("api error")
	errGetDeploymentMock    = errors.New("get deployment error")
	errUpdateDeploymentMock = errors.New("update deployment error")
	errRegMock              = errors.New("error")
)

func (s *Suite) TestGetDeployments() {
	// Create a mock Houston client
	mockClient := &mocks.ClientInterface{}

	// Define the expected response from the ListDeployments function
	expectedDeployments := []houston.Deployment{
		{ID: "123"},
		{ID: "456"},
	}

	// Set up the mock client to return the expected response
	mockClient.On("ListDeployments", mock.Anything).Return(expectedDeployments, nil)

	// Call the GetDeployments function with the mock client
	deployments, err := GetDeployments("workspace", mockClient)

	// Assert that the returned deployments match the expected deployments
	if !reflect.DeepEqual(deployments, expectedDeployments) {
		s.Fail("Expected deployments to be %v, but got %v", expectedDeployments, deployments)
	}

	// Assert that there was no error returned
	if err != nil {
		s.Fail("Expected no error, but got %v", err)
	}

	// Assert that the ListDeployments function was called with the correct parameters
	expectedRequest := houston.ListDeploymentsRequest{WorkspaceID: "workspace"}
	mockClient.AssertCalled(s.T(), "ListDeployments", expectedRequest)

	mockClientErr := &mocks.ClientInterface{}
	mockClientErr.On("ListDeployments", mock.Anything).Return(nil, errMock)
	_, err = GetDeployments("workspace", mockClientErr)
	s.EqualError(err, GetDeploymentsErr(errMock).Error())
}

func (s *Suite) TestSelectDeployment() {
	s.Run("no deployments", func() {
		deployments := []houston.Deployment{}
		message := "Choose a deployment:"
		dep, err := SelectDeployment(deployments, message)
		s.Equal(houston.Deployment{}, dep)
		s.NoError(err)
	})
	s.Run("One deployment", func() {
		createdAt := time.Now().Add(-1 * time.Hour)
		deployments := []houston.Deployment{
			{
				ID:          "123",
				Label:       "deployment-1",
				ReleaseName: "release-1",
				CreatedAt:   createdAt,
			},
		}
		message := "Choose a deployment:"
		dep, err := SelectDeployment(deployments, message)
		s.Equal(deployments[0], dep)
		s.NoError(err)
	})
	//
	s.Run("Multiple deployments", func() {
		createdAt := time.Now().Add(-1 * time.Hour)
		deployments := []houston.Deployment{
			{
				ID:          "123",
				Label:       "deployment-1",
				ReleaseName: "release-1",
				CreatedAt:   createdAt,
			},
			{
				ID:          "456",
				Label:       "deployment-2",
				ReleaseName: "release-2",
				CreatedAt:   createdAt.Add(-1 * time.Minute),
			},
			{
				ID:          "789",
				Label:       "deployment-3",
				ReleaseName: "release-3",
				CreatedAt:   createdAt.Add(-2 * time.Minute),
			},
		}
		message := "Choose a deployment:"
		testUtil.MockUserInput(s.T(), "2\n")
		dep, err := SelectDeployment(deployments, message)
		s.Equal(deployments[1], dep)
		s.NoError(err)
	})
	//
	s.Run("Invalid choice", func() {
		createdAt := time.Now().Add(-1 * time.Hour)
		deployments := []houston.Deployment{
			{
				ID:          "123",
				Label:       "deployment-1",
				ReleaseName: "release-1",
				CreatedAt:   createdAt,
			},
			{
				ID:          "456",
				Label:       "deployment-2",
				ReleaseName: "release-2",
				CreatedAt:   createdAt.Add(-1 * time.Minute),
			},
			{
				ID:          "789",
				Label:       "deployment-3",
				ReleaseName: "release-3",
				CreatedAt:   createdAt.Add(-2 * time.Minute),
			},
		}
		message := "Choose a deployment:"
		testUtil.MockUserInput(s.T(), "4\n")

		dep, err := SelectDeployment(deployments, message)
		s.Equal(houston.Deployment{}, dep)
		s.Equal(ErrInvalidDeploymentKey, err)
	})
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockAppConfig := &houston.AppConfig{
		Version:              "0.15.1",
		BaseDomain:           "local.astronomer.io",
		SMTPConfigured:       true,
		ManualReleaseNames:   false,
		HardDeleteDeployment: true,
		ManualNamespaceNames: false,
	}
	mockDeployment := &houston.Deployment{
		ID:             "ckbv818oa00r107606ywhoqtw",
		Type:           "airflow",
		Label:          "test2",
		ReleaseName:    "boreal-penumbra-1102",
		Version:        "0.0.0",
		AirflowVersion: "1.10.5",
		DeploymentInfo: houston.DeploymentInfo{},
		Workspace:      houston.Workspace{},
		Urls: []houston.DeploymentURL{
			{
				Type: "airflow",
				URL:  "http://airflow.com",
			},
			{
				Type: "flower",
				URL:  "http://flower.com",
			},
		},
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}

	label := "label"
	ws := "ck1qg6whg001r08691y117hub"
	releaseName := ""
	role := "test-role"
	executor := houston.CeleryExecutorType
	airflowVersion := "1.10.5"
	runtimeVersion := "5.0.1"
	dagDeploymentType := houston.ImageDeploymentType
	nfsLocation := ""
	triggerReplicas := 0
	clusterID := "testClusterID"
	req := &CreateDeploymentRequest{label, ws, releaseName, role, executor, airflowVersion, "", dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, clusterID, "", ""}

	s.Run("create success. Cluster is not passed in the payload", func() {
		req.ClusterID = ""
		api := new(mocks.ClientInterface)
		// Have to use mock anything for now as vars is too big
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
		req.ClusterID = clusterID
	})

	s.Run("create success. Cluster is passed in the payload", func() {
		api := new(mocks.ClientInterface)
		// Have to use mock anything for now as vars is too big
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
	})

	s.Run("create success. Mode is passed in the payload when set", func() {
		req.Mode = houston.OperatorDeploymentMode
		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool {
			return vars["mode"] == houston.OperatorDeploymentMode
		})).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		_, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		api.AssertExpectations(s.T())
		req.Mode = ""
	})

	s.Run("create success. Mode is omitted from the payload when empty", func() {
		req.Mode = ""
		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool {
			_, ok := vars["mode"]
			return !ok
		})).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		_, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		api.AssertExpectations(s.T())
	})

	s.Run("create trigger enabled", func() {
		mockAppConfig.TriggererEnabled = true

		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		triggerReplicas = 1
		buf := new(bytes.Buffer)
		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
	})

	s.Run("create trigger enabled with trigger replicas count -1", func() {
		mockAppConfig.TriggererEnabled = true

		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		triggerReplicas = -1
		buf := new(bytes.Buffer)
		req = &CreateDeploymentRequest{label, ws, releaseName, role, executor, airflowVersion, "", dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas, clusterID, "", ""}
		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
	})

	s.Run("create nfslocation enabled", func() {
		mockAppConfig.TriggererEnabled = false

		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		nfsLocation = "test:/test"
		triggerReplicas = 0

		buf := new(bytes.Buffer)
		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
	})

	s.Run("create git sync enabled", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		nfsLocation = ""
		dagDeploymentType = houston.GitSyncDeploymentType

		myTests := []*struct {
			repoURL              string
			revision             string
			dagDirectoryLocation string
			branchName           string
			syncInterval         int
			sshKey               string
			knownHosts           string
			expectedOutput       string
			expectedError        string
		}{
			{repoURL: "https://github.com/bote795/public-ariflow-dags-test.git", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../../../cmd/apc/testfiles/ssh_key", knownHosts: "../../../../cmd/apc/testfiles/known_hosts", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../../../cmd/apc/testfiles/ssh_key", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../../../cmd/apc/testfiles/wrong_ssh_key", knownHosts: "../../../../cmd/apc/testfiles/known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "wrong path specified, no file exists for ssh key"},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../../../cmd/apc/testfiles/ssh_key", knownHosts: "../../../../cmd/apc/testfiles/wrong_known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "wrong path specified, no file exists for known hosts"},
			{repoURL: "https://gitlab.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../../../cmd/apc/testfiles/ssh_key", knownHosts: "../../../../cmd/apc/testfiles/known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "git repository host not present in known hosts file"},
		}

		for _, tt := range myTests {
			buf := new(bytes.Buffer)
			createReq := &CreateDeploymentRequest{label, ws, releaseName, role, executor, "", runtimeVersion, dagDeploymentType, "", tt.repoURL, tt.revision, tt.branchName, tt.dagDirectoryLocation, tt.sshKey, tt.knownHosts, tt.syncInterval, triggerReplicas, clusterID, "", ""}
			_, err := Create(createReq, api, buf, mockAppConfig)
			if tt.expectedError != "" {
				s.EqualError(err, tt.expectedError)
			} else {
				s.NoError(err)
			}
		}
	})

	s.Run("create with pre-create namespace deployment success", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		releaseName = ""
		dagDeploymentType = houston.VolumeDeploymentType
		nfsLocation = "test:/test"

		buf := new(bytes.Buffer)

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

		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
	})

	s.Run("create pre-create namespace deployment error", func() {
		appConfig := *mockAppConfig
		appConfig.Flags = houston.FeatureFlags{
			ManualNamespaceNames: true,
		}
		mockNamespaces := []houston.Namespace{
			{Name: "test1"},
			{Name: "test2"},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return(mockNamespaces, nil)

		buf := new(bytes.Buffer)

		// mock os.Stdin
		input := []byte("5")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = Create(req, api, buf, &appConfig)
		s.EqualError(err, "number is out of available range")
		api.AssertExpectations(s.T())
	})

	s.Run("create get namespaces error", func() {
		appConfig := *mockAppConfig
		appConfig.Flags = houston.FeatureFlags{
			ManualNamespaceNames: true,
		}

		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return([]houston.Namespace{}, errMock)

		buf := new(bytes.Buffer)
		_, err := Create(req, api, buf, &appConfig)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("create api error", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(nil, errMock)

		buf := new(bytes.Buffer)
		_, err := Create(req, api, buf, mockAppConfig)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("create free form namespace success", func() {
		mockAppConfig.Flags.NamespaceFreeFormEntry = true

		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("test1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		created, err := Create(req, api, buf, mockAppConfig)
		s.NoError(err)
		s.Equal(mockDeployment, created)
		api.AssertExpectations(s.T())
	})
	s.Run("create free form namespace error", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeployment", mock.Anything).Return(nil, errMock)

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("    ")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = Create(req, api, buf, mockAppConfig)
		s.EqualError(err, "no kubernetes namespaces specified")
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockDeployment := &houston.Deployment{
		ID:             "ckbv818oa00r107606ywhoqtw",
		Type:           "airflow",
		Label:          "test",
		ReleaseName:    "prehistoric-gravity312",
		Version:        "1.1.0",
		AirflowVersion: "1.1.0",
	}

	s.Run("delete success", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: false}).Return(mockDeployment, nil)

		deleted, err := Delete(mockDeployment.ID, false, api)
		s.NoError(err)
		s.Equal(mockDeployment, deleted)
		api.AssertExpectations(s.T())
	})

	s.Run("delete api error", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: false}).Return(nil, errMock)

		_, err := Delete(mockDeployment.ID, false, api)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("delete hard success", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: true}).Return(mockDeployment, nil)

		deleted, err := Delete(mockDeployment.ID, true, api)
		s.NoError(err)
		s.Equal(mockDeployment, deleted)
	})
}

func (s *Suite) TestAdopt() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockDeployment := &houston.Deployment{
		ID:          "ckbv818oa00r107606ywhoqtw",
		Label:       "prod-airflow-4",
		ReleaseName: "prod-airflow-4",
		Namespace:   "airflow-prod4",
		ClusterID:   "cluster-test-id",
	}
	req := &houston.AdoptDeploymentRequest{
		WorkspaceID:             "workspace-test-id",
		ClusterID:               "cluster-test-id",
		CRNamespace:             "airflow-prod4",
		CRName:                  "prod-airflow-4",
		AcceptIncompatibilities: true,
	}

	s.Run("adopt success", func() {
		api := new(mocks.ClientInterface)
		api.On("AdoptDeployment", req).Return(mockDeployment, nil)

		adopted, err := Adopt(req, api)
		s.NoError(err)
		s.Equal(mockDeployment, adopted)
		api.AssertExpectations(s.T())
	})

	s.Run("adopt api error", func() {
		api := new(mocks.ClientInterface)
		api.On("AdoptDeployment", req).Return(nil, errMock)

		_, err := Adopt(req, api)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUnadopt() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockDeployment := &houston.Deployment{
		ID:          "ckbv818oa00r107606ywhoqtw",
		Label:       "prod-airflow-4",
		ReleaseName: "prod-airflow-4",
	}

	s.Run("unadopt success", func() {
		api := new(mocks.ClientInterface)
		api.On("UnadoptDeployment", houston.UnadoptDeploymentRequest{DeploymentID: mockDeployment.ID}).Return(mockDeployment, nil)

		unadopted, err := Unadopt(mockDeployment.ID, api)
		s.NoError(err)
		s.Equal(mockDeployment, unadopted)
		api.AssertExpectations(s.T())
	})

	s.Run("unadopt api error", func() {
		api := new(mocks.ClientInterface)
		api.On("UnadoptDeployment", houston.UnadoptDeploymentRequest{DeploymentID: mockDeployment.ID}).Return(nil, errMock)

		_, err := Unadopt(mockDeployment.ID, api)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestList() {
	clusterID := "testClusterID"
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockDeployments := []houston.Deployment{
		{
			ID:                    "ckbv801t300qh0760pck7ea0c",
			Type:                  "airflow",
			Label:                 "test",
			ReleaseName:           "burning-terrestrial-5940",
			Version:               "1.1.0",
			AirflowVersion:        "1.1.0",
			DesiredAirflowVersion: "1.1.0",
			Workspace: houston.Workspace{
				ID:    "ckbv818oa00r107606ywhoqtw",
				Label: "w1",
			},
		},
	}

	expectedRequest := houston.ListDeploymentsRequest{
		WorkspaceID: mockDeployments[0].Workspace.ID,
	}

	s.Run("list deployments for workspace success", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeployments", expectedRequest).Return(mockDeployments, nil)

		got, err := List(mockDeployments[0].Workspace.ID, false, api, clusterID)
		s.NoError(err)
		s.Equal(mockDeployments, got)
		api.AssertExpectations(s.T())
	})

	s.Run("ordered by label, last first", func() {
		a, b := mockDeployments[0], mockDeployments[0]
		a.Label, b.Label = "alpha", "beta"
		api := new(mocks.ClientInterface)
		api.On("ListDeployments", expectedRequest).Return([]houston.Deployment{a, b}, nil)

		got, err := List(mockDeployments[0].Workspace.ID, false, api, clusterID)
		s.NoError(err)
		s.Equal([]houston.Deployment{b, a}, got)
	})

	s.Run("list namespace api error", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeployments", expectedRequest).Return([]houston.Deployment{}, errMock)

		_, err := List(mockDeployments[0].Workspace.ID, false, api, clusterID)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("list namespace all enabled", func() {
		expectedRequest := houston.PaginatedDeploymentsRequest{
			Take:      -1,
			ClusterID: clusterID,
		}

		api := new(mocks.ClientInterface)
		api.On("ListPaginatedDeployments", expectedRequest).Return(mockDeployments, nil)

		got, err := List(mockDeployments[0].Workspace.ID, true, api, clusterID)
		s.NoError(err)
		s.Equal(mockDeployments, got)
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockAppConfig := &houston.AppConfig{
		Version:                "0.15.1",
		BaseDomain:             "local.astronomer.io",
		SMTPConfigured:         true,
		ManualReleaseNames:     false,
		ConfigureDagDeployment: false,
		NfsMountDagDeployment:  false,
		HardDeleteDeployment:   true,
		ManualNamespaceNames:   true,
		TriggererEnabled:       false,
		Flags:                  houston.FeatureFlags{},
	}
	mockDeployment := &houston.Deployment{
		ID:             "ckbv801t300qh0760pck7ea0c",
		Type:           "airflow",
		Label:          "test123",
		ReleaseName:    "burning-terrestrial-5940",
		Version:        "0.0.0",
		AirflowVersion: "2.2.2",
		Urls: []houston.DeploymentURL{
			{
				Type: "airflow",
				URL:  "http://airflow.com",
			},
			{
				Type: "flower",
				URL:  "http://flower.com",
			},
		},
		DeploymentInfo: houston.DeploymentInfo{
			Current: "2.2.2-1",
		},
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}

	role := "test-role"

	s.Run("update success", func() {
		api := new(mocks.ClientInterface)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

		for _, dagDeploymentType := range []string{"", houston.ImageDeploymentType} {
			got, err := Update(mockDeployment.ID, role, map[string]string{"executor": houston.CeleryExecutorType}, dagDeploymentType, "", "", "", "", "", "", "", "", 1, 0, api, mockAppConfig)
			s.NoError(err)
			s.Equal(mockDeployment, got)
			api.AssertExpectations(s.T())
		}
	})

	s.Run("update triggerer enabled", func() {
		mockAppConfig.TriggererEnabled = true

		api := new(mocks.ClientInterface)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

		for _, dagDeploymentType := range []string{"", houston.ImageDeploymentType} {
			got, err := Update(mockDeployment.ID, role, map[string]string{"executor": houston.CeleryExecutorType}, dagDeploymentType, "", "", "", "", "", "", "", "", 1, 1, api, mockAppConfig)
			s.NoError(err)
			s.Equal(mockDeployment, got)
			api.AssertExpectations(s.T())
		}
	})

	s.Run("update error", func() {
		api := new(mocks.ClientInterface)
		api.On("UpdateDeployment", mock.Anything).Return(nil, errMock)

		deploymentConfig := make(map[string]string)
		deploymentConfig["executor"] = houston.CeleryExecutorType

		_, err := Update(mockDeployment.ID, role, deploymentConfig, "", "", "", "", "", "", "", "", "", 1, 0, api, mockAppConfig)

		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestAirflowUpgrade() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeployment := &houston.Deployment{
		ID:                    "ckbv818oa00r107606ywhoqtw",
		Type:                  "airflow",
		Label:                 "test123",
		ReleaseName:           "burning-terrestrial-5940",
		Version:               "0.0.0",
		AirflowVersion:        "1.10.5",
		DesiredAirflowVersion: "1.10.10",
	}

	s.Run("upgrade airflow success", func() {
		expectedVars := map[string]interface{}{"deploymentId": mockDeployment.ID, "desiredAirflowVersion": mockDeployment.DesiredAirflowVersion}

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(mockDeployment, nil)
		buf := new(bytes.Buffer)
		got, err := AirflowUpgrade(mockDeployment.ID, mockDeployment.DesiredAirflowVersion, api, buf)
		s.NoError(err)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "1.10.5"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageCertified, Version: "1.10.10"}, got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade airflow get deployment error", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, errGetDeploymentMock)

		buf := new(bytes.Buffer)
		_, err := AirflowUpgrade(mockDeployment.ID, mockDeployment.DesiredAirflowVersion, api, buf)
		s.Error(err, errGetDeploymentMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade airflow update deployment error", func() {
		expectedVars := map[string]interface{}{"deploymentId": mockDeployment.ID, "desiredAirflowVersion": mockDeployment.DesiredAirflowVersion}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(nil, errUpdateDeploymentMock)

		buf := new(bytes.Buffer)
		_, err := AirflowUpgrade(mockDeployment.ID, mockDeployment.DesiredAirflowVersion, api, buf)
		s.Error(err, errUpdateDeploymentMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestAirflowUpgradeCancel() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "ckggzqj5f4157qtc9lescmehm"

	mockDeployment := &houston.Deployment{
		ID:                    "ckggzqj5f4157qtc9lescmehm",
		Type:                  "airflow",
		Label:                 "test",
		ReleaseName:           "burning-terrestrial-5940",
		Version:               "0.0.0",
		AirflowVersion:        "1.10.5",
		DesiredAirflowVersion: "1.10.10",
	}

	expectedVars := map[string]interface{}{"deploymentId": mockDeployment.ID, "desiredAirflowVersion": mockDeployment.AirflowVersion}

	s.Run("upgrade cancel success", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(mockDeployment, nil)

		got, err := AirflowUpgradeCancel(deploymentID, api)
		s.NoError(err)
		s.Equal(VersionChangeCanceled, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "1.10.5"}, got.Current)
		s.Equal((*ImageVersion)(nil), got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("nothing to upgrade cancel", func() {
		api := new(mocks.ClientInterface)
		mockResp := *mockDeployment
		mockResp.AirflowVersion = mockResp.DesiredAirflowVersion
		api.On("GetDeployment", mockDeployment.ID).Return(&mockResp, nil)

		got, err := AirflowUpgradeCancel(deploymentID, api)
		s.NoError(err)
		s.Equal(VersionChangeNothingToCancel, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "1.10.10"}, got.Current)
		s.Equal((*ImageVersion)(nil), got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade cancel get deployment error", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, errGetDeploymentMock)

		_, err := AirflowUpgradeCancel(deploymentID, api)
		s.EqualError(err, errGetDeploymentMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade cancel error", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		expectedVars["desiredAirflowVersion"] = mockDeployment.AirflowVersion
		api.On("UpdateDeploymentAirflow", expectedVars).Return(nil, errUpdateDeploymentMock)

		_, err := AirflowUpgradeCancel(deploymentID, api)
		s.EqualError(err, errUpdateDeploymentMock.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade empty desired version", func() {
		mockDeploymentConfig := &houston.DeploymentConfig{
			AirflowVersions: []string{
				"1.10.7",
				"1.10.10",
				"1.10.12",
			},
		}
		expectedVars["desiredAirflowVersion"] = mockDeployment.DesiredAirflowVersion
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("GetDeploymentConfig", nil).Return(mockDeploymentConfig, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(mockDeployment, nil)

		// mock os.Stdin for when prompted by getAirflowVersionSelection()
		input := []byte("2")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		got, err := AirflowUpgrade(deploymentID, "", api, buf)
		s.T().Log(buf.String()) // Log the buffer so that this test is recognized by go test

		s.NoError(err)
		rows, rest := s.picked(buf.String())
		s.Empty(rest, "the picker is all it draws; the result is the caller's")
		s.Equal([][]string{
			{"#", "AIRFLOW", "VERSION"},
			{"1", "Astronomer-Certified-1.10.7"},
			{"2", "Astronomer-Certified-1.10.10"},
			{"3", "Astronomer-Certified-1.10.12"},
		}, rows)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "1.10.5"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageCertified, Version: "1.10.10"}, got.Desired)
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) Test_getAirflowVersionSelection() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeploymentConfig := &houston.DeploymentConfig{
		AirflowVersions: []string{
			"1.10.7",
			"1.10.10",
			"1.10.12",
		},
	}

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeploymentConfig", nil).Return(mockDeploymentConfig, nil)

		buf := new(bytes.Buffer)

		// mock os.Stdin
		input := []byte("2")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		airflowVersion, err := getAirflowVersionSelection("1.10.7", api, buf)
		s.T().Log(buf.String()) // Log the buffer so that this test is recognized by go test
		s.NoError(err)
		s.Equal("1.10.12", airflowVersion)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeploymentConfig", nil).Return(nil, errMock)

		buf := new(bytes.Buffer)
		airflowVersion, err := getAirflowVersionSelection("1.10.7", api, buf)
		s.EqualError(err, errMock.Error())
		s.Equal("", airflowVersion)
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) Test_meetsAirflowUpgradeReqs() {
	airflowVersion := "1.10.12"
	desiredAirflowVersion := "2.0.0"
	err := meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	s.Error(err)
	s.EqualError(err, "Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to 1.10.14 "+
		"first and make sure your Dags and configs are 2.0 compatible")

	airflowVersion = "2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	s.Error(err)
	s.EqualError(err, "Error: You tried to set --desired-airflow-version to 2.0.0, but this Airflow Deployment "+
		"is already running 2.0.0. Please indicate a higher version of Airflow and try again.")

	airflowVersion = "1.10.14"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	s.NoError(err)

	airflowVersion = "1.10.7"
	desiredAirflowVersion = "1.10.10"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	s.NoError(err)

	airflowVersion = "-1.10.12"
	desiredAirflowVersion = "2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	s.Error(err)
	s.EqualError(err, "invalid semantic version")

	airflowVersion = "1.10.12"
	desiredAirflowVersion = "-2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	s.Error(err)
	s.EqualError(err, "invalid semantic version")
}

func (s *Suite) TestGetDeploymentSelectionNamespaces() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockAvailableNamespaces := []houston.Namespace{
		{Name: "test1"},
		{Name: "test2"},
	}

	s.Run("get available namespaces", func() {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return(mockAvailableNamespaces, nil)

		buf := new(bytes.Buffer)

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

		name, err := getDeploymentSelectionNamespaces(api, buf, "testClusterID", "")
		s.NoError(err)
		rows, rest := s.picked(buf.String())
		s.Equal([][]string{{"#", "AVAILABLE", "KUBERNETES", "NAMESPACES"}, {"1", "test1"}, {"2", "test2"}}, rows)
		s.Empty(rest)
		s.Equal("test1", name)
		api.AssertExpectations(s.T())
	})

	// Not a number is a parse failure; a number that is no row, or one
	// spelled other than plainly, is out of range. Neither indexes the list.
	s.Run("an answer that is no row", func() {
		for answer, want := range map[string]error{
			"x\n":  ErrParsingInt{in: "x"},
			"01\n": ErrParsingInt{in: "01"},
			"0\n":  ErrNumberOutOfRange,
			"3\n":  ErrNumberOutOfRange,
		} {
			api := new(mocks.ClientInterface)
			api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return(mockAvailableNamespaces, nil)
			testUtil.MockUserInput(s.T(), answer)
			_, err := getDeploymentSelectionNamespaces(api, new(bytes.Buffer), "testClusterID", "")
			s.Equal(want, err, "answer %q", answer)
		}
	})

	s.Run("no namespace", func() {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return([]houston.Namespace{}, nil)

		buf := new(bytes.Buffer)
		name, err := getDeploymentSelectionNamespaces(api, buf, "testClusterID", "")
		expected := ``
		s.Equal(expected, name)
		s.EqualError(err, "no kubernetes namespaces are available")
		api.AssertExpectations(s.T())
	})

	s.Run("parse error", func() {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return(mockAvailableNamespaces, nil)

		buf := new(bytes.Buffer)

		// mock os.Stdin
		input := []byte("test")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		name, err := getDeploymentSelectionNamespaces(api, buf, "testClusterID", "")
		s.Equal("", name)
		s.EqualError(err, "cannot parse test to int")
		api.AssertExpectations(s.T())
	})

	s.Run("api error", func() {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", map[string]interface{}{"clusterID": "testClusterID"}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		name, err := getDeploymentSelectionNamespaces(api, buf, "testClusterID", "")
		s.Equal("", name)
		s.EqualError(err, errMock.Error())
	})
}

func (s *Suite) TestGetDeploymentNamespaceName() {
	// mock os.Stdin
	input := []byte("test1")
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.Write(input)
	s.NoError(err)
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	name, _ := getDeploymentNamespaceName("")
	s.Equal("test1", name)
}

func (s *Suite) TestGetDeploymentNamespaceNameError() {
	// mock os.Stdin
	input := []byte("   ")
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.Write(input)
	s.NoError(err)
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	name, err := getDeploymentNamespaceName("")
	s.Equal("", name)
	s.EqualError(err, "no kubernetes namespaces specified")
}

func (s *Suite) TestAddDagDeploymentArgs() {
	tests := []*struct {
		dagDeploymentType string
		nfsLocation       string
		sshKey            string
		knownHosts        string
		gitRepoURL        string
		gitRevision       string
		gitBranchName     string
		gitDAGDir         string
		gitSyncInterval   int
		expectedError     string
		expectedOutput    map[string]interface{}
	}{
		{
			dagDeploymentType: houston.ImageDeploymentType,
			expectedError:     "",
			expectedOutput:    map[string]interface{}{"dagDeployment": map[string]interface{}{"type": houston.ImageDeploymentType}},
		},
		{
			dagDeploymentType: houston.VolumeDeploymentType,
			nfsLocation:       "test",
			expectedError:     "",
			expectedOutput:    map[string]interface{}{"dagDeployment": map[string]interface{}{"type": houston.VolumeDeploymentType, "nfsLocation": "test"}},
		},
		{
			dagDeploymentType: houston.GitSyncDeploymentType,
			sshKey:            "../../../../cmd/apc/testfiles/ssh_key",
			knownHosts:        "../../../../cmd/apc/testfiles/known_hosts",
			gitRepoURL:        "https://github.com/neel-astro/private-airflow-dags-test",
			gitRevision:       "test-revision",
			gitBranchName:     "test-branch",
			gitDAGDir:         "test-dags",
			gitSyncInterval:   1,
			expectedError:     "",
			expectedOutput:    map[string]interface{}{"dagDeployment": map[string]interface{}{"branchName": "test-branch", "dagDirectoryLocation": "test-dags", "knownHosts": "github.com ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7hRTest1ngUDbO9IDSwBK6TbQa+PXYPCPy6rbTrTtw7PHkccKrpp0yVhp5HdEIcKr6pLlVDBfOLX9QUsyCOV0wzfjIJNlGEYsdlLJizHhbn2mUjvTestingTYP81eFzLQNnPHt4EVVUh7VfDESU84KezmD5QlWpXLmvU31/yMf+Se8xhHTestingFImWwoG6mbUoWf9nzpIoaSjB+weqqUTestingXVal72J+UX2B+2RPW3RcT0eOzQgqlJL3RKrTJvdsjE3JEAvGq3lGHSZXy28G3skua2SmVi/w4yCE6gbODqnTWlg7+wC604ydTestingS5ap43JXiUFFAaQ==", "repositoryUrl": "https://github.com/neel-astro/private-airflow-dags-test", "rev": "test-revision", "sshKey": "Test_ssh_key_file_content\n", "syncInterval": 1, "type": houston.GitSyncDeploymentType}},
		},
	}

	for _, tt := range tests {
		output := map[string]interface{}{}
		err := addDagDeploymentArgs(output, tt.dagDeploymentType, tt.nfsLocation, tt.sshKey, tt.knownHosts, tt.gitRepoURL, tt.gitRevision, tt.gitBranchName, tt.gitDAGDir, tt.gitSyncInterval)
		if tt.expectedError != "" {
			s.Equal(tt.expectedError, err.Error())
		} else {
			s.NoError(err)
		}
		s.Equal(output, tt.expectedOutput)
	}
}

func (s *Suite) TestRuntimeUpgrade() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeployment := &houston.Deployment{
		ID:                    "ckbv818oa00r107606ywhoqtw",
		Type:                  "airflow",
		Label:                 "test123",
		ReleaseName:           "burning-terrestrial-5940",
		Version:               "0.0.0",
		AirflowVersion:        "2.2.0",
		DesiredAirflowVersion: "2.2.0",
		RuntimeVersion:        "4.2.4",
		RuntimeAirflowVersion: "2.2.4",
		DesiredRuntimeVersion: "4.2.5",
	}

	s.Run("upgrade runtime success when deployment is coming from AC migration", func() {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)
		buf := new(bytes.Buffer)
		got, err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		s.NoError(err)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageRuntime, Version: "4.2.5"}, got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade runtime success if deployment was always using runtime", func() {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}

		api := new(mocks.ClientInterface)
		mockDeployment.AirflowVersion = ""
		mockDeployment.DesiredAirflowVersion = ""
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)
		buf := new(bytes.Buffer)
		got, err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		s.NoError(err)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageRuntime, Version: "4.2.5"}, got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("not on runtime", func() {
		mockResp := *mockDeployment
		mockResp.AirflowVersion = "2.2.5"
		mockResp.RuntimeVersion = ""

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(&mockResp, nil)

		buf := new(bytes.Buffer)
		_, err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		s.ErrorIs(err, errDeploymentNotOnRuntime)
	})

	s.Run("upgrade runtime get deployment error", func() {
		mockError := errors.New("get deployment error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		_, err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		s.Error(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade runtime update deployment error", func() {
		mockError := errors.New("update deployment error")
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		buf := new(bytes.Buffer)
		_, err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		s.Error(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade runtime with empty desired version", func() {
		mockRuntimeReleases := houston.RuntimeReleases{
			houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
			houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
		}
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		vars := make(map[string]interface{})
		vars["clusterId"] = ""
		api.On("GetRuntimeReleases", vars).Return(mockRuntimeReleases, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)

		// mock os.Stdin for when prompted by getAirflowVersionSelection()
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

		buf := new(bytes.Buffer)
		got, err := RuntimeUpgrade(mockDeployment.ID, "", api, buf)
		s.T().Log(buf.String()) // Log the buffer so that this test is recognized by go test

		s.NoError(err)
		rows, rest := s.picked(buf.String())
		s.Empty(rest, "the picker is all it draws; the result is the caller's")
		s.Equal([][]string{{"#", "RUNTIME", "VERSION"}, {"1", "Runtime-4.2.5"}}, rows)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageRuntime, Version: "4.2.5"}, got.Desired)
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestRuntimeUpgradeCancel() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "ckggzqj5f4157qtc9lescmehm"

	mockDeployment := &houston.Deployment{
		ID:                    "ckggzqj5f4157qtc9lescmehm",
		Type:                  "airflow",
		Label:                 "test",
		ReleaseName:           "burning-terrestrial-5940",
		Version:               "0.0.0",
		RuntimeVersion:        "4.2.4",
		DesiredRuntimeVersion: "4.2.5",
	}

	expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID}

	s.Run("upgrade cancel success", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)

		got, err := RuntimeUpgradeCancel(deploymentID, api)
		s.NoError(err)
		s.Equal(VersionChangeCanceled, got.Action)
		s.Equal(ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Current)
		s.Equal((*ImageVersion)(nil), got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade cancel get deployment error", func() {
		mockError := errors.New("get deployment error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		_, err := RuntimeUpgradeCancel(deploymentID, api)
		s.EqualError(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("upgrade cancel error", func() {
		mockError := errors.New("update deployment error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		_, err := RuntimeUpgradeCancel(deploymentID, api)
		s.EqualError(err, mockError.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestRuntimeMigrate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeployment := &houston.Deployment{
		ID:             "ckbv818oa00r107606ywhoqtw",
		Type:           "airflow",
		Label:          "test123",
		ReleaseName:    "burning-terrestrial-5940",
		Version:        "0.0.0",
		RuntimeVersion: "",
		AirflowVersion: "2.2.4",
	}

	s.Run("migrate runtime success", func() {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": "4.2.4"}

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		vars := make(map[string]interface{})
		vars["airflowVersion"] = mockDeployment.AirflowVersion
		vars["clusterId"] = ""
		api.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{houston.RuntimeRelease{Version: "4.2.3", AirflowVersion: "2.2.3"}, houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"}}, nil)
		mockMigrateRuntimeResp := *mockDeployment
		mockMigrateRuntimeResp.RuntimeVersion = "4.2.4"
		api.On("UpdateDeploymentRuntime", expectedVars).Return(&mockMigrateRuntimeResp, nil)
		got, err := RuntimeMigrate(mockDeployment.ID, api)
		s.NoError(err)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "2.2.4"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("migrate runtime success for 1.0.0", func() {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": "4.2.4"}

		mockDeploymentResp := *mockDeployment
		mockDeploymentResp.ClusterID = "test-cluster-id"
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(&mockDeploymentResp, nil)
		vars := make(map[string]interface{})
		vars["airflowVersion"] = mockDeployment.AirflowVersion
		vars["clusterId"] = "test-cluster-id"
		api.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{houston.RuntimeRelease{Version: "4.2.3", AirflowVersion: "2.2.3"}, houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"}}, nil)
		mockMigrateRuntimeUpdateResp := *mockDeployment
		mockMigrateRuntimeUpdateResp.RuntimeVersion = "4.2.4"
		api.On("UpdateDeploymentRuntime", expectedVars).Return(&mockMigrateRuntimeUpdateResp, nil)
		got, err := RuntimeMigrate(mockDeployment.ID, api)
		s.NoError(err)
		s.Equal(VersionChangeStarted, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "2.2.4"}, got.Current)
		s.Equal(&ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("migrate runtime get deployment error", func() {
		mockError := errors.New("get deployment error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		_, err := RuntimeMigrate(mockDeployment.ID, api)
		s.Error(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("already on runtime error", func() {
		api := new(mocks.ClientInterface)
		mockDeploymentResp := *mockDeployment
		mockDeploymentResp.RuntimeVersion = "4.2.4"
		mockDeploymentResp.AirflowVersion = ""
		api.On("GetDeployment", mockDeployment.ID).Return(&mockDeploymentResp, nil)

		_, err := RuntimeMigrate(mockDeployment.ID, api)
		s.Error(err, errDeploymentAlreadyOnRuntime)
		api.AssertExpectations(s.T())
	})

	s.Run("migrate runtime get runtime releases error", func() {
		mockError := errors.New("get runtime releases error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		vars := make(map[string]interface{})
		vars["airflowVersion"] = mockDeployment.AirflowVersion
		vars["clusterId"] = ""
		api.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, mockError)

		_, err := RuntimeMigrate(mockDeployment.ID, api)
		s.Error(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("invalid airflow version to migrate to runtime", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		vars := make(map[string]interface{})
		vars["airflowVersion"] = mockDeployment.AirflowVersion
		vars["clusterId"] = ""
		api.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{}, nil)

		_, err := RuntimeMigrate(mockDeployment.ID, api)
		s.Error(err, errInvalidAirflowVersion)
		api.AssertExpectations(s.T())
	})

	s.Run("migrate runtime update deployment error", func() {
		mockError := errors.New("update deployment error")
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": "4.2.4"}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		vars := make(map[string]interface{})
		vars["airflowVersion"] = mockDeployment.AirflowVersion
		vars["clusterId"] = ""
		api.On("GetRuntimeReleases", vars).Return(houston.RuntimeReleases{houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"}}, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		_, err := RuntimeMigrate(mockDeployment.ID, api)
		s.Error(err, mockError.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestRuntimeMigrateCancel() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "ckggzqj5f4157qtc9lescmehm"

	mockDeployment := &houston.Deployment{
		ID:                    "ckggzqj5f4157qtc9lescmehm",
		Type:                  "airflow",
		Label:                 "test",
		ReleaseName:           "burning-terrestrial-5940",
		Version:               "0.0.0",
		RuntimeVersion:        "",
		DesiredRuntimeVersion: "4.2.4",
		AirflowVersion:        "2.2.4",
	}

	expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID}

	s.Run("migrate cancel success", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)

		got, err := RuntimeMigrateCancel(deploymentID, api)
		s.NoError(err)
		s.Equal(VersionChangeCanceled, got.Action)
		s.Equal(ImageVersion{Image: ImageCertified, Version: "2.2.4"}, got.Current)
		s.Equal((*ImageVersion)(nil), got.Desired)
		api.AssertExpectations(s.T())
	})

	s.Run("migrate cancel get deployment error", func() {
		mockError := errors.New("get deployment error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		_, err := RuntimeMigrateCancel(deploymentID, api)
		s.EqualError(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("migrate cancel error", func() {
		mockError := errors.New("update deployment error")
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		_, err := RuntimeMigrateCancel(deploymentID, api)
		s.EqualError(err, mockError.Error())
		api.AssertExpectations(s.T())
	})

	s.Run("already migrated error", func() {
		api := new(mocks.ClientInterface)
		mockDeploymentResp := *mockDeployment
		mockDeploymentResp.AirflowVersion = ""
		mockDeploymentResp.RuntimeVersion = mockDeployment.DesiredRuntimeVersion
		api.On("GetDeployment", mockDeployment.ID).Return(&mockDeploymentResp, nil)

		got, err := RuntimeMigrateCancel(deploymentID, api)
		s.NoError(err)
		s.Equal(VersionChangeNothingToCancel, got.Action)
		s.Equal(ImageVersion{Image: ImageRuntime, Version: "4.2.4"}, got.Current)
		s.Equal((*ImageVersion)(nil), got.Desired)
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestMeetsRuntimeUpgradeReqs() {
	type args struct {
		runtimeVersion        string
		desiredRuntimeVersion string
	}
	tests := []*struct {
		name        string
		args        args
		expectedErr error
	}{
		{
			name:        "valid case",
			args:        args{runtimeVersion: "4.2.4", desiredRuntimeVersion: "4.2.5"},
			expectedErr: nil,
		},
		{
			name:        "invalid case",
			args:        args{runtimeVersion: "4.2.4", desiredRuntimeVersion: "4.2.4"},
			expectedErr: ErrInvalidRuntimeVersion{currentVersion: "4.2.4", desiredVersion: "4.2.4"},
		},
		{
			name:        "error parsing runtime version",
			args:        args{runtimeVersion: "invalid version", desiredRuntimeVersion: "4.2.5"},
			expectedErr: fmt.Errorf("invalid semantic version"),
		},
		{
			name:        "error parsing desired runtime version",
			args:        args{runtimeVersion: "4.2.5", desiredRuntimeVersion: "invalid version"},
			expectedErr: fmt.Errorf("invalid semantic version"),
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := meetsRuntimeUpgradeReqs(tt.args.runtimeVersion, tt.args.desiredRuntimeVersion)
			if tt.expectedErr == nil {
				s.NoError(err)
			} else {
				s.EqualError(err, tt.expectedErr.Error())
			}
		})
	}
}

// picked splits what a picker wrote from what followed it: the cells of each
// line of its table, and the rest of out after its "> " prompt.
func (s *Suite) picked(out string) (rows [][]string, rest string) {
	table, rest, found := strings.Cut(out, "\n\n> ")
	s.Require().True(found, "no picker prompt in %q", out)
	for _, line := range strings.Split(table, "\n") {
		rows = append(rows, strings.Fields(line))
	}
	return rows, rest
}
