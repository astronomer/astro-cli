package deployment

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	semver "github.com/Masterminds/semver/v3"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock                 = errors.New("api error")
	errGetDeploymentMock    = errors.New("get deployment error")
	errUpdateDeploymentMock = errors.New("update deployment error")
	errRegMock              = errors.New("error")
)

func TestGetDeployments(t *testing.T) {
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
		t.Errorf("Expected deployments to be %v, but got %v", expectedDeployments, deployments)
	}

	// Assert that there was no error returned
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}

	// Assert that the ListDeployments function was called with the correct parameters
	expectedRequest := houston.ListDeploymentsRequest{WorkspaceID: "workspace"}
	mockClient.AssertCalled(t, "ListDeployments", expectedRequest)

	mockClientErr := &mocks.ClientInterface{}
	mockClientErr.On("ListDeployments", mock.Anything).Return(nil, errMock)
	_, err = GetDeployments("workspace", mockClientErr)
	assert.EqualError(t, err, GetDeploymentsErr(errMock).Error())
}

func TestSelectDeployment(t *testing.T) {
	t.Run("no deployments", func(t *testing.T) {
		deployments := []houston.Deployment{}
		message := "Choose a deployment:"
		dep, err := SelectDeployment(deployments, message)
		assert.Equal(t, houston.Deployment{}, dep)
		assert.Nil(t, err)
	})
	t.Run("One deployment", func(t *testing.T) {
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
		assert.Equal(t, deployments[0], dep)
		assert.Nil(t, err)
	})
	//
	t.Run("Multiple deployments", func(t *testing.T) {
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
		testUtil.MockUserInput(t, "2\n")
		dep, err := SelectDeployment(deployments, message)
		assert.Equal(t, deployments[1], dep)
		assert.Nil(t, err)
	})
	//
	t.Run("Invalid choice", func(t *testing.T) {
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
		testUtil.MockUserInput(t, "4\n")

		dep, err := SelectDeployment(deployments, message)
		assert.Equal(t, houston.Deployment{}, dep)
		assert.Equal(t, ErrInvalidDeploymentKey, err)
	})
}

func TestCheckManualReleaseNames(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("manual release names true", func(t *testing.T) {
		appConfig := &houston.AppConfig{
			ManualReleaseNames: true,
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)

		assert.True(t, checkManualReleaseNames(api))
		api.AssertExpectations(t)
	})

	t.Run("manual release names false", func(t *testing.T) {
		appConfig := &houston.AppConfig{
			ManualReleaseNames: false,
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(appConfig, nil)

		assert.False(t, checkManualReleaseNames(api))
		api.AssertExpectations(t)
	})

	t.Run("manual release names error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(nil, errMock)

		assert.False(t, checkManualReleaseNames(api))
		api.AssertExpectations(t)
	})
}

