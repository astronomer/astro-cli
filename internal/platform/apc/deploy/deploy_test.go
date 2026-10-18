package deploy

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	errSomeContainerIssue = errors.New("some container issue")
	errMockHouston        = errors.New("some houston error")
	description           = "Deployed via <astro deploy>"

	mockDeployment = &houston.Deployment{
		ID:                    "cknz133ra49758zr9w34b87ua",
		Type:                  "airflow",
		Label:                 "test",
		ReleaseName:           "testDeploymentName",
		Version:               "0.15.6",
		AirflowVersion:        "2.0.0",
		DesiredAirflowVersion: "2.0.0",
		DeploymentInfo:        houston.DeploymentInfo{},
		Workspace: houston.Workspace{
			ID:    "ckn4phn1k0104v5xtrer5lpli",
			Label: "w1",
		},
		Urls: []houston.DeploymentURL{
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
			{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
			{URL: "registry.local.astronomer.io", Type: "registry"},
		},
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}
)

var mockAirflowImageList = []houston.AirflowImage{
	{Version: "2.1.0", Tag: "2.1.0-onbuild"},
	{Version: "2.0.2", Tag: "2.0.2-onbuild"},
	{Version: "2.0.0", Tag: "2.0.0-onbuild"},
	{Version: "1.10.15", Tag: "1.10.15-onbuild"},
	{Version: "1.10.14", Tag: "1.10.14-onbuild"},
	{Version: "1.10.12", Tag: "1.10.12-onbuild"},
	{Version: "1.10.7", Tag: "1.10.7-onbuild"},
	{Version: "1.10.5", Tag: "1.10.5-onbuild"},
}

type Suite struct {
	suite.Suite
	fsForDockerConfig afero.Fs
	fsForLocalConfig  afero.Fs
	houstonMock       *houston_mocks.ClientInterface
}

func TestDeploy(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestDeploymentExists() {
	deployments := []houston.Deployment{
		{ID: "dev-test-1"},
		{ID: "dev-test-2"},
	}
	s.True(deploymentExists("dev-test-1", deployments))
}

func (s *Suite) TestDeploymentNameDoesntExists() {
	deployments := []houston.Deployment{
		{ID: "dev-test-1"},
		{ID: "dev-test-2"},
	}
	s.False(deploymentExists("dev-test", deployments))
}

func (s *Suite) SetupSuite() {
	// Common setup logic for the test suite
	s.fsForLocalConfig = afero.NewMemMapFs()
	afero.WriteFile(s.fsForLocalConfig, config.HomeConfigFile, testUtil.NewTestConfig("localhost"), 0o777)

	s.fsForDockerConfig = afero.NewMemMapFs()
	afero.WriteFile(s.fsForLocalConfig, config.HomeConfigFile, testUtil.NewTestConfig("docker"), 0o777)
}

func (s *Suite) SetupTest() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	s.houstonMock = new(houston_mocks.ClientInterface)
}

func (s *Suite) SetupSubTest() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	s.houstonMock = new(houston_mocks.ClientInterface)
}

func (s *Suite) TearDownSubTest() {
	s.houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TearDownSuite() {
	// Cleanup logic, if any (e.g., clearing mocks)
	s.houstonMock = nil
	s.fsForDockerConfig = nil
	s.fsForLocalConfig = nil
}

func (s *Suite) TearDownTest() {
	s.houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestGetAirflowUILink() {
	config.InitConfig(s.fsForDockerConfig)
	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
	}

	expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/airflow"
	actualResult := getAirflowUILink("testDeploymentID", mockURLs)
	s.Equal(expectedResult, actualResult)
}

func (s *Suite) TestGetDeploymentRegistryURL() {
	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
		{URL: "registry.local.astronomer.io", Type: "registry"},
	}
	expectedResult := "registry.local.astronomer.io"
	actualResult, _ := getDeploymentRegistryURL(mockURLs)
	s.Equal(expectedResult, actualResult)
}

func (s *Suite) TestGetDeploymentRegistryURLFailure() {
	mockURLs := []houston.DeploymentURL{
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: "airflow"},
		{URL: "https://deployments.local.astronomer.io/testDeploymentName/flower", Type: "flower"},
	}
	_, err := getDeploymentRegistryURL(mockURLs)
	s.EqualError(err, "no valid registry url found failed to push")
}

