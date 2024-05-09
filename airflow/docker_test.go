package airflow

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"

	"github.com/astronomer/astro-cli/config"
	"github.com/sirupsen/logrus"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	docker_types "github.com/docker/docker/api/types"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMockDocker   = errors.New("mock docker compose error")
	errMockSettings = errors.New("mock Settings error")
	//go:embed testfiles/pip_freeze_new-version.txt
	pipFreezeFile string
	//go:embed testfiles/pip_freeze_old-version.txt
	pipFreezeFile2 string
)

var (
	airflowVersionLabel        = "2.2.5"
	deploymentID               = "test-deployment-id"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:     deploymentID,
			Status: "HEALTHY",
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
	mockGetDeploymentsResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id: deploymentID,
		},
	}
)

func TestRepositoryName(t *testing.T) {
	assert.Equal(t, repositoryName("test-repo"), "test-repo/airflow")
}

func TestImageName(t *testing.T) {
	assert.Equal(t, ImageName("test-repo", "0.15.0"), "test-repo/airflow:0.15.0")
}

func TestCheckServiceStateTrue(t *testing.T) {
	assert.True(t, checkServiceState("RUNNING test", "RUNNING"))
}

func TestCheckServiceStateFalse(t *testing.T) {
	assert.False(t, checkServiceState("RUNNING test", "FAILED"))
}

func TestGenerateConfig(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.LocalPlatform)
	err := afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	assert.NoError(t, err)
	config.InitConfig(fs)
	t.Run("returns config with default healthcheck", func(t *testing.T) {
		expectedCfg := `version: '3.4'

x-common-env-vars: &common-env-vars
  AIRFLOW__CORE__EXECUTOR: LocalExecutor
  AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
  AIRFLOW__DATABASE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
  AIRFLOW__CORE__LOAD_EXAMPLES: "False"
  AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
  AIRFLOW__WEBSERVER__SECRET_KEY: "test-project-name"
  AIRFLOW__WEBSERVER__RBAC: "True"
  AIRFLOW__WEBSERVER__EXPOSE_CONFIG: "True"
  ASTRONOMER_ENVIRONMENT: local

networks:
  airflow:
    driver: bridge

volumes:
  postgres_data:
    driver: local
  airflow_logs:
    driver: local

services:
  postgres:
    image: docker.io/postgres:12.6
    restart: unless-stopped
    networks:
      - airflow
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
    ports:
      - 127.0.0.1:5432:5432
    volumes:
      
      - postgres_data:/var/lib/postgresql/data
      
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres

  scheduler:
    image: test-project-name/airflow:latest
    command: >
      bash -c "(airflow db upgrade || airflow upgradedb) && airflow scheduler"
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-scheduler"
    depends_on:
      - postgres
    environment: *common-env-vars
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z

      
      - airflow_logs:/usr/local/airflow/logs
      
    

  webserver:
    image: test-project-name/airflow:latest
    command: >
      bash -c 'if [[ -z "$$AIRFLOW__API__AUTH_BACKEND" ]] && [[ $$(pip show -f apache-airflow | grep basic_auth.py) ]];
        then export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.basic_auth ;
        else export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.default ; fi &&
        { airflow users create "$$@" || airflow create_user "$$@" ; } &&
        { airflow sync-perm || airflow sync_perm ;} &&
        airflow webserver' -- -r Admin -u admin -e admin@example.com -f admin -l user -p admin
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-webserver"
    depends_on:
      - scheduler
      - postgres
    environment: *common-env-vars
    ports:
      - 127.0.0.1:8080:8080
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z
      
      - airflow_logs:/usr/local/airflow/logs
      
    healthcheck:
      test: curl --fail http://webserver:8080/health || exit 1
      interval: 2s
      retries: 15
      start_period: 5s
      timeout: 60s
    

`
		mockM1Checker := func(myOS, myArch string) bool {
			return false
		}
		isM1 = mockM1Checker
		cfg, err := generateConfig("test-project-name", "airflow_home", ".env", "", "airflow_settings.yaml", map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, expectedCfg, cfg)
	})
	t.Run("returns config with triggerer enabled", func(t *testing.T) {
		expectedCfg := `version: '3.4'

x-common-env-vars: &common-env-vars
  AIRFLOW__CORE__EXECUTOR: LocalExecutor
  AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
  AIRFLOW__DATABASE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
  AIRFLOW__CORE__LOAD_EXAMPLES: "False"
  AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
  AIRFLOW__WEBSERVER__SECRET_KEY: "test-project-name"
  AIRFLOW__WEBSERVER__RBAC: "True"
  AIRFLOW__WEBSERVER__EXPOSE_CONFIG: "True"
  ASTRONOMER_ENVIRONMENT: local

networks:
  airflow:
    driver: bridge

volumes:
  postgres_data:
    driver: local
  airflow_logs:
    driver: local

services:
  postgres:
    image: docker.io/postgres:12.6
    restart: unless-stopped
    networks:
      - airflow
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
    ports:
      - 127.0.0.1:5432:5432
    volumes:
      
      - postgres_data:/var/lib/postgresql/data
      
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres

  scheduler:
    image: test-project-name/airflow:latest
    command: >
      bash -c "(airflow db upgrade || airflow upgradedb) && airflow scheduler"
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-scheduler"
    depends_on:
      - postgres
    environment: *common-env-vars
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z

      
      - airflow_logs:/usr/local/airflow/logs
      
    

  webserver:
    image: test-project-name/airflow:latest
    command: >
      bash -c 'if [[ -z "$$AIRFLOW__API__AUTH_BACKEND" ]] && [[ $$(pip show -f apache-airflow | grep basic_auth.py) ]];
        then export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.basic_auth ;
        else export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.default ; fi &&
        { airflow users create "$$@" || airflow create_user "$$@" ; } &&
        { airflow sync-perm || airflow sync_perm ;} &&
        airflow webserver' -- -r Admin -u admin -e admin@example.com -f admin -l user -p admin
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-webserver"
    depends_on:
      - scheduler
      - postgres
    environment: *common-env-vars
    ports:
      - 127.0.0.1:8080:8080
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z
      
      - airflow_logs:/usr/local/airflow/logs
      
    healthcheck:
      test: curl --fail http://webserver:8080/health || exit 1
      interval: 2s
      retries: 15
      start_period: 5s
      timeout: 60s
    

  triggerer:
    image: test-project-name/airflow:latest
    command: >
      bash -c "(airflow db upgrade || airflow upgradedb) && airflow triggerer"
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-triggerer"
    depends_on:
      - postgres
    environment: *common-env-vars
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      
      - airflow_logs:/usr/local/airflow/logs
      
    

`
		mockM1Checker := func(myOS, myArch string) bool {
			return false
		}
		isM1 = mockM1Checker
		cfg, err := generateConfig("test-project-name", "airflow_home", ".env", "", "airflow_settings.yaml", map[string]string{runtimeVersionLabelName: triggererAllowedRuntimeVersion})
		assert.NoError(t, err)
		assert.Equal(t, expectedCfg, cfg)
	})
}