func TestCreate(t *testing.T) {
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
	req := &CreateDeploymentRequest{label, ws, releaseName, role, executor, airflowVersion, "", dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas}

	t.Run("create success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		// Have to use mock anything for now as vars is too big
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		err := Create(req, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
		api.AssertExpectations(t)
	})

	t.Run("create trigger enabled", func(t *testing.T) {
		mockAppConfig.TriggererEnabled = true

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		triggerReplicas = 1
		buf := new(bytes.Buffer)
		err := Create(req, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
		api.AssertExpectations(t)
	})

	t.Run("create trigger enabled with trigger replicas count -1", func(t *testing.T) {
		mockAppConfig.TriggererEnabled = true

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		triggerReplicas = -1
		buf := new(bytes.Buffer)
		req = &CreateDeploymentRequest{label, ws, releaseName, role, executor, airflowVersion, "", dagDeploymentType, nfsLocation, "", "", "", "", "", "", 1, triggerReplicas}
		err := Create(req, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
		api.AssertExpectations(t)
	})

	t.Run("create nfslocation enabled", func(t *testing.T) {
		mockAppConfig.TriggererEnabled = false

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		nfsLocation = "test:/test"
		triggerReplicas = 0

		buf := new(bytes.Buffer)
		err := Create(req, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
		api.AssertExpectations(t)
	})

	t.Run("create git sync enabled", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		nfsLocation = ""
		dagDeploymentType = houston.GitSyncDeploymentType

		myTests := []struct {
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
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../cmd/software/testfiles/ssh_key", knownHosts: "../../cmd/software/testfiles/known_hosts", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../cmd/software/testfiles/ssh_key", syncInterval: 100, expectedOutput: "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs", expectedError: ""},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../cmd/software/testfiles/wrong_ssh_key", knownHosts: "../../cmd/software/testfiles/known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "wrong path specified, no file exists for ssh key"},
			{repoURL: "https://github.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../cmd/software/testfiles/ssh_key", knownHosts: "../../cmd/software/testfiles/wrong_known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "wrong path specified, no file exists for known hosts"},
			{repoURL: "https://gitlab.com/neel-astro/private-airflow-dags-test", revision: "304e0ff3e4dde9063204ff52ce39b8aa01b5b682", dagDirectoryLocation: "dagscopy/", branchName: "main", sshKey: "../../cmd/software/testfiles/ssh_key", knownHosts: "../../cmd/software/testfiles/known_hosts", syncInterval: 100, expectedOutput: "", expectedError: "git repository host not present in known hosts file"},
		}

		for _, tt := range myTests {
			buf := new(bytes.Buffer)
			createReq := &CreateDeploymentRequest{label, ws, releaseName, role, executor, "", runtimeVersion, dagDeploymentType, "", tt.repoURL, tt.revision, tt.branchName, tt.dagDirectoryLocation, tt.sshKey, tt.knownHosts, tt.syncInterval, triggerReplicas}
			err := Create(createReq, api, buf)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.Contains(t, buf.String(), tt.expectedOutput)
		}
	})

	t.Run("create with pre-create namespace deployment success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		releaseName = ""
		dagDeploymentType = houston.VolumeDeploymentType
		nfsLocation = "test:/test"

		buf := new(bytes.Buffer)

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

		err = Create(req, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
		api.AssertExpectations(t)
	})

	t.Run("create pre-create namespace deployment error", func(t *testing.T) {
		appConfig := *mockAppConfig
		appConfig.Flags = houston.FeatureFlags{
			ManualNamespaceNames: true,
		}
		mockNamespaces := []houston.Namespace{
			{Name: "test1"},
			{Name: "test2"},
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(&appConfig, nil)
		api.On("GetAvailableNamespaces", nil).Return(mockNamespaces, nil)

		buf := new(bytes.Buffer)

		// mock os.Stdin
		input := []byte("5")
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

		err = Create(req, api, buf)
		assert.EqualError(t, err, "number is out of available range")
		api.AssertExpectations(t)
	})

	t.Run("create get namespaces error", func(t *testing.T) {
		appConfig := *mockAppConfig
		appConfig.Flags = houston.FeatureFlags{
			ManualNamespaceNames: true,
		}

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(&appConfig, nil)
		api.On("GetAvailableNamespaces", nil).Return([]houston.Namespace{}, errMock)

		buf := new(bytes.Buffer)
		err := Create(req, api, buf)
		assert.EqualError(t, err, errMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("create api error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := Create(req, api, buf)
		assert.EqualError(t, err, errMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("create free form namespace success", func(t *testing.T) {
		mockAppConfig.Flags.NamespaceFreeFormEntry = true

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("Test1")
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

		err = Create(req, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully created deployment with Celery executor. Deployment can be accessed at the following URLs")
		api.AssertExpectations(t)
	})
	t.Run("create free form namespace error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("CreateDeployment", mock.Anything).Return(nil, errMock)

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("    ")
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

		err = Create(req, api, buf)
		assert.EqualError(t, err, "no kubernetes namespaces specified")
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockDeployment := &houston.Deployment{
		ID:             "ckbv818oa00r107606ywhoqtw",
		Type:           "airflow",
		Label:          "test",
		ReleaseName:    "prehistoric-gravity312",
		Version:        "1.1.0",
		AirflowVersion: "1.1.0",
	}

	t.Run("delete success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: false}).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		err := Delete(mockDeployment.ID, false, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully deleted deployment")
		api.AssertExpectations(t)
	})

	t.Run("delete api error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: false}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := Delete(mockDeployment.ID, false, api, buf)
		assert.EqualError(t, err, errMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("delete hard success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: mockDeployment.ID, HardDelete: true}).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		err := Delete(mockDeployment.ID, true, api, buf)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Successfully deleted deployment")
	})
}

func TestList(t *testing.T) {
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

	t.Run("list deployments for workspace success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListDeployments", expectedRequest).Return(mockDeployments, nil)

		buf := new(bytes.Buffer)
		err := List(mockDeployments[0].Workspace.ID, false, api, buf)
		assert.NoError(t, err)
		expected := ` NAME     DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 TAG     IMAGE VERSION                  
 test     burning-terrestrial-5940     v1.1.0     ckbv801t300qh0760pck7ea0c     ?       Astronomer-Certified-1.1.0     
`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("list namespace api error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListDeployments", expectedRequest).Return([]houston.Deployment{}, errMock)

		buf := new(bytes.Buffer)
		err := List(mockDeployments[0].Workspace.ID, false, api, buf)
		assert.EqualError(t, err, errMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("list namespace all enabled", func(t *testing.T) {
		expectedRequest.WorkspaceID = ""

		api := new(mocks.ClientInterface)
		api.On("ListDeployments", expectedRequest).Return(mockDeployments, nil)

		buf := new(bytes.Buffer)
		err := List(mockDeployments[0].Workspace.ID, true, api, buf)
		assert.NoError(t, err)
		expected := ` NAME     DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 TAG     IMAGE VERSION                  
 test     burning-terrestrial-5940     v1.1.0     ckbv801t300qh0760pck7ea0c     ?       Astronomer-Certified-1.1.0     
`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})
}

func TestUpdate(t *testing.T) {
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

	t.Run("update success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

		expected := ` NAME        DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG         IMAGE VERSION                  
 test123     burning-terrestrial-5940     0.0.0     ckbv801t300qh0760pck7ea0c     2.2.2-1     Astronomer-Certified-2.2.2     

 Successfully updated deployment
`
		myTests := []struct {
			deploymentConfig  map[string]string
			dagDeploymentType string
			expectedOutput    string
		}{
			{deploymentConfig: map[string]string{"executor": houston.CeleryExecutorType}, dagDeploymentType: "", expectedOutput: expected},
			{deploymentConfig: map[string]string{"executor": houston.CeleryExecutorType}, dagDeploymentType: houston.ImageDeploymentType, expectedOutput: expected},
		}
		for _, tt := range myTests {
			buf := new(bytes.Buffer)
			err := Update(mockDeployment.ID, role, tt.deploymentConfig, tt.dagDeploymentType, "", "", "", "", "", "", "", "", 1, 0, api, buf)
			assert.NoError(t, err)
			assert.Equal(t, expected, buf.String())
			api.AssertExpectations(t)
		}
	})

	t.Run("update triggerer enabled", func(t *testing.T) {
		mockAppConfig.TriggererEnabled = true

		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(mockDeployment, nil)

		expected := ` NAME        DEPLOYMENT NAME              ASTRO     DEPLOYMENT ID                 TAG         IMAGE VERSION                  
 test123     burning-terrestrial-5940     0.0.0     ckbv801t300qh0760pck7ea0c     2.2.2-1     Astronomer-Certified-2.2.2     

 Successfully updated deployment
`
		myTests := []struct {
			deploymentConfig  map[string]string
			dagDeploymentType string
			expectedOutput    string
		}{
			{deploymentConfig: map[string]string{"executor": houston.CeleryExecutorType}, dagDeploymentType: "", expectedOutput: expected},
			{deploymentConfig: map[string]string{"executor": houston.CeleryExecutorType}, dagDeploymentType: houston.ImageDeploymentType, expectedOutput: expected},
		}
		for _, tt := range myTests {
			buf := new(bytes.Buffer)
			err := Update(mockDeployment.ID, role, tt.deploymentConfig, tt.dagDeploymentType, "", "", "", "", "", "", "", "", 1, 1, api, buf)
			assert.NoError(t, err)
			assert.Equal(t, expected, buf.String())
			api.AssertExpectations(t)
		}
	})

	t.Run("update error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
		api.On("UpdateDeployment", mock.Anything).Return(nil, errMock)

		deploymentConfig := make(map[string]string)
		deploymentConfig["executor"] = houston.CeleryExecutorType

		buf := new(bytes.Buffer)
		err := Update(mockDeployment.ID, role, deploymentConfig, "", "", "", "", "", "", "", "", "", 1, 0, api, buf)

		assert.EqualError(t, err, errMock.Error())
		api.AssertExpectations(t)
	})
}

func TestAirflowUpgrade(t *testing.T) {
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

	t.Run("upgrade airflow success", func(t *testing.T) {
		expectedVars := map[string]interface{}{"deploymentId": mockDeployment.ID, "desiredAirflowVersion": mockDeployment.DesiredAirflowVersion}

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(mockDeployment, nil)
		buf := new(bytes.Buffer)
		err := AirflowUpgrade(mockDeployment.ID, mockDeployment.DesiredAirflowVersion, api, buf)
		assert.NoError(t, err)
		expected := ` NAME        DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 IMAGE VERSION                    
 test123     burning-terrestrial-5940     v0.0.0     ckbv818oa00r107606ywhoqtw     Astronomer-Certified-1.10.10     

The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment airflow upgrade --cancel

`

		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("upgrade airflow get deployment error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, errGetDeploymentMock)

		buf := new(bytes.Buffer)
		err := AirflowUpgrade(mockDeployment.ID, mockDeployment.DesiredAirflowVersion, api, buf)
		assert.Error(t, err, errGetDeploymentMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("upgrade airflow update deployment error", func(t *testing.T) {
		expectedVars := map[string]interface{}{"deploymentId": mockDeployment.ID, "desiredAirflowVersion": mockDeployment.DesiredAirflowVersion}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(nil, errUpdateDeploymentMock)

		buf := new(bytes.Buffer)
		err := AirflowUpgrade(mockDeployment.ID, mockDeployment.DesiredAirflowVersion, api, buf)
		assert.Error(t, err, errUpdateDeploymentMock.Error())
		api.AssertExpectations(t)
	})
}

func TestAirflowUpgradeCancel(t *testing.T) {
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

	t.Run("upgrade cancel success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentAirflow", expectedVars).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		err := AirflowUpgradeCancel(deploymentID, api, buf)
		assert.NoError(t, err)
		expected := `
Airflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 1.10.5.
`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("nothing to upgrade cancel", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		mockResp := *mockDeployment
		mockResp.AirflowVersion = mockResp.DesiredAirflowVersion
		api.On("GetDeployment", mockDeployment.ID).Return(&mockResp, nil)

		buf := new(bytes.Buffer)
		err := AirflowUpgradeCancel(deploymentID, api, buf)
		assert.NoError(t, err)
		expected := `
Nothing to cancel. You are currently running Airflow 1.10.10 and you have not indicated that you want to upgrade.`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("upgrade cancel get deployment error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, errGetDeploymentMock)

		buf := new(bytes.Buffer)
		err := AirflowUpgradeCancel(deploymentID, api, buf)
		assert.EqualError(t, err, errGetDeploymentMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("upgrade cancel error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		expectedVars["desiredAirflowVersion"] = mockDeployment.AirflowVersion
		api.On("UpdateDeploymentAirflow", expectedVars).Return(nil, errUpdateDeploymentMock)

		buf := new(bytes.Buffer)
		err := AirflowUpgradeCancel(deploymentID, api, buf)
		assert.EqualError(t, err, errUpdateDeploymentMock.Error())
		api.AssertExpectations(t)
	})

	t.Run("upgrade empty desired version", func(t *testing.T) {
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

		buf := new(bytes.Buffer)
		err = AirflowUpgrade(deploymentID, "", api, buf)
		t.Log(buf.String()) // Log the buffer so that this test is recognized by go test

		assert.NoError(t, err)
		expected := `#     AIRFLOW VERSION                  
1     Astronomer-Certified-1.10.7      
2     Astronomer-Certified-1.10.10     
3     Astronomer-Certified-1.10.12     
 NAME     DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 IMAGE VERSION                    
 test     burning-terrestrial-5940     v0.0.0     ckggzqj5f4157qtc9lescmehm     Astronomer-Certified-1.10.10     

The upgrade from Airflow 1.10.5 to 1.10.10 has been started. To complete this process, add an Airflow 1.10.10 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment airflow upgrade --cancel

`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})
}

func Test_getAirflowVersionSelection(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockDeploymentConfig := &houston.DeploymentConfig{
		AirflowVersions: []string{
			"1.10.7",
			"1.10.10",
			"1.10.12",
		},
	}

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeploymentConfig", nil).Return(mockDeploymentConfig, nil)

		buf := new(bytes.Buffer)

		// mock os.Stdin
		input := []byte("2")
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

		airflowVersion, err := getAirflowVersionSelection("1.10.7", api, buf)
		t.Log(buf.String()) // Log the buffer so that this test is recognized by go test
		assert.NoError(t, err)
		assert.Equal(t, "1.10.12", airflowVersion)
		api.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeploymentConfig", nil).Return(nil, errMock)

		buf := new(bytes.Buffer)
		airflowVersion, err := getAirflowVersionSelection("1.10.7", api, buf)
		assert.EqualError(t, err, errMock.Error())
		assert.Equal(t, "", airflowVersion)
		api.AssertExpectations(t)
	})
}

func Test_meetsAirflowUpgradeReqs(t *testing.T) {
	airflowVersion := "1.10.12"
	desiredAirflowVersion := "2.0.0"
	err := meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to 1.10.14 "+
		"first and make sure your DAGs and configs are 2.0 compatible")

	airflowVersion = "2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Error: You tried to set --desired-airflow-version to 2.0.0, but this Airflow Deployment "+
		"is already running 2.0.0. Please indicate a higher version of Airflow and try again.")

	airflowVersion = "1.10.14"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.NoError(t, err)

	airflowVersion = "1.10.7"
	desiredAirflowVersion = "1.10.10"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.NoError(t, err)

	airflowVersion = "-1.10.12"
	desiredAirflowVersion = "2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Invalid Semantic Version")

	airflowVersion = "1.10.12"
	desiredAirflowVersion = "-2.0.0"
	err = meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion)
	assert.Error(t, err)
	assert.EqualError(t, err, "Invalid Semantic Version")
}

func TestCheckNFSMountDagDeploymentError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(nil, errMock)
	assert.Equal(t, false, CheckNFSMountDagDeployment(api))
	api.AssertExpectations(t)
}

func TestCheckNFSMountDagDeploymentSuccess(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockAppConfig := &houston.AppConfig{
		Version:                "0.15.1",
		BaseDomain:             "local.astronomer.io",
		SMTPConfigured:         true,
		ManualReleaseNames:     false,
		NfsMountDagDeployment:  true,
		ConfigureDagDeployment: false,
		Flags: houston.FeatureFlags{
			ManualNamespaceNames:  false,
			NfsMountDagDeployment: true,
			HardDeleteDeployment:  false,
		},
	}
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	assert.Equal(t, true, CheckNFSMountDagDeployment(api))
	api.AssertExpectations(t)
}

func TestCheckHardDeleteDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockAppConfig := &houston.AppConfig{
		Version:              "0.15.1",
		BaseDomain:           "local.astronomer.io",
		HardDeleteDeployment: true,
		Flags: houston.FeatureFlags{
			HardDeleteDeployment: true,
		},
	}

	t.Run("check hard delete success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)

		hardDelete := CheckHardDeleteDeployment(api)
		assert.Equal(t, true, hardDelete)
		api.AssertExpectations(t)
	})

	t.Run("check hard delete error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(nil, errRegMock)

		hardDelete := CheckHardDeleteDeployment(api)
		assert.False(t, hardDelete)
		api.AssertExpectations(t)
	})
}

func TestCheckTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockAppConfig := &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(mockAppConfig, nil)

		triggererEnabled := CheckTriggererEnabled(api)
		assert.True(t, triggererEnabled)
	})

	t.Run("error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", nil).Return(nil, errRegMock)

		triggererEnabled := CheckTriggererEnabled(api)
		assert.False(t, triggererEnabled)
	})
}