func (s *Suite) TestGetAirflowUILinkFailure() {
	actualResult := getAirflowUILink("", []houston.DeploymentURL{})
	s.Equal(actualResult, "")

	config.InitConfig(s.fsForLocalConfig)

	actualResult = getAirflowUILink("testDeploymentID", []houston.DeploymentURL{})
	s.Equal(actualResult, "")
}

func (s *Suite) TestGetDagDeployURL() {
	s.Run("Returns dagserver URL when available in deployment", func() {
		mockDeployment := &houston.Deployment{
			ReleaseName: "test-deployment",
			Urls: []houston.DeploymentURL{
				{URL: "https://deployments.local.astronomer.io/testDeploymentName/dags/upload", Type: houston.DagServerURLType},
				{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: houston.AirflowURLType},
				{URL: "registry.local.astronomer.io", Type: "registry"},
			},
		}

		expectedResult := "https://deployments.local.astronomer.io/testDeploymentName/dags/upload"
		actualResult := getDagDeployURL(mockDeployment)
		s.Equal(expectedResult, actualResult)
	})

	s.Run("Constructs URL from airflow URL when dagserver URL not available in deployment", func() {
		mockDeployment := &houston.Deployment{
			ReleaseName: "test-deployment",
			Urls: []houston.DeploymentURL{
				{URL: "https://deployments.local.astronomer.io/testDeploymentName/airflow", Type: houston.AirflowURLType},
				{URL: "registry.local.astronomer.io", Type: "registry"},
			},
		}

		expectedResult := "https://deployments.local.astronomer.io/test-deployment/dags/upload"
		actualResult := getDagDeployURL(mockDeployment)
		s.Equal(expectedResult, actualResult)
	})

	s.Run("Returns empty string when no valid URLs available", func() {
		mockDeployment := &houston.Deployment{
			ReleaseName: "test-deployment",
			Urls: []houston.DeploymentURL{
				{URL: "https://flower.example.com", Type: "flower"},
				{URL: "registry.local.astronomer.io", Type: "registry"},
			},
		}

		expectedResult := ""
		actualResult := getDagDeployURL(mockDeployment)
		s.Equal(expectedResult, actualResult)
	})
}