func TestCheckTriggererEnabled(t *testing.T) {
	t.Run("astro-runtime supported version", func(t *testing.T) {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{runtimeVersionLabelName: triggererAllowedRuntimeVersion})
		assert.NoError(t, err)
		assert.True(t, triggererEnabled)
	})

	t.Run("astro-runtime unsupported version", func(t *testing.T) {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{runtimeVersionLabelName: "3.0.0"})
		assert.NoError(t, err)
		assert.False(t, triggererEnabled)
	})

	t.Run("astronomer-certified supported version", func(t *testing.T) {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{airflowVersionLabelName: "2.4.0-onbuild"})
		assert.NoError(t, err)
		assert.True(t, triggererEnabled)
	})

	t.Run("astronomer-certified unsupported version", func(t *testing.T) {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{airflowVersionLabelName: "2.1.0"})
		assert.NoError(t, err)
		assert.False(t, triggererEnabled)
	})
}

func TestDockerComposeInit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := DockerComposeInit("./testfiles", "", "Dockerfile", "")
	assert.NoError(t, err)
}

func TestDockerComposeStart(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	waitTime := 1 * time.Second
	t.Run("success", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Twice()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Twice()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, waitTime, nil)
		assert.NoError(t, err)

		err = mockDockerCompose.Start("custom-image", "", "", "", noCache, false, waitTime, nil)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with shorter default startup time", func(t *testing.T) {
		defaultTimeOut := 1 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		mockIsM1 := func(myOS, myArch string) bool {
			return false
		}
		isM1 = mockIsM1

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			assert.Equal(t, defaultTimeOut, timeout)
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, defaultTimeOut, nil)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with longer default startup time", func(t *testing.T) {
		defaultTimeOut := 1 * time.Minute
		expectedTimeout := 5 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		mockIsM1 := func(myOS, myArch string) bool {
			return true
		}
		isM1 = mockIsM1

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			assert.Equal(t, expectedTimeout, timeout)
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, defaultTimeOut, nil)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with user provided startup time", func(t *testing.T) {
		userProvidedTimeOut := 8 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			assert.Equal(t, userProvidedTimeOut, timeout)
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, userProvidedTimeOut, nil)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with invalid airflow version label", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: "2.3.4.dev+astro1"}, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Twice()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Twice()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, waitTime, nil)
		assert.NoError(t, err)

		err = mockDockerCompose.Start("custom-image", "", "", "", noCache, false, waitTime, nil)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("project already running", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Start("", "", "", "", false, false, waitTime, nil)
		assert.Contains(t, err.Error(), "cannot start, project already running")

		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Start("", "", "", "", false, false, waitTime, nil)
		assert.ErrorIs(t, err, errMockDocker)

		composeMock.AssertExpectations(t)
	})

	t.Run("image build failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, waitTime, nil)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("list label failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, waitTime, nil)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("compose up failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(errMockDocker).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, waitTime, nil)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("webserver health check failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Twice()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return errMockDocker
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, false, waitTime, nil)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeExport(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test", airflowHome: "/home/airflow", envFile: "/home/airflow/.env"}

	t.Run("success", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("settings.yaml", "docker-compose.yaml")
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return(nil, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("settings.yaml", "docker-compose.yaml")
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("list label failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("settings.yaml", "docker-compose.yaml")
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
	})

	t.Run("generate yaml failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("", "")
		assert.ErrorContains(t, err, "failed to write to compose file")

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeStop(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop(false)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with wait but on first try", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "exited"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logrus.SetLevel(5) // debug level
		var out bytes.Buffer
		logrus.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		assert.NoError(t, err)

		assert.Contains(t, out.String(), "postgres container reached exited state")
		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success after waiting for once", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "running"}}, nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "exited"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logrus.SetLevel(5) // debug level
		var out bytes.Buffer
		logrus.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		assert.NoError(t, err)

		assert.Contains(t, out.String(), "postgres container is still in running state, waiting for it to be in exited state")
		assert.Contains(t, out.String(), "postgres container reached exited state")
		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("time out during the wait for postgres exit", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "running"}}, nil)

		// reducing timeout
		stopPostgresWaitTimeout = 11 * time.Millisecond
		stopPostgresWaitTicker = 10 * time.Millisecond

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logrus.SetLevel(5) // debug level
		var out bytes.Buffer
		logrus.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		assert.NoError(t, err)

		assert.Contains(t, out.String(), "postgres container is still in running state, waiting for it to be in exited state")
		assert.Contains(t, out.String(), "timed out waiting for postgres container to be in exited state")
		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("list label failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop(false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
	})

	t.Run("compose stop failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop(false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposePS(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running", Publishers: api.PortPublishers{{PublishedPort: 8080}}}}, nil).Once()

		mockDockerCompose.composeService = composeMock

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := mockDockerCompose.PS()
		assert.NoError(t, err)

		w.Close()
		out, _ := io.ReadAll(r)

		assert.Contains(t, string(out), "test-webserver")
		assert.Contains(t, string(out), "running")
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.PS()
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeKill(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Down", mock.Anything, mockDockerCompose.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Kill()
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("compose down failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Down", mock.Anything, mockDockerCompose.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Kill()
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	containerNames := []string{WebserverDockerContainerName, SchedulerDockerContainerName, TriggererDockerContainerName}
	follow := false
	t.Run("success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		composeMock.On("Logs", mock.Anything, mockDockerCompose.projectName, mock.Anything, api.LogOptions{Services: containerNames, Follow: follow}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})

	t.Run("project not running", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		assert.Contains(t, err.Error(), "cannot view logs, project not running")
		composeMock.AssertExpectations(t)
	})

	t.Run("compose logs failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		composeMock.On("Logs", mock.Anything, mockDockerCompose.projectName, mock.Anything, api.LogOptions{Services: containerNames, Follow: follow}).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeRun(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		testCmd := []string{"test", "command"}
		str := bytes.NewReader([]byte(`0`))
		mockResp := bufio.NewReader(str)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", docker_types.ExecConfig{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(docker_types.IDResponse{ID: "test-exec-id"}, nil).Once()
		mockCLIClient.On("ContainerExecAttach", context.Background(), "test-exec-id", docker_types.ExecStartCheck{Detach: false}).Return(docker_types.HijackedResponse{Reader: mockResp}, nil).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
		mockCLIClient.AssertExpectations(t)
	})

	t.Run("exec id is empty", func(t *testing.T) {
		testCmd := []string{"test", "command"}
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", docker_types.ExecConfig{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(docker_types.IDResponse{}, nil).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		assert.Contains(t, err.Error(), "exec ID is empty")
		composeMock.AssertExpectations(t)
		mockCLIClient.AssertExpectations(t)
	})

	t.Run("container not running", func(t *testing.T) {
		testCmd := []string{"test", "command"}
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", docker_types.ExecConfig{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(docker_types.IDResponse{}, errMockDocker).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		assert.Contains(t, err.Error(), "airflow is not running. To start a local Airflow environment, run 'astro dev start'")
		composeMock.AssertExpectations(t)
		mockCLIClient.AssertExpectations(t)
	})

	t.Run("get webserver container id failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Run([]string{"test", "command"}, "test-user")
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposePytest(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", "")

		assert.NoError(t, err)
		assert.Equal(t, "", resp)
		imageHandler.AssertExpectations(t)
	})

	t.Run("success custom image", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "custom-image-name", "", "", "")

		assert.NoError(t, err)
		assert.Equal(t, "", resp)
		imageHandler.AssertExpectations(t)
	})

	t.Run("unexpected exit code", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("1", nil).Once()

		mockResponse := "1"
		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", "")
		assert.Contains(t, err.Error(), "something went wrong while Pytesting your DAGs")
		assert.Equal(t, mockResponse, resp)
		imageHandler.AssertExpectations(t)
	})

	t.Run("image build failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "", "", "", "")
		assert.ErrorIs(t, err, errMockDocker)
		imageHandler.AssertExpectations(t)
	})
}