func TestGetDeploymentSelectionNamespaces(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockAvailableNamespaces := []houston.Namespace{
		{Name: "test1"},
		{Name: "test2"},
	}

	t.Run("get available namespaces", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", nil).Return(mockAvailableNamespaces, nil)

		buf := new(bytes.Buffer)

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

		name, err := getDeploymentSelectionNamespaces(api, buf)
		assert.NoError(t, err)
		expected := `#     AVAILABLE KUBERNETES NAMESPACES     
1     test1                               
2     test2                               
`
		assert.Equal(t, expected, buf.String())
		assert.Equal(t, "test1", name)
		api.AssertExpectations(t)
	})

	t.Run("no namespace", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", nil).Return([]houston.Namespace{}, nil)

		buf := new(bytes.Buffer)
		name, err := getDeploymentSelectionNamespaces(api, buf)
		expected := ``
		assert.Equal(t, expected, name)
		assert.EqualError(t, err, "no kubernetes namespaces are available")
		api.AssertExpectations(t)
	})

	t.Run("parse error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", nil).Return(mockAvailableNamespaces, nil)

		buf := new(bytes.Buffer)

		// mock os.Stdin
		input := []byte("test")
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

		name, err := getDeploymentSelectionNamespaces(api, buf)
		assert.Equal(t, "", name)
		assert.EqualError(t, err, "cannot parse test to int")
		api.AssertExpectations(t)
	})

	t.Run("api error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAvailableNamespaces", nil).Return(nil, errMock)

		buf := new(bytes.Buffer)
		name, err := getDeploymentSelectionNamespaces(api, buf)
		assert.Equal(t, "", name)
		assert.EqualError(t, err, errMock.Error())
	})
}

func TestCheckPreCreateNamespacesDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockAppConfig := &houston.AppConfig{
		Flags: houston.FeatureFlags{
			ManualNamespaceNames: true,
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)

	usesPreCreateNamespace := CheckPreCreateNamespaceDeployment(api)
	assert.Equal(t, true, usesPreCreateNamespace)
	api.AssertExpectations(t)
}

func TestGetDeploymentNamespaceName(t *testing.T) {
	// mock os.Stdin
	input := []byte("Test1")
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

	name, _ := getDeploymentNamespaceName()
	assert.Equal(t, "Test1", name)
}

func TestGetDeploymentNamespaceNameError(t *testing.T) {
	// mock os.Stdin
	input := []byte("   ")
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

	name, err := getDeploymentNamespaceName()
	assert.Equal(t, "", name)
	assert.EqualError(t, err, "no kubernetes namespaces specified")
}

func TestAddDagDeploymentArgs(t *testing.T) {
	tests := []struct {
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
			sshKey:            "../../cmd/software/testfiles/ssh_key",
			knownHosts:        "../../cmd/software/testfiles/known_hosts",
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
			assert.Equal(t, tt.expectedError, err.Error())
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, output, tt.expectedOutput)
	}
}

func TestRuntimeUpgrade(t *testing.T) {
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

	t.Run("upgrade runtime success when deployment is coming from AC migration", func(t *testing.T) {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)
		buf := new(bytes.Buffer)
		err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		assert.NoError(t, err)
		expected := ` NAME        DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 IMAGE VERSION     
 test123     burning-terrestrial-5940     v0.0.0     ckbv818oa00r107606ywhoqtw     Runtime-4.2.5     

The upgrade from Runtime 4.2.4 to 4.2.5 has been started. To complete this process, add an Runtime 4.2.5 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment runtime upgrade --cancel

`

		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("upgrade runtime success if deployment was always using runtime", func(t *testing.T) {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}

		api := new(mocks.ClientInterface)
		mockDeployment.AirflowVersion = ""
		mockDeployment.DesiredAirflowVersion = ""
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)
		buf := new(bytes.Buffer)
		err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		assert.NoError(t, err)
		expected := ` NAME        DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 IMAGE VERSION     
 test123     burning-terrestrial-5940     v0.0.0     ckbv818oa00r107606ywhoqtw     Runtime-4.2.5     

The upgrade from Runtime 4.2.4 to 4.2.5 has been started. To complete this process, add an Runtime 4.2.5 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment runtime upgrade --cancel

`

		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("not on runtime", func(t *testing.T) {
		mockResp := *mockDeployment
		mockResp.AirflowVersion = "2.2.5"
		mockResp.RuntimeVersion = ""

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(&mockResp, nil)

		buf := new(bytes.Buffer)
		err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		assert.ErrorIs(t, err, errDeploymentNotOnRuntime)
	})

	t.Run("upgrade runtime get deployment error", func(t *testing.T) {
		mockError := errors.New("get deployment error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		assert.Error(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("upgrade runtime update deployment error", func(t *testing.T) {
		mockError := errors.New("update deployment error") //nolint:goerr113
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeUpgrade(mockDeployment.ID, mockDeployment.DesiredRuntimeVersion, api, buf)
		assert.Error(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("upgrade runtime with empty desired version", func(t *testing.T) {
		mockRuntimeReleases := houston.RuntimeReleases{
			houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.5"},
			houston.RuntimeRelease{Version: "4.2.5", AirflowVersion: "2.2.5"},
		}
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": mockDeployment.DesiredRuntimeVersion}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("GetRuntimeReleases", "").Return(mockRuntimeReleases, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)

		// mock os.Stdin for when prompted by getAirflowVersionSelection()
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

		buf := new(bytes.Buffer)
		err = RuntimeUpgrade(mockDeployment.ID, "", api, buf)
		t.Log(buf.String()) // Log the buffer so that this test is recognized by go test

		assert.NoError(t, err)
		expected := `#     RUNTIME VERSION     
1     Runtime-4.2.5       
 NAME        DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 IMAGE VERSION     
 test123     burning-terrestrial-5940     v0.0.0     ckbv818oa00r107606ywhoqtw     Runtime-4.2.5     

The upgrade from Runtime 4.2.4 to 4.2.5 has been started. To complete this process, add an Runtime 4.2.5 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment runtime upgrade --cancel

`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})
}

func TestRuntimeUpgradeCancel(t *testing.T) {
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

	t.Run("upgrade cancel success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		err := RuntimeUpgradeCancel(deploymentID, api, buf)
		assert.NoError(t, err)
		expected := `
Runtime upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Runtime 4.2.4.
`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("upgrade cancel get deployment error", func(t *testing.T) {
		mockError := errors.New("get deployment error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeUpgradeCancel(deploymentID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("upgrade cancel error", func(t *testing.T) {
		mockError := errors.New("update deployment error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeUpgradeCancel(deploymentID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}

func TestRuntimeMigrate(t *testing.T) {
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

	t.Run("migrate runtime success", func(t *testing.T) {
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": "4.2.4"}

		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("GetRuntimeReleases", mockDeployment.AirflowVersion).Return(houston.RuntimeReleases{houston.RuntimeRelease{Version: "4.2.3", AirflowVersion: "2.2.3"}, houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"}}, nil)
		mockMigrateRuntimeResp := *mockDeployment
		mockMigrateRuntimeResp.RuntimeVersion = "4.2.4"
		api.On("UpdateDeploymentRuntime", expectedVars).Return(&mockMigrateRuntimeResp, nil)
		buf := new(bytes.Buffer)
		err := RuntimeMigrate(mockDeployment.ID, api, buf)
		assert.NoError(t, err)
		expected := ` NAME        DEPLOYMENT NAME              ASTRO      DEPLOYMENT ID                 IMAGE VERSION     
 test123     burning-terrestrial-5940     v0.0.0     ckbv818oa00r107606ywhoqtw     Runtime-4.2.4     

The migration from Airflow 2.2.4 image to Runtime 4.2.4 has been started. To complete this process, add an Runtime 4.2.4 image to your Dockerfile and deploy to Astronomer.
To cancel, run: 
 $ astro deployment runtime migrate --cancel

`

		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("migrate runtime get deployment error", func(t *testing.T) {
		mockError := errors.New("get deployment error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeMigrate(mockDeployment.ID, api, buf)
		assert.Error(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("already on runtime error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		mockDeploymentResp := *mockDeployment
		mockDeploymentResp.RuntimeVersion = "4.2.4"
		mockDeploymentResp.AirflowVersion = ""
		api.On("GetDeployment", mockDeployment.ID).Return(&mockDeploymentResp, nil)

		buf := new(bytes.Buffer)
		err := RuntimeMigrate(mockDeployment.ID, api, buf)
		assert.Error(t, err, errDeploymentAlreadyOnRuntime)
		api.AssertExpectations(t)
	})

	t.Run("migrate runtime get runtime releases error", func(t *testing.T) {
		mockError := errors.New("get runtime releases error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("GetRuntimeReleases", mockDeployment.AirflowVersion).Return(houston.RuntimeReleases{}, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeMigrate(mockDeployment.ID, api, buf)
		assert.Error(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("invalid airflow version to migrate to runtime", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("GetRuntimeReleases", mockDeployment.AirflowVersion).Return(houston.RuntimeReleases{}, nil)

		buf := new(bytes.Buffer)
		err := RuntimeMigrate(mockDeployment.ID, api, buf)
		assert.Error(t, err, errInvalidAirflowVersion)
		api.AssertExpectations(t)
	})

	t.Run("migrate runtime update deployment error", func(t *testing.T) {
		mockError := errors.New("update deployment error") //nolint:goerr113
		expectedVars := map[string]interface{}{"deploymentUuid": mockDeployment.ID, "desiredRuntimeVersion": "4.2.4"}
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("GetRuntimeReleases", mockDeployment.AirflowVersion).Return(houston.RuntimeReleases{houston.RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"}}, nil)
		api.On("UpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeMigrate(mockDeployment.ID, api, buf)
		assert.Error(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}

func TestRuntimeMigrateCancel(t *testing.T) {
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

	t.Run("migrate cancel success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(mockDeployment, nil)

		buf := new(bytes.Buffer)
		err := RuntimeMigrateCancel(deploymentID, api, buf)
		assert.NoError(t, err)
		expected := `
Runtime migrate process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow 2.2.4.
`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})

	t.Run("migrate cancel get deployment error", func(t *testing.T) {
		mockError := errors.New("get deployment error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeMigrateCancel(deploymentID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("migrate cancel error", func(t *testing.T) {
		mockError := errors.New("update deployment error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", mockDeployment.ID).Return(mockDeployment, nil)
		api.On("CancelUpdateDeploymentRuntime", expectedVars).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := RuntimeMigrateCancel(deploymentID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})

	t.Run("already migrated error", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		mockDeploymentResp := *mockDeployment
		mockDeploymentResp.AirflowVersion = ""
		mockDeploymentResp.RuntimeVersion = mockDeployment.DesiredRuntimeVersion
		api.On("GetDeployment", mockDeployment.ID).Return(&mockDeploymentResp, nil)

		buf := new(bytes.Buffer)
		err := RuntimeMigrateCancel(deploymentID, api, buf)
		assert.NoError(t, err)
		expected := `
Nothing to cancel. You are already running Runtime 4.2.4 and you have either not indicated that you want to migrate or migration has been completed.`
		assert.Equal(t, expected, buf.String())
		api.AssertExpectations(t)
	})
}

func TestMeetsRuntimeUpgradeReqs(t *testing.T) {
	type args struct {
		runtimeVersion        string
		desiredRuntimeVersion string
	}
	tests := []struct {
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
			expectedErr: ErrInvalidRuntimeVersion{currentVersion: semver.MustParse("4.2.4"), desiredVersion: "4.2.4"},
		},
		{
			name:        "error parsing runtime version",
			args:        args{runtimeVersion: "invalid version", desiredRuntimeVersion: "4.2.5"},
			expectedErr: fmt.Errorf("Invalid Semantic Version"), //nolint
		},
		{
			name:        "error parsing desired runtime version",
			args:        args{runtimeVersion: "4.2.5", desiredRuntimeVersion: "invalid version"},
			expectedErr: fmt.Errorf("Invalid Semantic Version"), //nolint
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := meetsRuntimeUpgradeReqs(tt.args.runtimeVersion, tt.args.desiredRuntimeVersion)
			if tt.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedErr.Error())
			}
		})
	}
}