func (s *Suite) TestDeployDagsOnlyFailure() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deploymentID := "test-deployment-id"
	wsID := "test-workspace-id"
	// A project with a dags directory, so each refusal below is Houston's.
	project := s.T().TempDir()
	s.Require().NoError(os.Mkdir(filepath.Join(project, "dags"), 0o755))
	prevWorkingPath := config.WorkingPath
	config.WorkingPath = project
	defer func() { config.WorkingPath = prevWorkingPath }()

	s.Run("No dags directory, to a Deployment that takes uploads: refused, and nothing is uploaded", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		uploads := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uploads++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		uploadURL := server.URL
		dagOnly := &houston.Deployment{ID: deploymentID, DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}
		enabled := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
		// A file named dags is not a dags directory either.
		withFile := s.T().TempDir()
		s.Require().NoError(os.WriteFile(filepath.Join(withFile, "dags"), nil, 0o600))
		for _, parent := range []string{s.T().TempDir(), filepath.Join(project, "dags", "missing"), withFile} {
			s.houstonMock.On("GetDeployment", deploymentID).Return(dagOnly, nil).Once()
			s.houstonMock.On("GetAppConfig", mock.Anything).Return(enabled, nil).Once()
			got, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, parent, &uploadURL, true, description, Options{})
			s.ErrorIs(err, ErrNoDagsDirectory, parent)
			s.ErrorContains(err, filepath.Join(parent, "dags")+" is not a directory. Nothing was uploaded, and the Deployment keeps the Dags it had")
			s.Equal(deploymentID, got)
		}
		s.Zero(uploads)
	})

	// The refusals come before the dags directory is looked for: a
	// Deployment or cluster that takes no upload says what to change, and a
	// deploy that goes on past the image does not say an upload was skipped
	// for want of a directory, when none would have been made with one.
	s.Run("No dags directory: the refusals still come first", func() {
		noDags := s.T().TempDir()
		gitSync := &houston.Deployment{ID: deploymentID, ReleaseName: "rel", DagDeployment: houston.DagDeploymentConfig{Type: houston.GitSyncDeploymentType}}
		image := &houston.Deployment{ID: deploymentID, ReleaseName: "rel", DagDeployment: houston.DagDeploymentConfig{Type: houston.ImageDeploymentType}}
		// Before 0.29.0 no Deployment is read with its type.
		untyped := &houston.Deployment{ID: deploymentID, ReleaseName: "rel"}
		dagOnly := &houston.Deployment{ID: deploymentID, ReleaseName: "rel", DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}
		enabled := &houston.AppConfig{Version: "1.0.0", Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
		disabled := &houston.AppConfig{Version: "2.0.0"}
		legacy := &houston.AppConfig{Version: "0.25.0"}
		for _, tc := range []struct {
			name       string
			deployment *houston.Deployment
			appConfig  *houston.AppConfig
			want       error
		}{
			{"a git-sync Deployment", gitSync, enabled, ErrDagOnlyDeployNotEnabledForDeployment},
			{"an image Deployment", image, enabled, ErrDagOnlyDeployNotEnabledForDeployment},
			{"a Deployment on 0.25.0", untyped, legacy, ErrDagOnlyDeployDisabledInConfigLegacy},
			{"a cluster without DAG-only deploys", dagOnly, disabled, ErrDagOnlyDeployDisabledInConfig},
		} {
			workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
				return nil, nil
			}
			s.houstonMock.On("GetDeployment", deploymentID).Return(tc.deployment, nil).Once()
			s.houstonMock.On("GetAppConfig", mock.Anything).Return(tc.appConfig, nil).Once()
			_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, noDags, nil, true, description, Options{})
			s.ErrorIs(err, tc.want, tc.name)
			s.NotErrorIs(err, ErrNoDagsDirectory, tc.name)
		}
		// No Deployment to deploy to.
		workspaceDeploymentsVar = workspaceDeployments
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, "", noDags, nil, true, description, Options{})
		s.ErrorIs(err, errInvalidDeploymentID)
	})

	// The directory is found after Houston is asked, and the bundle is made
	// from it: one gone meanwhile is not uploaded as an empty bundle.
	s.Run("A dags directory removed mid-deploy is refused, and nothing is uploaded", func() {
		uploads := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uploads++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		uploadURL := server.URL
		dagOnly := &houston.Deployment{ID: deploymentID, DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}
		enabled := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}

		parent := s.T().TempDir()
		dags := filepath.Join(parent, "dags")
		s.Require().NoError(os.Mkdir(dags, 0o755))
		s.Require().NoError(os.WriteFile(filepath.Join(dags, "dag.py"), nil, 0o600))
		// Removed while the Deployment is looked up.
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			s.Require().NoError(os.RemoveAll(dags))
			return nil, nil
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(dagOnly, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(enabled, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, parent, &uploadURL, true, description, Options{
			WarnEmptyDags: func(string) { s.Fail("warned of no DAGs, for a dags directory that is gone") },
		})
		s.ErrorIs(err, ErrNoDagsDirectory)
		s.Zero(uploads)
	})

	s.Run("A dags directory removed before the bundle is made is refused, and nothing is uploaded", func() {
		uploads := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uploads++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		uploadURL := server.URL
		dagOnly := &houston.Deployment{ID: deploymentID, DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}
		enabled := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}

		parent := s.T().TempDir()
		dags := filepath.Join(parent, "dags")
		s.Require().NoError(os.Mkdir(dags, 0o755))
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(dagOnly, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(enabled, nil).Once()
		// An empty dags directory is warned of; the directory goes while
		// the warning is given, before Tar.
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, parent, &uploadURL, true, description, Options{
			WarnEmptyDags: func(string) { s.Require().NoError(os.RemoveAll(dags)) },
		})
		s.ErrorIs(err, ErrNoDagsDirectory)
		s.Zero(uploads)
		_, statErr := os.Stat(filepath.Join(parent, "dags.tar"))
		s.True(os.IsNotExist(statErr), "the empty bundle is removed")
	})

	s.Run("When config flag is set to false on Houston before 2.0.0", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}

		appConfig := &houston.AppConfig{
			Version: "1.0.0",
			Flags:   *featureFlags,
		}
		deployment := &houston.Deployment{
			ClusterID: "test-cluster-id",
			ID:        deploymentID,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, ErrDagOnlyDeployDisabledInConfigLegacy)
	})

	s.Run("When config flag is set to false on Houston 2.0.0+", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: false,
		}

		appConfig := &houston.AppConfig{
			Version: "2.0.0",
			Flags:   *featureFlags,
		}
		deployment := &houston.Deployment{
			ClusterID: "test-cluster-id",
			ID:        deploymentID,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, ErrDagOnlyDeployDisabledInConfig)
	})

	s.Run("When the workspace's Deployments cannot be listed", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, errMockHouston
		}
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, errMockHouston)
	})

	s.Run("When config flag is set to true but an error occurs in the GetDeployment api call", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(nil, errMockHouston).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorContains(err, "failed to get deployment info: some houston error")
	})

	s.Run("When config flag is set to true but it is disabled at the deployment level", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.VolumeDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
			ID:            deploymentID,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, ErrDagOnlyDeployNotEnabledForDeployment)
	})

	s.Run("Valid Houston config, but unable to get context from astro-cli config", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		config.ResetCurrentContext()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.EqualError(err, "could not get current context! Error: no context set, have you authenticated to Astro or APC? Run astro login and try again")
		context.Switch("localhost")
	})

	s.Run("Valid Houston config, able to get context from config but no release name present", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, config.WorkingPath, nil, false, description, Options{})
		s.ErrorIs(err, errInvalidDeploymentID)
	})
	// An empty dags folder is warned of, and uploaded: there is no question.
	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is empty: warned of, and uploaded", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			s.Equal(http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			s.NoError(err, "Error parsing multipart form")
			s.NotNil(r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", &server.URL, false, description, Options{})
		s.NoError(err)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was created
		destFilePath = "./dags.tar.gz"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading gZipped file")
		defer os.Remove(destFilePath)
	})

	// An empty folder asks nothing, so it deploys under a guard that refuses
	// every question; the Deployment is the one returned; the upload's notes
	// go to Progress, and the warning to WarnEmptyDags.
	s.Run("An empty folder asks nothing, and the Deployment is returned", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		appConfig := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType},
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", "picked-id").Return(deployment, nil).Once()
		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		s.Require().NoError(os.Mkdir("dags", os.ModePerm))
		defer os.RemoveAll("dags")
		defer os.Remove("./dags.tar")
		defer os.Remove("./dags.tar.gz")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		defer server.Close()
		// Under a guard that refuses every question, so asking would fail.
		defer input.SetGuard(func() string { return "with --output json it cannot" })()

		progress := new(bytes.Buffer)
		var warned []string
		got, err := DagsOnlyDeploy(s.houstonMock, wsID, "picked-id", ".", &server.URL, false, description, Options{Progress: progress, WarnEmptyDags: func(w string) { warned = append(warned, w) }})
		s.NoError(err)
		s.Equal("picked-id", got)
		s.Contains(progress.String(), "upload successful")
		s.Equal([]string{"no Dags found in the dags folder: the upload replaces the Deployment's Dags with none"}, warned)
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. Tar creation throws an error", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		// A directory where the tar goes: it cannot be created.
		parent := s.T().TempDir()
		s.Require().NoError(os.Mkdir(filepath.Join(parent, "dags"), 0o755))
		s.Require().NoError(os.WriteFile(filepath.Join(parent, "dags", "dag.py"), nil, 0o600))
		s.Require().NoError(os.Mkdir(filepath.Join(parent, "dags.tar"), 0o755))
		_, err := DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, parent, nil, false, description, Options{})
		s.ErrorContains(err, filepath.Join(parent, "dags.tar"))

		// assert that no gz file exists
		_, err = os.Stat(filepath.Join(parent, "dags.tar.gz"))
		s.True(os.IsNotExist(err))
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. Tar is successfully created. But gzip creation throws an error", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		gzipMockError := errors.New("some gzip error")

		// mock the gzip creation to throw an error
		gzipFile = func(srcFilePath, destFilePath string) error {
			return gzipMockError
		}

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", nil, false, description, Options{})
		s.ErrorIs(err, gzipMockError)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was not created
		destFilePath = "./dags.tar.gz"
		_, err = os.Stat(destFilePath)
		s.True(os.IsNotExist(err))

		gzipFile = fileutil.GzipFile
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. No need of User confirmation", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			s.Equal(http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			s.NoError(err, "Error parsing multipart form")
			s.NotNil(r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", &server.URL, false, description, Options{})
		s.NoError(err)

		// Validate that dags.tar file was created
		destFilePath := "./dags.tar"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading tar file")
		defer os.Remove(destFilePath)

		// Validate that dags.tar.gz file was created
		destFilePath = "./dags.tar.gz"
		_, err = os.ReadFile(destFilePath)
		s.NoError(err, "Error reading gZipped file")
		defer os.Remove(destFilePath)
	})

	s.Run("Valid Houston config. Valid Houston deployment. The Dags folder is non-empty. No need of User confirmation. Files should be auto-cleaned", func() {
		workspaceDeploymentsVar = func(houston.ClientInterface, string, string) ([]houston.Deployment, error) {
			return nil, nil
		}
		featureFlags := &houston.FeatureFlags{
			DagOnlyDeployment: true,
		}
		appConfig := &houston.AppConfig{
			Flags: *featureFlags,
		}
		dagDeployment := &houston.DagDeploymentConfig{
			Type: houston.DagOnlyDeploymentType,
		}
		deployment := &houston.Deployment{
			ReleaseName:   "testReleaseName",
			DagDeployment: *dagDeployment,
			ClusterID:     "test-cluster-id",
		}
		s.houstonMock.On("GetDeployment", deploymentID).Return(deployment, nil).Once()

		// create the non-empty dags folder
		err := os.Mkdir("dags", os.ModePerm)
		s.NoError(err)
		defer os.RemoveAll("dags")
		fileContent := []byte("print('Hello, World!')")
		err = os.WriteFile("./dags/test.py", fileContent, os.ModePerm)
		s.NoError(err)

		// Prepare a test server to capture the request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Assert the request method is POST
			s.Equal(http.MethodPost, r.Method)

			// Assert the correct form field name
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			s.NoError(err, "Error parsing multipart form")
			s.NotNil(r.MultipartForm.File["file"], "Form file not found in request")

			// Respond with a success status code
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		s.houstonMock.On("GetAppConfig", mock.Anything).Return(appConfig, nil).Once()
		_, err = DagsOnlyDeploy(s.houstonMock, wsID, deploymentID, ".", &server.URL, true, description, Options{})
		s.NoError(err)

		// assert that no tar or gz file exists
		_, err = os.Stat("./dags.tar")
		s.True(os.IsNotExist(err))
		_, err = os.Stat("./dags.tar.gz")
		s.True(os.IsNotExist(err))
	})
}