func TestDockerComposedUpgradeTest(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cwd, err := os.Getwd()
	assert.NoError(t, err)
	mockDockerCompose := DockerCompose{projectName: "test", dockerfile: "Dockerfile", airflowHome: cwd}

	pipFreeze := "upgrade-test-old-version--new-version/pip_freeze_old-version.txt"
	pipFreeze2 := "upgrade-test-old-version--new-version/pip_freeze_new-version.txt"
	parseTest := cwd + "/.astro/test_dag_integrity_default.py"
	oldDockerFile := cwd + "/Dockerfile"
	// Write files out
	err = fileutil.WriteStringToFile(pipFreeze, pipFreezeFile)
	assert.NoError(t, err)
	err = fileutil.WriteStringToFile(pipFreeze2, pipFreezeFile2)
	assert.NoError(t, err)
	err = fileutil.WriteStringToFile(parseTest, "")
	assert.NoError(t, err)
	err = fileutil.WriteStringToFile(oldDockerFile, "")
	assert.NoError(t, err)

	defer afero.NewOsFs().Remove(pipFreeze)
	defer afero.NewOsFs().Remove(pipFreeze2)
	defer afero.NewOsFs().Remove(parseTest)
	defer afero.NewOsFs().Remove("upgrade-test-old-version--new-version/Dockerfile")
	defer afero.NewOsFs().Remove("upgrade-test-old-version--new-version/dependency_compare.txt")
	defer afero.NewOsFs().Remove("upgrade-test-old-version--new-version")
	defer afero.NewOsFs().Remove(oldDockerFile)

	t.Run("success no deployment id", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Times(3)
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze2).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)

		assert.NoError(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("success with deployment id", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Twice()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentsResponse, nil).Once()
		imageHandler.On("Pull", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Times(2)
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze2).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.UpgradeTest("new-version", "test-deployment-id", "", "", "", true, false, false, mockPlatformCoreClient)

		assert.NoError(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("image build failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("GetLabel failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", errMockDocker)

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("GetLabel failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", errMockDocker)

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("ConflictTest failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("Create old pip freeze failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("build new image failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(nil).Once()
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("build new image for pytest failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Times(2)
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze2).Return(nil).Once()
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("pytest failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Times(3)
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze2).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(1, errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("get deployments failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Twice()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentsResponse, nil).Once()
		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "deployment-id", "", "", "", true, false, false, mockPlatformCoreClient)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("image pull failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Twice()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentsResponse, nil).Once()
		imageHandler.On("Pull", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker)

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "test-deployment-id", "", "", "", true, false, false, mockPlatformCoreClient)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("build new image failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Twice()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("ConflictTest", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("", nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, cwd+"/"+pipFreeze2).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err = mockDockerCompose.UpgradeTest("new-version", "", "", "", "", true, false, false, nil)
		assert.Error(t, err)
		imageHandler.AssertExpectations(t)
	})

	t.Run("no domain", func(t *testing.T) {
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		err = mockDockerCompose.UpgradeTest("new-version", "deployment-id", "", "", "", false, false, false, nil)
		assert.Error(t, err)
	})
}

func TestDockerComposeParse(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test", airflowHome: "./testfiles"}
	t.Run("success", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", "")
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("exit code 1", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("1", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", "")
		assert.Contains(t, err.Error(), "See above for errors detected in your DAGs")
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("exit code 2", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("2", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", "")
		assert.Contains(t, err.Error(), "something went wrong while parsing your DAGs")
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("file does not exists", func(t *testing.T) {
		DefaultTestPath = "test_invalid_file.py"

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := mockDockerCompose.Parse("", "test", "")
		assert.NoError(t, err)

		w.Close()
		out, _ := io.ReadAll(r)

		assert.Contains(t, string(out), "does not exist. Please run `astro dev init` to create it")
	})

	t.Run("invalid file name", func(t *testing.T) {
		DefaultTestPath = "\x0004"

		err := mockDockerCompose.Parse("", "test", "")
		assert.Contains(t, err.Error(), "invalid argument")
	})
}

func TestDockerComposeBash(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	container := "scheduler"
	t.Run("success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(container)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("Bash error", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(container)
		assert.Contains(t, err.Error(), errMock.Error())
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(container)
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})

	t.Run("project not running", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(container)
		assert.Contains(t, err.Error(), "cannot exec into container, project not running")
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeSettings(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("import success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		initSettings = func(id, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, version uint64, connections, variables, pools bool) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("import failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		initSettings = func(id, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, version uint64, connections, variables, pools bool) error {
			return errMockSettings
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", false, false, false)
		assert.ErrorIs(t, err, errMockSettings)
		composeMock.AssertExpectations(t)
	})

	t.Run("export success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		exportSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("export failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		exportSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return errMockSettings
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", false, false, false, false)
		assert.ErrorIs(t, err, errMockSettings)
		composeMock.AssertExpectations(t)
	})

	t.Run("env export success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		envExportSettings = func(id, settingsFile string, version uint64, connections, variables bool) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, true)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("env export failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		envExportSettings = func(id, settingsFile string, version uint64, connections, variables bool) error {
			return errMockSettings
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, true)
		assert.ErrorIs(t, err, errMockSettings)
		composeMock.AssertExpectations(t)
	})

	t.Run("list lables import error", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMock).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		assert.Contains(t, err.Error(), errMock.Error())
		composeMock.AssertExpectations(t)
	})

	t.Run("list lables export error", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMock).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		assert.Contains(t, err.Error(), errMock.Error())
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure import", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure export", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})

	t.Run("project not running import", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		assert.Contains(t, err.Error(), "project not running, run astro dev start to start project")
		composeMock.AssertExpectations(t)
	})

	t.Run("project not running export", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		assert.Contains(t, err.Error(), "project not running, run astro dev start to start project")
		composeMock.AssertExpectations(t)
	})

	t.Run("file does not exist import", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings_invalid.yaml", ".env", true, true, true)
		assert.Contains(t, err.Error(), "file specified does not exist")
		composeMock.AssertExpectations(t)
	})

	t.Run("file does not exist export", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings_invalid.yaml", ".env", true, true, true, false)
		assert.Contains(t, err.Error(), "file specified does not exist")
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeRunDAG(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success with container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-scheduler-id", State: "running", Name: "test-scheduler"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, false)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("error with container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-scheduler-id", State: "running", Name: "test-scheduler"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success without container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, false)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("error without container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("build error without container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("PS error without container", func(t *testing.T) {
		noCache := false
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		composeMock.AssertExpectations(t)
	})
}

func TestCheckWebserverHealth(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		settingsFile := "docker_test.go" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: fmt.Sprintf("test-%s", WebserverDockerContainerName), State: "running"}}, nil).Once()
		mockEventsCall := composeMock.On("Events", mock.AnythingOfType("*context.timerCtx"), "test", mock.Anything)
		mockEventsCall.RunFn = func(args mock.Arguments) {
			consumer := args.Get(2).(api.EventsOptions).Consumer
			err := consumer(api.Event{Status: "exec_create"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_start"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_die"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "health_status: healthy"})
			assert.ErrorIs(t, err, errComposeProjectRunning)
			mockEventsCall.ReturnArguments = mock.Arguments{err}
		}

		openURL = func(url string) error {
			return nil
		}

		orgInitSetting := initSettings
		initSettings = func(id, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, version uint64, connections, variables, pools bool) error {
			return nil
		}
		defer func() { initSettings = orgInitSetting }()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := checkWebserverHealth(settingsFile, nil, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
		assert.NoError(t, err)

		w.Close()
		out, _ := io.ReadAll(r)
		assert.Contains(t, string(out), "Project is running! All components are now available.")
	})

	t.Run("success with podman", func(t *testing.T) {
		settingsFile := "docker_test.go" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: fmt.Sprintf("test-%s", WebserverDockerContainerName), State: "running"}}, nil).Once()

		openURL = func(url string) error {
			return nil
		}

		orgInitSetting := initSettings
		initSettings = func(id, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, version uint64, connections, variables, pools bool) error {
			return nil
		}
		defer func() { initSettings = orgInitSetting }()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		// set config to podman
		config.CFG.DockerCommand.SetHomeString("podman")
		err := checkWebserverHealth(settingsFile, nil, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
		assert.NoError(t, err)

		w.Close()
		out, _ := io.ReadAll(r)
		assert.Contains(t, string(out), "Components will be available soon.")
	})

	// set config to docker
	config.CFG.DockerCommand.SetHomeString("docker")

	t.Run("compose ps failure", func(t *testing.T) {
		settingsFile := "./testfiles/test_dag_inegrity_file.py" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()
		mockEventsCall := composeMock.On("Events", mock.AnythingOfType("*context.timerCtx"), "test", mock.Anything)
		mockEventsCall.RunFn = func(args mock.Arguments) {
			consumer := args.Get(2).(api.EventsOptions).Consumer
			err := consumer(api.Event{Status: "exec_create"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_start"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_die"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "health_status: healthy"})
			assert.ErrorIs(t, err, errMockDocker)
			mockEventsCall.ReturnArguments = mock.Arguments{err}
		}

		openURL = func(url string) error {
			return nil
		}

		err := checkWebserverHealth(settingsFile, nil, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
		assert.ErrorIs(t, err, errMockDocker)
	})

	t.Run("timeout waiting for webserver to get to healthy with short timeout", func(t *testing.T) {
		settingsFile := "./testfiles/test_dag_inegrity_file.py" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: fmt.Sprintf("test-%s", WebserverDockerContainerName), State: "exec_die"}}, nil).Once()
		mockEventsCall := composeMock.On("Events", mock.AnythingOfType("*context.timerCtx"), "test", mock.Anything)
		mockEventsCall.RunFn = func(args mock.Arguments) {
			consumer := args.Get(2).(api.EventsOptions).Consumer
			err := consumer(api.Event{Status: "exec_create"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_start"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_die"})
			assert.NoError(t, err)
			err = context.DeadlineExceeded
			mockEventsCall.ReturnArguments = mock.Arguments{err}
		}

		openURL = func(url string) error {
			return nil
		}
		mockIsM1 := func(myOS, myArch string) bool {
			return false
		}
		isM1 = mockIsM1

		err := checkWebserverHealth(settingsFile, nil, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
		assert.ErrorContains(t, err, "The webserver health check timed out after 1s")
	})
	t.Run("timeout waiting for webserver to get to healthy with long timeout", func(t *testing.T) {
		settingsFile := "./testfiles/test_dag_inegrity_file.py" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: fmt.Sprintf("test-%s", WebserverDockerContainerName), State: "exec_die"}}, nil).Once()
		mockEventsCall := composeMock.On("Events", mock.AnythingOfType("*context.timerCtx"), "test", mock.Anything)
		mockEventsCall.RunFn = func(args mock.Arguments) {
			consumer := args.Get(2).(api.EventsOptions).Consumer
			err := consumer(api.Event{Status: "exec_create"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_start"})
			assert.NoError(t, err)
			err = consumer(api.Event{Status: "exec_die"})
			assert.NoError(t, err)
			err = context.DeadlineExceeded
			mockEventsCall.ReturnArguments = mock.Arguments{err}
		}

		openURL = func(url string) error {
			return nil
		}
		mockIsM1 := func(myOS, myArch string) bool {
			return true
		}
		isM1 = mockIsM1

		err := checkWebserverHealth(settingsFile, nil, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
		assert.ErrorContains(t, err, "The webserver health check timed out after 1s")
	})
}

