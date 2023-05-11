package airflow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"

	testUtils "github.com/astronomer/astro-cli/pkg/testing"
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
)

var airflowVersionLabel = "2.2.5"

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
	configYaml := testUtils.NewTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	_, err := DockerComposeInit("./testfiles", "", "Dockerfile", "")
	assert.NoError(t, err)
}

func TestDockerComposeStart(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	waitTime := 1 * time.Second
	t.Run("success", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Twice()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Twice()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, waitTime)
		assert.NoError(t, err)

		err = mockDockerCompose.Start("custom-image", "", "", noCache, false, waitTime)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with shorter default startup time", func(t *testing.T) {
		defaultTimeOut := 1 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		mockIsM1 := func(myOS, myArch string) bool {
			return false
		}
		isM1 = mockIsM1

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			assert.Equal(t, defaultTimeOut, timeout)
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, defaultTimeOut)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with longer default startup time", func(t *testing.T) {
		defaultTimeOut := 1 * time.Minute
		expectedTimeout := 5 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		mockIsM1 := func(myOS, myArch string) bool {
			return true
		}
		isM1 = mockIsM1

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			assert.Equal(t, expectedTimeout, timeout)
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, defaultTimeOut)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with user provided startup time", func(t *testing.T) {
		userProvidedTimeOut := 8 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			assert.Equal(t, userProvidedTimeOut, timeout)
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, userProvidedTimeOut)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success with invalid airflow version label", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: "2.3.4.dev+astro1"}, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Twice()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Twice()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, waitTime)
		assert.NoError(t, err)

		err = mockDockerCompose.Start("custom-image", "", "", noCache, false, waitTime)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("project already running", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Start("", "", "", false, false, waitTime)
		assert.Contains(t, err.Error(), "cannot start, project already running")

		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Start("", "", "", false, false, waitTime)
		assert.ErrorIs(t, err, errMockDocker)

		composeMock.AssertExpectations(t)
	})

	t.Run("image build failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, waitTime)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("list label failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, waitTime)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("compose up failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(errMockDocker).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, waitTime)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("webserver health check failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Twice()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
			return errMockDocker
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", noCache, false, waitTime)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposeExport(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop()
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("list label failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop()
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

		err := mockDockerCompose.Stop()
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})
}

func TestDockerComposePS(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, []string{}, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "")

		assert.NoError(t, err)
		assert.Equal(t, "", resp)
		imageHandler.AssertExpectations(t)
	})

	t.Run("unexpected exit code", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("1", nil).Once()

		mockResponse := "1"
		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "")
		assert.Contains(t, err.Error(), "something went wrong while Pytesting your DAGs")
		assert.Equal(t, mockResponse, resp)
		imageHandler.AssertExpectations(t)
	})

	t.Run("image build failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "", "", "")
		assert.ErrorIs(t, err, errMockDocker)
		imageHandler.AssertExpectations(t)
	})
}

func TestDockerComposeParse(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test", airflowHome: "./testfiles"}
	t.Run("success", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("0", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test")
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("exit code 1", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("1", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test")
		assert.Contains(t, err.Error(), "See above for errors detected in your DAGs")
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("exit code 2", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return("2", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test")
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

		err := mockDockerCompose.Parse("", "test")
		assert.NoError(t, err)

		w.Close()
		out, _ := io.ReadAll(r)

		assert.Contains(t, string(out), "does not exist. Please run `astro dev init` to create it")
	})

	t.Run("invalid file name", func(t *testing.T) {
		DefaultTestPath = "\x0004"

		err := mockDockerCompose.Parse("", "test")
		assert.Contains(t, err.Error(), "invalid argument")
	})
}

func TestDockerComposeBash(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("import success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		initSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
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
		initSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
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
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success with container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-scheduler-id", State: "running", Name: "test-scheduler"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", noCache, false)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("error with container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-scheduler-id", State: "running", Name: "test-scheduler"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("success without container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", noCache, false)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("error without container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("build error without container", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("PS error without container", func(t *testing.T) {
		noCache := false
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.RunDAG("", "", "", noCache, false)
		assert.ErrorIs(t, err, errMockDocker)

		composeMock.AssertExpectations(t)
	})
}

func TestCheckWebserverHealth(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
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
		initSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return nil
		}
		defer func() { initSettings = orgInitSetting }()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := checkWebserverHealth(settingsFile, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
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
		initSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return nil
		}
		defer func() { initSettings = orgInitSetting }()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		// set config to podman
		config.CFG.DockerCommand.SetHomeString("podman")
		err := checkWebserverHealth(settingsFile, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
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

		err := checkWebserverHealth(settingsFile, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
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

		err := checkWebserverHealth(settingsFile, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
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

		err := checkWebserverHealth(settingsFile, &types.Project{Name: "test"}, composeMock, 2, false, 1*time.Second)
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
	configYaml := testUtils.NewTestConfig(testUtils.LocalPlatform)
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