// dagsFrom places a Deployment's DAGs by the tests DagsOnlyDeploy refuses on.
func TestDagsFrom(t *testing.T) {
	enabled := &houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}
	disabled := &houston.AppConfig{}
	// read is whether the query that read the Deployment asked for its type.
	const read, notRead = true, false
	for _, tc := range []struct {
		name      string
		typ       string
		appConfig *houston.AppConfig
		read      bool
		want      DagsFrom
	}{
		{"an image Deployment", houston.ImageDeploymentType, enabled, read, DagsFromImage},
		{"an image Deployment on a cluster without DAG-only deploys", houston.ImageDeploymentType, disabled, read, DagsFromImage},
		{"a DAG-only Deployment", houston.DagOnlyDeploymentType, enabled, read, DagsFromUpload},
		// The cluster refuses the upload, so it is not one.
		{"a DAG-only Deployment on a cluster without DAG-only deploys", houston.DagOnlyDeploymentType, disabled, read, DagsFromElsewhere},
		// Whether the cluster takes it is not known.
		{"a DAG-only Deployment and no cluster config", houston.DagOnlyDeploymentType, nil, read, DagsFromUnknown},
		{"a git-sync Deployment", houston.GitSyncDeploymentType, enabled, read, DagsFromElsewhere},
		{"a volume Deployment", houston.VolumeDeploymentType, disabled, read, DagsFromElsewhere},
		// No type is Houston's default, the image, on any cluster:
		// DagsOnlyDeploy refuses it as it does an image Deployment.
		{"no type on a cluster without DAG-only deploys", "", disabled, read, DagsFromImage},
		{"no type and no cluster config", "", nil, read, DagsFromImage},
		{"no type on a cluster with DAG-only deploys", "", enabled, read, DagsFromImage},
		// GetDeployment asks for no type before 0.29.0, so every Deployment
		// reads as one with none: not placed.
		{"no type, not asked for", "", enabled, notRead, DagsFromUnknown},
		{"no type, not asked for, on a cluster without DAG-only deploys", "", disabled, notRead, DagsFromUnknown},
		// A type this CLI does not know is not placed either.
		{"an unknown type", "dag-only", enabled, read, DagsFromUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &houston.Deployment{DagDeployment: houston.DagDeploymentConfig{Type: tc.typ}, DagDeploymentRead: tc.read}
			assert.Equal(t, tc.want, dagsFrom(info, tc.appConfig))
		})
	}
	assert.Equal(t, DagsFromUnknown, dagsFrom(nil, enabled), "no Deployment")
	var zero DagsFrom
	assert.Equal(t, DagsFromUnknown, zero, "the zero value is not placed")
}