var errExecMock = errors.New("docker is not running")

func TestStartDocker(t *testing.T) {
	t.Run("start docker success", func(t *testing.T) {
		counter := 0
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch cmd {
			case "open":
				return nil
			case "docker":
				if counter == 0 {
					counter++
					return errExecMock
				}
				return nil
			default:
				return errExecMock
			}
		}

		err := startDocker()
		assert.NoError(t, err)
	})

	t.Run("start docker fail", func(t *testing.T) {
		timeoutNum = 5

		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch cmd {
			case "open":
				return nil
			case "docker":
				return errExecMock
			default:
				return errExecMock
			}
		}
		err := startDocker()
		assert.Contains(t, err.Error(), "timed out waiting for docker")
	})
}

func TestCreateDockerProject(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.LocalPlatform)
	err := afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	assert.NoError(t, err)
	config.InitConfig(fs)
	t.Run("case when project doesnot have docker-compose.override.yml", func(t *testing.T) {
		prj, err := createDockerProject("test", "", "", "test-image:latest", "", "", map[string]string{})
		assert.NoError(t, err)
		postgresService := types.ServiceConfig{}
		serviceFound := false
		for _, service := range prj.Services {
			if service.Name == "webserver" {
				postgresService = service
				serviceFound = true
				break
			}
		}
		assert.True(t, serviceFound)
		assert.Equal(t, "test-image:latest", postgresService.Image)
	})

	t.Run("case when project has docker-compose.override.yml", func(t *testing.T) {
		composeOverrideFilename = "./testfiles/docker-compose.override.yml"
		prj, err := createDockerProject("test", "", "", "test-image:latest", "", "", map[string]string{})
		assert.NoError(t, err)
		postgresService := types.ServiceConfig{}
		serviceFound := false
		for _, service := range prj.Services {
			if service.Name == "postgres" {
				postgresService = service
				serviceFound = true
				break
			}
		}
		assert.True(t, serviceFound)
		assert.Equal(t, "postgres", postgresService.Name)
		assert.Equal(t, 5433, int(postgresService.Ports[len(prj.Services[0].Ports)-1].Published))
	})
}

