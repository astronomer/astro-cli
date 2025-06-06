package airflow

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	docker_types "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
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
	runtimeVersionLabel        = "12.0.0"
	labels                     = map[string]string{airflowVersionLabelName: airflowVersionLabel, runtimeVersionLabelName: runtimeVersionLabel}
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

func (s *Suite) TestRepositoryName() {
	s.Equal(repositoryName("test-repo"), "test-repo/airflow")
}

func (s *Suite) TestImageName() {
	s.Equal(ImageName("test-repo", "0.15.0"), "test-repo/airflow:0.15.0")
}

func (s *Suite) TestCheckServiceStateTrue() {
	s.True(checkServiceState("RUNNING test", "RUNNING"))
}

func (s *Suite) TestCheckServiceStateFalse() {
	s.False(checkServiceState("RUNNING test", "FAILED"))
}

func (s *Suite) TestGenerateConfig() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.LocalPlatform)
	err := afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	s.NoError(err)
	config.InitConfig(fs)

	s.Run("returns config with default healthcheck", func() {
		expectedCfg := `x-common-env-vars: &common-env-vars
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
		cfg, err := generateConfig("test-project-name", "airflow_home", ".env", "", "airflow_settings.yaml", map[string]string{
			runtimeVersionLabelName: runtimeVersionLabel,
		})
		s.NoError(err)
		s.Equal(expectedCfg, cfg)
	})

	s.Run("returns config with triggerer enabled", func() {
		expectedCfg := `x-common-env-vars: &common-env-vars
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
		cfg, err := generateConfig("test-project-name", "airflow_home", ".env", "", "airflow_settings.yaml", map[string]string{runtimeVersionLabelName: triggererAllowedRuntimeVersion})
		s.NoError(err)
		s.Equal(expectedCfg, cfg)
	})

	s.Run("returns config for Airflow 3 runtime image", func() {
		expectedCfg := `x-common-env-vars: &common-env-vars
  AIRFLOW__API__BASE_URL: "http://localhost:8080"
  AIRFLOW__API__PORT: 8080
  AIRFLOW__API_AUTH__JWT_SECRET: "test-project-name"
  AIRFLOW__CORE__AUTH_MANAGER: airflow.api_fastapi.auth.managers.simple.simple_auth_manager.SimpleAuthManager
  AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS: "True"
  AIRFLOW__CORE__EXECUTION_API_SERVER_URL: "http://api-server:8080/execution/"
  AIRFLOW__CORE__EXECUTOR: LocalExecutor
  AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
  AIRFLOW__CORE__LOAD_EXAMPLES: "False"
  AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
  AIRFLOW__DATABASE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
  AIRFLOW__SCHEDULER__STANDALONE_DAG_PROCESSOR: True
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

  db-migration:
    depends_on:
      - postgres
    image: test-project-name/airflow:latest
    command:
      - airflow
      - db
      - migrate
    networks:
      - airflow
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-db-migration"
    environment: *common-env-vars

  scheduler:
    depends_on:
      - db-migration
    image: test-project-name/airflow:latest
    command:
      - airflow
      - scheduler
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-scheduler"
    environment: *common-env-vars
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z
      
      
      - airflow_logs:/usr/local/airflow/logs
      
    

  dag-processor:
    depends_on:
      - db-migration
    image: test-project-name/airflow:latest
    command:
      - airflow
      - dag-processor
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-dag-processor"
    environment: *common-env-vars
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z
      
      - airflow_logs:/usr/local/airflow/logs
      
    

  api-server:
    depends_on:
      - db-migration
    image: test-project-name/airflow:latest
    command:
      - airflow
      - api-server
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-api-server"
    environment: *common-env-vars
    ports:
      - 127.0.0.1:8080:8080
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_home/tests:/usr/local/airflow/tests:z
      
      - airflow_logs:/usr/local/airflow/logs
      
    

  triggerer:
    depends_on:
      - db-migration
    image: test-project-name/airflow:latest
    command:
      - airflow
      - triggerer
    restart: unless-stopped
    networks:
      - airflow
    user: astro
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
      io.astronomer.docker.component: "airflow-triggerer"
    environment: *common-env-vars
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      
      - airflow_logs:/usr/local/airflow/logs
      
    
`
		cfg, err := generateConfig("test-project-name", "airflow_home", ".env", "", "airflow_settings.yaml", map[string]string{runtimeVersionLabelName: "3.0-1"})
		s.NoError(err)
		s.Equal(expectedCfg, cfg)
	})
}