func TestUpgradeDockerfile(t *testing.T) {
	t.Run("update Dockerfile with new tag", func(t *testing.T) {
		// Create a temporary old Dockerfile
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/astro-runtime:old-tag\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		assert.NoError(t, err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "new-tag"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		assert.NoError(t, err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		assert.NoError(t, err)
		assert.Contains(t, string(newContent), "FROM quay.io/astronomer/astro-runtime:new-tag")
	})

	t.Run("update Dockerfile with new image", func(t *testing.T) {
		// Create a temporary old Dockerfile
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/astro-runtime:old-tag\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		assert.NoError(t, err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newImage := "new-image"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, "", newImage)
		defer os.Remove(newDockerfilePath)

		// Check for errors
		assert.NoError(t, err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		assert.NoError(t, err)
		assert.Contains(t, string(newContent), "FROM new-image")
	})

	t.Run("update Dockerfile for ap-airflow with runtime version", func(t *testing.T) {
		// Create a temporary old Dockerfile with a line matching the pattern
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/ap-airflow:old-tag"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		assert.NoError(t, err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "5.0.0"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		assert.NoError(t, err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		assert.NoError(t, err)
		assert.Contains(t, string(newContent), "FROM quay.io/astronomer/astro-runtime:5.0.0\n")
	})

	t.Run("update Dockerfile for ap-airflow with non-runtime version", func(t *testing.T) {
		// Create a temporary old Dockerfile with a line matching the pattern
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/ap-airflow:old-tag\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		assert.NoError(t, err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "new-tag"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		assert.NoError(t, err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		assert.NoError(t, err)
		assert.Contains(t, string(newContent), "FROM quay.io/astronomer/ap-airflow:new-tag")
	})

	t.Run("error reading old Dockerfile", func(t *testing.T) {
		// Define test data with an invalid path
		oldDockerfilePath := "non_existent_Dockerfile"
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "new-tag"

		// Call the function
		err := upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	})
}