func (s *Suite) TestCheckTriggererEnabled() {
	s.Run("astro-runtime supported version", func() {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{runtimeVersionLabelName: triggererAllowedRuntimeVersion})
		s.NoError(err)
		s.True(triggererEnabled)
	})

	s.Run("astro-runtime unsupported version", func() {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{runtimeVersionLabelName: "3.0.0"})
		s.NoError(err)
		s.False(triggererEnabled)
	})

	s.Run("astronomer-certified supported version", func() {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{airflowVersionLabelName: "2.4.0-onbuild"})
		s.NoError(err)
		s.True(triggererEnabled)
	})

	s.Run("astronomer-certified unsupported version", func() {
		triggererEnabled, err := CheckTriggererEnabled(map[string]string{airflowVersionLabelName: "2.1.0"})
		s.NoError(err)
		s.False(triggererEnabled)
	})
}

func (s *Suite) TestDockerComposeInit() {
	_, err := DockerComposeInit("./testfiles", "", "Dockerfile", "")
	s.NoError(err)
}

func (s *Suite) TestDockerComposeStart() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	waitTime := 1 * time.Second
	s.Run("success", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Times(2)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, waitTime, nil)
		s.NoError(err)

		err = mockDockerCompose.Start("custom-image", "", "", "", noCache, true, waitTime, nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with shorter default startup time", func() {
		defaultTimeOut := 1 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Times(1)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			s.Equal(defaultTimeOut, timeout)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, defaultTimeOut, nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with longer default startup time", func() {
		expectedTimeout := 10 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Times(1)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			s.Equal(expectedTimeout, timeout)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, expectedTimeout, nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with user provided startup time", func() {
		userProvidedTimeOut := 8 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Times(1)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			s.Equal(userProvidedTimeOut, timeout)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, userProvidedTimeOut, nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with invalid airflow version label", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: "2.3.4.dev+astro1", runtimeVersionLabelName: runtimeVersionLabel}, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Times(2)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, waitTime, nil)
		s.NoError(err)

		err = mockDockerCompose.Start("custom-image", "", "", "", noCache, true, waitTime, nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("image build failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, waitTime, nil)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("list label failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, waitTime, nil)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose up failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, waitTime, nil)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("webserver health check failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Twice()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return errMockDocker
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", "", "", noCache, true, waitTime, nil)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeExport() {
	mockDockerCompose := DockerCompose{projectName: "test", airflowHome: "/home/airflow", envFile: "/home/airflow/.env"}

	s.Run("success", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("settings.yaml", "docker-compose.yaml")
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure", func() {
		imageHandler := new(mocks.ImageHandler)
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return(nil, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("settings.yaml", "docker-compose.yaml")
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
	})

	s.Run("list label failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("settings.yaml", "docker-compose.yaml")
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
	})

	s.Run("generate yaml failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.Anything, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ComposeExport("", "")
		s.ErrorContains(err, "failed to write to compose file")

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeStop() {
	mockDockerCompose := DockerCompose{projectName: "test"}

	s.Run("success", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop(false)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with wait but on first try", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "exited"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logger.SetLevel(5) // debug level
		var out bytes.Buffer
		logger.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		s.NoError(err)

		s.Contains(out.String(), "postgres container reached exited state")
		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success after waiting for once", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "running"}}, nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "exited"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logger.SetLevel(5) // debug level
		var out bytes.Buffer
		logger.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		s.NoError(err)

		s.Contains(out.String(), "postgres container is still in running state, waiting for it to be in exited state")
		s.Contains(out.String(), "postgres container reached exited state")
		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("time out during the wait for postgres exit", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-postgres", Name: "test-postgres", State: "running"}}, nil)

		// reducing timeout
		stopPostgresWaitTimeout = 11 * time.Millisecond
		stopPostgresWaitTicker = 10 * time.Millisecond

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logger.SetLevel(5) // debug level
		var out bytes.Buffer
		logger.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		s.NoError(err)

		s.Contains(out.String(), "postgres container is still in running state, waiting for it to be in exited state")
		s.Contains(out.String(), "timed out waiting for postgres container to be in exited state")
		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("list label failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop(false)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
	})

	s.Run("compose stop failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Stop", mock.Anything, mock.Anything, api.StopOptions{}).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Stop(false)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposePS() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	s.Run("success", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running", Publishers: api.PortPublishers{{PublishedPort: 8080}}}}, nil).Once()

		mockDockerCompose.composeService = composeMock

		r, w, _ := os.Pipe()
		os.Stdout = w

		err := mockDockerCompose.PS()
		s.NoError(err)

		w.Close()
		out, _ := io.ReadAll(r)

		s.Contains(string(out), "test-webserver")
		s.Contains(string(out), "running")
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.PS()
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeKill() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	s.Run("success", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Down", mock.Anything, mockDockerCompose.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Kill()
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose down failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Down", mock.Anything, mockDockerCompose.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Kill()
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeLogs() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	containerNames := []string{WebserverDockerContainerName, SchedulerDockerContainerName, TriggererDockerContainerName}
	follow := false
	s.Run("success", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		composeMock.On("Logs", mock.Anything, mockDockerCompose.projectName, mock.Anything, api.LogOptions{Services: containerNames, Follow: follow}).Return(nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("project not running", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		s.Contains(err.Error(), "cannot view logs, project not running")
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose logs failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		composeMock.On("Logs", mock.Anything, mockDockerCompose.projectName, mock.Anything, api.LogOptions{Services: containerNames, Follow: follow}).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Logs(follow, containerNames...)
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeRun() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	s.Run("success", func() {
		testCmd := []string{"test", "command"}
		str := bytes.NewReader([]byte(`0`))
		mockResp := bufio.NewReader(str)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(docker_types.IDResponse{ID: "test-exec-id"}, nil).Once()
		mockCLIClient.On("ContainerExecAttach", context.Background(), "test-exec-id", container.ExecStartOptions{Detach: false}).Return(docker_types.HijackedResponse{Reader: mockResp}, nil).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
		mockCLIClient.AssertExpectations(s.T())
	})

	s.Run("exec id is empty", func() {
		testCmd := []string{"test", "command"}
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(docker_types.IDResponse{}, nil).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		s.Contains(err.Error(), "exec ID is empty")
		composeMock.AssertExpectations(s.T())
		mockCLIClient.AssertExpectations(s.T())
	})

	s.Run("container not running", func() {
		testCmd := []string{"test", "command"}
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(docker_types.IDResponse{}, errMockDocker).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		s.Contains(err.Error(), "airflow is not running. To start a local Airflow environment, run 'astro dev start'")
		composeMock.AssertExpectations(s.T())
		mockCLIClient.AssertExpectations(s.T())
	})

	s.Run("get webserver container id failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Run([]string{"test", "command"}, "test-user")
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposePytest() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	s.Run("success", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", "")

		s.NoError(err)
		s.Equal("", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("success custom image", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "custom-image-name", "", "", "")

		s.NoError(err)
		s.Equal("", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("unexpected exit code", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("1", nil).Once()

		mockResponse := "1"
		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", "")
		s.Contains(err.Error(), "something went wrong while Pytesting your DAGs")
		s.Equal(mockResponse, resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("image build failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "", "", "", "")
		s.ErrorIs(err, errMockDocker)
		imageHandler.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeUpgradeTest() {
	cwd := s.T().TempDir()
	mockDockerCompose := DockerCompose{projectName: "test", dockerfile: "Dockerfile", airflowHome: cwd}

	pipFreeze := filepath.Join(cwd, "upgrade-test-old-version--new-version", "pip_freeze_old-version.txt")
	pipFreeze2 := filepath.Join(cwd, "upgrade-test-old-version--new-version", "pip_freeze_new-version.txt")
	parseTest := filepath.Join(cwd, ".astro", "test_dag_integrity_default.py")
	oldDockerFile := filepath.Join(cwd, "Dockerfile")
	// Ensure directories exist
	err := os.MkdirAll(filepath.Dir(pipFreeze), 0o777)
	s.NoError(err)
	err = os.MkdirAll(filepath.Dir(parseTest), 0o777)
	s.NoError(err)
	// Write files out
	err = fileutil.WriteStringToFile(pipFreeze, pipFreezeFile)
	s.NoError(err)
	err = fileutil.WriteStringToFile(pipFreeze2, pipFreezeFile2)
	s.NoError(err)
	err = fileutil.WriteStringToFile(parseTest, "")
	s.NoError(err)
	err = fileutil.WriteStringToFile(oldDockerFile, "")
	s.NoError(err)

	s.Run("success no deployment id", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Times(3)
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze2).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil) // All tests enabled by default

		s.NoError(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("success with deployment id", func() {
		imageHandler := new(mocks.ImageHandler)

		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Twice()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentsResponse, nil).Once()
		imageHandler.On("Pull", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Times(2)
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze2).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "test-deployment-id", "", "", true, true, true, false, "", mockPlatformCoreClient) // All tests enabled by default

		s.NoError(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("image build failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil)
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("GetLabel failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", errMockDocker)

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil)
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("Create old pip freeze failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil) // versionTest=true is required for this path
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("build new image failure (version test)", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(nil).Once()
		// This tests failure during version comparison phase
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil) // versionTest=true is required for this path
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("build new image failure (dag test)", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Times(2) // 1st for version, 2nd for version compare
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze2).Return(nil).Once()
		// This tests failure during dag test phase
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil) // dagTest=true is required for this path
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("pytest failure (dag test)", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Times(3) // 1st for version, 2nd for version compare, 3rd for dag test
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze2).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("1", errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, true, true, false, "", nil) // dagTest=true is required for this path
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("get deployments failure", func() {
		imageHandler := new(mocks.ImageHandler)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once() // Error on first call

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "deployment-id", "", "", false, false, false, false, "", mockPlatformCoreClient)
		s.Error(err)
		// No image handler expectations needed as it fails before pull/build
	})

	s.Run("image pull failure", func() {
		imageHandler := new(mocks.ImageHandler)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Twice()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentsResponse, nil).Once()
		imageHandler.On("Pull", mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker)

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "test-deployment-id", "", "", false, false, false, false, "", mockPlatformCoreClient)
		s.Error(err)
		imageHandler.AssertExpectations(s.T()) // Only Pull is called
	})

	s.Run("Create new pip freeze failure", func() { // Renamed from "build new image failure" for clarity
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Times(2) // 1st for version, 2nd for version compare
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze).Return(nil).Once()
		imageHandler.On("CreatePipFreeze", mock.Anything, pipFreeze2).Return(errMockDocker).Once() // Error here

		mockDockerCompose.imageHandler = imageHandler
		// Add default values for new lint flags
		err := mockDockerCompose.UpgradeTest("new-version", "", "", "", true, false, false, false, "", nil) // versionTest=true is required for this path
		s.Error(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("no domain", func() {
		// Reset context assumes config package handles this correctly
		// No need to save/restore, just reset and expect failure
		err := config.ResetCurrentContext()
		s.NoError(err)

		// Add default values for new lint flags
		err = mockDockerCompose.UpgradeTest("new-version", "deployment-id", "", "", false, false, false, false, "", nil)
		s.Error(err) // Expect error due to missing context/domain
	})

	// --- Lint Test Cases ---

	s.Run("success with lint test (default)", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "Dockerfile", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)

		ruffImageHandler := new(mocks.ImageHandler)
		ruffImageHandler.On("Pull", "", "", "").Return(nil).Once()
		ruffImageHandler.On("RunCommand", []string{"check", "--config", "/app/ruff.toml", "/app/project"}, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.ruffImageHandler = ruffImageHandler
		// Call with lintTest=true, includeLintDeprecations=false, lintConfigFile=""
		err := mockDockerCompose.UpgradeTest("3.0-1", "", "", "", false, false, true, false, "", nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		ruffImageHandler.AssertExpectations(s.T())
	})

	s.Run("success with lint test including deprecations", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "Dockerfile", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)

		ruffImageHandler := new(mocks.ImageHandler)
		ruffImageHandler.On("Pull", "", "", "").Return(nil).Once()
		ruffImageHandler.On("RunCommand", []string{"check", "--config", "/app/ruff.toml", "/app/project"}, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.ruffImageHandler = ruffImageHandler
		// Call with lintTest=true, includeLintDeprecations=true, lintConfigFile=""
		err := mockDockerCompose.UpgradeTest("3.0-1", "", "", "", false, false, true, true, "", nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		ruffImageHandler.AssertExpectations(s.T())
	})

	s.Run("success with lint test using provided config file", func() {
		// Create a dummy config file
		dummyConfigFile := filepath.Join(cwd, "my-custom-ruff.toml")
		err := fileutil.WriteStringToFile(dummyConfigFile, "[lint]\nselect=[\"DUMMY\"]\n")
		s.NoError(err)
		defer os.Remove(dummyConfigFile) // Clean up dummy file

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "Dockerfile", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)

		ruffImageHandler := new(mocks.ImageHandler)
		// Expect RunCommand, specifically check that the provided config file and dags dir are mounted
		expectedMounts := map[string]string{
			cwd:             "/app/project",   // Use the dummy dags dir path from cwd
			dummyConfigFile: "/app/ruff.toml", // Check this mapping
		}
		ruffImageHandler.On("Pull", "", "", "").Return(nil).Once()
		ruffImageHandler.On("RunCommand", []string{"check", "--config", "/app/ruff.toml", "/app/project"}, expectedMounts, mock.Anything, mock.Anything).Return(nil).Once()

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.ruffImageHandler = ruffImageHandler
		// Call with lintTest=true, includeLintDeprecations=false, lintConfigFile=path/to/dummy
		// Temporarily set config.WorkingPath to cwd for this test case to match mount logic
		originalWorkingPath := config.WorkingPath
		config.WorkingPath = cwd
		defer func() { config.WorkingPath = originalWorkingPath }()
		err = mockDockerCompose.UpgradeTest("3.0-1", "", "", "", false, false, true, false, "my-custom-ruff.toml", nil)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		ruffImageHandler.AssertExpectations(s.T())
	})

	s.Run("lint test failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "Dockerfile", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)

		ruffImageHandler := new(mocks.ImageHandler)
		ruffImageHandler.On("Pull", "", "", "").Return(nil).Once()
		ruffImageHandler.On("RunCommand", []string{"check", "--config", "/app/ruff.toml", "/app/project"}, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.ruffImageHandler = ruffImageHandler
		// Call with lintTest=true, includeLintDeprecations=false, lintConfigFile=""
		err := mockDockerCompose.UpgradeTest("3.0-1", "", "", "", false, false, true, false, "", nil)
		s.Error(err)
		s.Contains(err.Error(), "one of the tests run above failed")

		imageHandler.AssertExpectations(s.T())
		ruffImageHandler.AssertExpectations(s.T())
	})

	s.Run("lint test skipped for Airflow 2", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "Dockerfile", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("GetLabel", mock.Anything, mock.Anything).Return("old-version", nil)

		ruffImageHandler := new(mocks.ImageHandler) // Lint handler should not be called

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.ruffImageHandler = ruffImageHandler
		// Call with lintTest=true, includeLintDeprecations=false, lintConfigFile=""
		// Target version is 2.0.0, so lint test should be skipped internally
		err := mockDockerCompose.UpgradeTest("2.0.0", "", "", "", false, false, true, false, "", nil)
		s.NoError(err) // Should succeed without running lint

		imageHandler.AssertExpectations(s.T())
		ruffImageHandler.AssertExpectations(s.T()) // Verify RunCommand was not called
	})
}

func (s *Suite) TestDockerComposeParse() {
	defer func(orig string) {
		DefaultTestPath = orig
	}(DefaultTestPath)

	mockDockerCompose := DockerCompose{projectName: "test", airflowHome: "./testfiles"}
	s.Run("success", func() {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", "")
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("exit code 1", func() {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("1", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", "")
		s.Contains(err.Error(), "See above for errors detected in your DAGs")
		composeMock.AssertExpectations(s.T())
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("exit code 2", func() {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("2", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", "")
		s.Contains(err.Error(), "something went wrong while parsing your DAGs")
		composeMock.AssertExpectations(s.T())
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("file does not exists", func() {
		DefaultTestPath = "test_invalid_file.py"

		r, w, _ := os.Pipe()
		os.Stdout = w

		err := mockDockerCompose.Parse("", "test", "")
		s.NoError(err)

		w.Close()
		out, _ := io.ReadAll(r)

		s.Contains(string(out), "does not exist. Please run `astro dev init` to create it")
	})

	s.Run("invalid file name", func() {
		DefaultTestPath = "\x0004"

		err := mockDockerCompose.Parse("", "test", "")
		s.Contains(err.Error(), "invalid argument")
	})
}

func (s *Suite) TestDockerComposeBash() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	component := "scheduler"
	s.Run("success", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(component)
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("Bash error", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(component)
		s.Contains(err.Error(), errMock.Error())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(component)
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("project not running", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Bash(component)
		s.Contains(err.Error(), "cannot exec into container, project not running")
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeSettings() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	s.Run("import success", func() {
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
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("import failure", func() {
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
		s.ErrorIs(err, errMockSettings)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("export success", func() {
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
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("export failure", func() {
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
		s.ErrorIs(err, errMockSettings)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("env export success", func() {
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
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("env export failure", func() {
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
		s.ErrorIs(err, errMockSettings)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("list lables import error", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMock).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		s.Contains(err.Error(), errMock.Error())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("list lables export error", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMock).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		s.Contains(err.Error(), errMock.Error())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure import", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure export", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		s.ErrorIs(err, errMockDocker)
		composeMock.AssertExpectations(s.T())
	})

	s.Run("project not running import", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		s.Contains(err.Error(), "project not running, run astro dev start to start project")
		composeMock.AssertExpectations(s.T())
	})

	s.Run("project not running export", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		s.Contains(err.Error(), "project not running, run astro dev start to start project")
		composeMock.AssertExpectations(s.T())
	})

	s.Run("file does not exist import", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings_invalid.yaml", ".env", true, true, true)
		s.Contains(err.Error(), "file specified does not exist")
		composeMock.AssertExpectations(s.T())
	})

	s.Run("file does not exist export", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running", Name: "test-webserver"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings_invalid.yaml", ".env", true, true, true, false)
		s.Contains(err.Error(), "file specified does not exist")
		composeMock.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerComposeRunDAG() {
	mockDockerCompose := DockerCompose{projectName: "test"}
	s.Run("success with container", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("RunDAG", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-scheduler-id", State: "running", Name: "test-scheduler"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, true)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("error with container", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("RunDAG", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-scheduler-id", State: "running", Name: "test-scheduler"}}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, true)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success without container", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("RunDAG", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, true)
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("error without container", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("RunDAG", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, true)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("build error without container", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", "", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, true)
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("PS error without container", func() {
		noCache := false
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.RunDAG("", "", "", "", noCache, true)
		s.ErrorIs(err, errMockDocker)

		composeMock.AssertExpectations(s.T())
	})
}

var errExecMock = errors.New("docker is not running")

func (s *Suite) TestCreateDockerProject() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.LocalPlatform)
	err := afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	s.NoError(err)
	config.InitConfig(fs)
	s.Run("case when project doesnot have docker-compose.override.yml", func() {
		prj, err := createDockerProject("test", "", "", "test-image:latest", "", "", labels)
		s.NoError(err)
		postgresService := &types.ServiceConfig{}
		serviceFound := false
		for i := range prj.Services {
			service := prj.Services[i]
			if service.Name == "webserver" {
				postgresService = &service
				serviceFound = true
				break
			}
		}
		s.True(serviceFound)
		s.Equal("test-image:latest", postgresService.Image)
	})

	s.Run("case when project has docker-compose.override.yml", func() {
		composeOverrideFilename = "./testfiles/docker-compose.override.yml"
		prj, err := createDockerProject("test", "", "", "test-image:latest", "", "", labels)
		s.NoError(err)
		postgresService := &types.ServiceConfig{}
		serviceFound := false
		for i := range prj.Services {
			service := prj.Services[i]
			if service.Name == "postgres" {
				postgresService = &service
				serviceFound = true
				break
			}
		}
		s.True(serviceFound)
		s.Equal("postgres", postgresService.Name)
		s.Equal("5433", postgresService.Ports[len(prj.Services["postgres"].Ports)-1].Published)
	})
}

func (s *Suite) TestUpgradeDockerfile() {
	s.Run("update Dockerfile with new tag", func() {
		// Create a temporary old Dockerfile
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/astro-runtime:12.0.0\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		s.NoError(err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "13.0.0"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		s.NoError(err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		s.NoError(err)
		s.Contains(string(newContent), "FROM quay.io/astronomer/astro-runtime:13.0.0")
	})

	s.Run("update Dockerfile with new major version", func() {
		// Create a temporary old Dockerfile
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/astro-runtime:12.0.0\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		s.NoError(err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "3.0-1"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		s.NoError(err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		s.NoError(err)
		s.Contains(string(newContent), "FROM astrocrpublic.azurecr.io/runtime:3.0-1")
	})

	s.Run("update Dockerfile with new image", func() {
		// Create a temporary old Dockerfile
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/astro-runtime:old-tag\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		s.NoError(err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newImage := "new-image"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, "", newImage)
		defer os.Remove(newDockerfilePath)

		// Check for errors
		s.NoError(err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		s.NoError(err)
		s.Contains(string(newContent), "FROM new-image")
	})

	s.Run("update Dockerfile for ap-airflow with runtime version", func() {
		// Create a temporary old Dockerfile with a line matching the pattern
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/ap-airflow:old-tag"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		s.NoError(err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "5.0.0"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		s.NoError(err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		s.NoError(err)
		s.Contains(string(newContent), "FROM quay.io/astronomer/astro-runtime:5.0.0\n")
	})

	s.Run("update Dockerfile for ap-airflow with non-runtime version", func() {
		// Create a temporary old Dockerfile with a line matching the pattern
		oldDockerfilePath := "test_old_Dockerfile"
		oldContent := "FROM quay.io/astronomer/ap-airflow:old-tag\n"
		err := os.WriteFile(oldDockerfilePath, []byte(oldContent), 0o644)
		s.NoError(err)
		defer os.Remove(oldDockerfilePath)

		// Define test data
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "new-tag"

		// Call the function
		err = upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		s.NoError(err)

		// Read the new Dockerfile and check its content
		newContent, err := os.ReadFile(newDockerfilePath)
		s.NoError(err)
		s.Contains(string(newContent), "FROM quay.io/astronomer/ap-airflow:new-tag")
	})

	s.Run("error reading old Dockerfile", func() {
		// Define test data with an invalid path
		oldDockerfilePath := "non_existent_Dockerfile"
		newDockerfilePath := "test_new_Dockerfile"
		newTag := "new-tag"

		// Call the function
		err := upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, "")
		defer os.Remove(newDockerfilePath)

		// Check for errors
		s.Error(err)
		s.Contains(err.Error(), "no such file or directory")
	})
}
