package airflow

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	docker_types "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	errMockDocker   = errors.New("mock docker compose error")
	errMockSettings = errors.New("mock Settings error")
)

var (
	airflowVersionLabel        = "2.2.5"
	runtimeVersionLabel        = "12.0.0"
	labels                     = map[string]string{airflowVersionLabelName: airflowVersionLabel, runtimeVersionLabelName: runtimeVersionLabel}
	deploymentID               = "test-deployment-id"
	mockCoreDeploymentResponse = []astrov1.Deployment{
		{
			Id:     deploymentID,
			Status: "HEALTHY",
		},
	}
	mockListDeploymentsResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	mockGetDeploymentsResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
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

func (s *Suite) TestSanitizeImageName() {
	// Already-valid names must pass through byte-for-byte. Changing them would
	// rename cached image tags and orphan users' local images.
	unchanged := []string{
		"simple",
		"project_name",
		"my-project",
		"my-project_abc123", // typical ProjectNameUnique output
		"a__b",              // docker allows a double-underscore separator
		"foo.bar",
		"tmp155bkx9_684ec5",
	}
	for _, in := range unchanged {
		s.Equal(in, sanitizeImageName(in), "valid name %q must not change", in)
	}

	// Malformed names must become valid docker image names. These are the
	// shapes that broke `astro dev` builds from random temp-dir names.
	fixes := map[string]string{
		"a-_b":                  "a-b",                  // the reported "-_" double separator
		"__x":                   "x",                    // leading separators trimmed
		"-lead":                 "lead",                 // leading separator trimmed
		"trail-":                "trail",                // trailing separator trimmed
		"!!!":                   "project",              // all punctuation -> non-empty fallback
		"tmp-155-bkx-9-_684ec5": "tmp-155-bkx-9-684ec5", // the exact failing shape
		"UPPER":                 "upper",                // docker image names are lowercase
	}
	for in, want := range fixes {
		got := sanitizeImageName(in)
		s.Equal(want, got, "input %q", in)
		s.True(validImageName.MatchString(got), "sanitized %q -> %q must be a valid image name", in, got)
	}

	// Whatever the input, the result is always a valid, non-empty image name.
	for _, in := range []string{"", "-", "___", ".-.", "a-_-_-b", "9", "-_-"} {
		got := sanitizeImageName(in)
		s.NotEmpty(got)
		s.True(validImageName.MatchString(got), "sanitized %q -> %q must be a valid image name", in, got)
	}
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
  AIRFLOW__API__SECRET_KEY: "test-project-name"
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
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.NoError(err)

		err = mockDockerCompose.Start(&airflowTypes.StartOptions{ImageName: "custom-image", NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with shorter default startup time", func() {
		defaultTimeOut := 1 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			s.Equal(defaultTimeOut, timeout)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: defaultTimeOut})
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with longer default startup time", func() {
		expectedTimeout := 10 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			s.Equal(expectedTimeout, timeout)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: expectedTimeout})
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with user provided startup time", func() {
		userProvidedTimeOut := 8 * time.Minute
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Times(2)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			s.Equal(userProvidedTimeOut, timeout)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: userProvidedTimeOut})
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("success with invalid airflow version label", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: "2.3.4.dev+astro1", runtimeVersionLabelName: runtimeVersionLabel}, nil).Times(4)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.NoError(err)

		err = mockDockerCompose.Start(&airflowTypes.StartOptions{ImageName: "custom-image", NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.NoError(err)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("image build failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("list label failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		composeMock := new(mocks.DockerComposeAPI)

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose up failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(errMockDocker).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
		s.ErrorIs(err, errMockDocker)

		imageHandler.AssertExpectations(s.T())
		composeMock.AssertExpectations(s.T())
	})

	s.Run("webserver health check failure", func() {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(labels, nil).Twice()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		checkWebserverHealth = func(url string, timeout time.Duration, component string) error {
			return errMockDocker
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start(&airflowTypes.StartOptions{NoCache: noCache, NoBrowser: true, WaitTime: waitTime})
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

	// The poll interval is a second in production, and the cases below wait out
	// one or two of them. These are package variables, so put them back after.
	origTimeout, origTicker := stopPostgresWaitTimeout, stopPostgresWaitTicker
	stopPostgresWaitTicker = 10 * time.Millisecond
	s.T().Cleanup(func() {
		stopPostgresWaitTimeout, stopPostgresWaitTicker = origTimeout, origTicker
	})

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

		// This case needs one thing to happen before another: at least one tick
		// has to fire (so Ps is called and the "still running" line is logged)
		// and then the timeout has to win. The Ps expectation carries no
		// .Once(), so any number of ticks from one upward satisfies it.
		//
		// It used to set this to 11ms against a 10ms ticker — racing two
		// wall-clock timers 1ms apart. That is far inside the scheduling jitter
		// of a loaded CI runner, and when the timeout won the tick, zero ticks
		// fired: Ps was never called, and the "still running" assertion below
		// failed along with the Ps mock expectation. It went red on a docs-only
		// PR .
		//
		// Only that ONE assertion is jitter-sensitive. The "timed out" one
		// cannot fail here: Ps is stubbed to answer "running" forever, so the
		// exited-state branch in Stop is unreachable and the loop can only ever
		// leave through the timeout arm, which logs that line unconditionally.
		// Worth stating, because a future reader debugging this would otherwise
		// spend time on an assertion that is structurally incapable of failing.
		//
		// 200ms leaves room for ~20 ticks, so losing the first several to jitter
		// costs nothing, and the timeout is absolute — time.After still fires at
		// 200ms however many ticks got through.
		//
		// There is a third option this does not take, and it is not "inject a
		// clock". Setting the ticker to something that can never fire (an hour)
		// and the timeout to 1ms would leave only ONE ready channel, so select
		// has no choice to make and the test is exact rather than generous — at
		// the cost of the property this case uniquely holds, that a tick does
		// not RESET the timeout. Move `timeout := time.After(...)` inside Stop's
		// loop and this version hangs; the split version passes. That property
		// is worth the 200ms, and the ceiling below is what turns the hang into
		// a readable failure.
		//
		// Restored here rather than beside the ticker at method scope: the two
		// waiting cases above should keep the production 10s, which gives them a
		// 1000x margin instead of this case's 20x, and leaving 200ms set would
		// hand it to every subtest that follows.
		origCaseTimeout := stopPostgresWaitTimeout
		stopPostgresWaitTimeout = 200 * time.Millisecond
		defer func() { stopPostgresWaitTimeout = origCaseTimeout }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		logger.SetLevel(5) // debug level
		var out bytes.Buffer
		logger.SetOutput(&out)

		err := mockDockerCompose.Stop(true)
		s.NoError(err)

		s.Contains(out.String(), "postgres container is still in running state, waiting for it to be in exited state")
		s.Contains(out.String(), "timed out waiting for postgres container to be in exited state")
		// An upper bound as well as a lower one. The old 11ms supplied a ceiling
		// by accident; widening the timeout removed it, and without one a change
		// that made the effective wait an order of magnitude longer than
		// configured would show up only as a slower suite — in the one case
		// whose whole subject is the relation between those two bounds.
		// ~20 ticks plus the Stop call is the expected shape.
		s.LessOrEqual(len(composeMock.Calls), 40,
			"Stop polled far more than the timeout allows; is the timeout being reset?")
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

		data, err := mockDockerCompose.PS()
		s.NoError(err)
		s.Equal("docker", data.Mode)
		s.Len(data.Containers, 1)
		s.Equal("test-webserver", data.Containers[0].Name)
		s.Equal("running", data.Containers[0].State)
		s.Contains(data.Containers[0].Ports, "8080")
		composeMock.AssertExpectations(s.T())
	})

	s.Run("compose ps failure", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		_, err := mockDockerCompose.PS()
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
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(container.ExecCreateResponse{ID: "test-exec-id"}, nil).Once()
		mockCLIClient.On("ContainerExecAttach", context.Background(), "test-exec-id", container.ExecStartOptions{Detach: false}).Return(docker_types.HijackedResponse{Reader: mockResp}, nil).Once()
		mockCLIClient.On("ContainerExecInspect", context.Background(), "test-exec-id").Return(container.ExecInspect{ExitCode: 0}, nil).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		s.NoError(err)
		composeMock.AssertExpectations(s.T())
		mockCLIClient.AssertExpectations(s.T())
	})

	s.Run("non-zero exit code", func() {
		testCmd := []string{"test", "command"}
		str := bytes.NewReader([]byte(`0`))
		mockResp := bufio.NewReader(str)

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(container.ExecCreateResponse{ID: "test-exec-id"}, nil).Once()
		mockCLIClient.On("ContainerExecAttach", context.Background(), "test-exec-id", container.ExecStartOptions{Detach: false}).Return(docker_types.HijackedResponse{Reader: mockResp}, nil).Once()
		mockCLIClient.On("ContainerExecInspect", context.Background(), "test-exec-id").Return(container.ExecInspect{ExitCode: 1}, nil).Once()
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.cliClient = mockCLIClient

		err := mockDockerCompose.Run(testCmd, "test-user")
		s.EqualError(err, "command exited with code 1")
		composeMock.AssertExpectations(s.T())
		mockCLIClient.AssertExpectations(s.T())
	})

	s.Run("exec id is empty", func() {
		testCmd := []string{"test", "command"}
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: "test-webserver", State: "running"}}, nil).Once()
		mockCLIClient := new(mocks.DockerCLIClient)
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(container.ExecCreateResponse{}, nil).Once()
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
		mockCLIClient.On("ContainerExecCreate", context.Background(), "test-webserver-id", container.ExecOptions{User: "test-user", AttachStdout: true, Cmd: testCmd}).Return(container.ExecCreateResponse{}, errMockDocker).Once()
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
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", nil)

		s.NoError(err)
		s.Equal("", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("success custom image", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("0", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "custom-image-name", "", "", nil)

		s.NoError(err)
		s.Equal("", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("unexpected exit code", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("1", nil).Once()

		mockResponse := "1"
		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", nil)
		s.Contains(err.Error(), "something went wrong while Pytesting your DAGs")
		s.Equal(mockResponse, resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("internal error exit code 10 reported as failure", func() {
		// exit code 10 substring-contains "0"; the old check reported it as a pass
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("10", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", nil)
		s.Contains(err.Error(), "something went wrong while Pytesting your DAGs")
		s.Equal("10", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("interrupt exit code 130 reported as failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("130", nil).Once()

		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "", "", "", nil)
		s.Contains(err.Error(), "something went wrong while Pytesting your DAGs")
		s.Equal("130", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("image build failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "", "", "", nil)
		s.ErrorIs(err, errMockDocker)
		imageHandler.AssertExpectations(s.T())
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

		err := mockDockerCompose.Parse("", "test", nil)
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

		err := mockDockerCompose.Parse("", "test", nil)
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

		err := mockDockerCompose.Parse("", "test", nil)
		s.Contains(err.Error(), "something went wrong while parsing your DAGs")
		composeMock.AssertExpectations(s.T())
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("internal error exit code 10", func() {
		// exit code 10 substring-contains "1"; the old check reported it as a clean DAG error
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("10", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", nil)
		s.Contains(err.Error(), "something went wrong while parsing your DAGs")
		composeMock.AssertExpectations(s.T())
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("interrupt exit code 130", func() {
		// exit code 130 (Ctrl-C) substring-contains "1"; the old check reported it as a clean DAG error
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: false}).Return("130", nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test", nil)
		s.Contains(err.Error(), "something went wrong while parsing your DAGs")
		composeMock.AssertExpectations(s.T())
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("file does not exists", func() {
		DefaultTestPath = "test_invalid_file.py"

		r, w, _ := os.Pipe()
		os.Stdout = w

		err := mockDockerCompose.Parse("", "test", nil)
		s.NoError(err)

		w.Close()
		out, _ := io.ReadAll(r)

		s.Contains(string(out), "does not exist. Please run `astro dev init` to create it")
	})

	s.Run("invalid file name", func() {
		DefaultTestPath = "\x0004"

		err := mockDockerCompose.Parse("", "test", nil)
		s.Contains(err.Error(), "invalid argument")
	})
}

func (s *Suite) TestDockerComposeBuild() {
	s.Run("success", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: "", NoCache: false}).Return(nil).Once()

		mockDockerCompose := DockerCompose{
			imageHandler: imageHandler,
		}

		err := mockDockerCompose.Build("", nil, false)
		s.NoError(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("success with no-cache", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: "", NoCache: true}).Return(nil).Once()

		mockDockerCompose := DockerCompose{
			imageHandler: imageHandler,
		}

		err := mockDockerCompose.Build("", nil, true)
		s.NoError(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("success with custom image", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", "my-custom-image:latest").Return(nil).Once()

		mockDockerCompose := DockerCompose{
			imageHandler: imageHandler,
		}

		err := mockDockerCompose.Build("my-custom-image:latest", nil, false)
		s.NoError(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("build failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", mock.Anything, mock.Anything, airflowTypes.ImageBuildConfig{Path: "", NoCache: false}).Return(errMock).Once()

		mockDockerCompose := DockerCompose{
			imageHandler: imageHandler,
		}

		err := mockDockerCompose.Build("", nil, false)
		s.ErrorIs(err, errMock)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("tag local image failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", "my-custom-image:latest").Return(errMock).Once()

		mockDockerCompose := DockerCompose{
			imageHandler: imageHandler,
		}

		err := mockDockerCompose.Build("my-custom-image:latest", nil, false)
		s.ErrorIs(err, errMock)
		imageHandler.AssertExpectations(s.T())
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
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Maybe()

		initSettings = func(airflowURL, authHeader, settingsFile string, envConns map[string]astrov1.EnvironmentObjectConnection, connections, variables, pools bool) error {
			return nil
		}

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		s.NoError(err)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("import failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Maybe()

		initSettings = func(airflowURL, authHeader, settingsFile string, envConns map[string]astrov1.EnvironmentObjectConnection, connections, variables, pools bool) error {
			return errMockSettings
		}

		mockDockerCompose.imageHandler = imageHandler
		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", false, false, false)
		s.ErrorIs(err, errMockSettings)
		imageHandler.AssertExpectations(s.T())
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

	s.Run("import list labels error", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMock).Once()

		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true)
		s.Contains(err.Error(), errMock.Error())
		imageHandler.AssertExpectations(s.T())
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

	s.Run("compose ps failure export", func() {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		err := mockDockerCompose.ExportSettings("./testfiles/airflow_settings.yaml", ".env", true, true, true, false)
		s.ErrorIs(err, errMockDocker)
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
		err := mockDockerCompose.ImportSettings("./testfiles/airflow_settings_invalid.yaml", ".env", true, true, true)
		s.Contains(err.Error(), "file specified does not exist")
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
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
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
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(nil).Once()
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
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, NoCache: noCache}).Return(errMockDocker).Once()

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

func (s *Suite) TestPrintStatusURL() {
	fs := afero.NewMemMapFs()
	configYaml := testUtil.NewTestConfig(testUtil.LocalPlatform)
	err := afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	s.NoError(err)
	config.InitConfig(fs)

	originalOpenURL := openURL
	openURL = func(url string) error { return nil }
	defer func() { openURL = originalOpenURL }()

	capture := func(fn func()) string {
		origStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		fn()
		w.Close()
		os.Stdout = origStdout
		out, _ := io.ReadAll(r)
		return string(out)
	}

	s.Run("uses config port when no overrides (Airflow 2)", func() {
		out := capture(func() {
			err := printStatus("./testfiles/non_existent_settings.yaml", nil, airflowMajorVersion2, true, nil)
			s.NoError(err)
		})
		s.Contains(out, "http://localhost:"+config.CFG.WebserverPort.GetString())
	})

	s.Run("uses webserver port override (Airflow 2)", func() {
		ovr := &PortOverrides{
			PostgresPort:  "55432",
			WebserverPort: "58080",
			APIServerPort: "58080",
		}
		out := capture(func() {
			err := printStatus("./testfiles/non_existent_settings.yaml", nil, airflowMajorVersion2, true, ovr)
			s.NoError(err)
		})
		s.Contains(out, "http://localhost:58080")
		s.Contains(out, "postgresql://localhost:55432/postgres")
		// Make sure the proxy hostname did NOT leak into the printed URL.
		s.NotContains(out, ".localhost:")
	})

	s.Run("uses api-server port override (Airflow 3)", func() {
		ovr := &PortOverrides{
			PostgresPort:  "55432",
			WebserverPort: "58080",
			APIServerPort: "58080",
		}
		out := capture(func() {
			err := printStatus("./testfiles/non_existent_settings.yaml", nil, airflowMajorVersion3, true, ovr)
			s.NoError(err)
		})
		s.Contains(out, "http://localhost:58080")
		s.NotContains(out, ".localhost:")
	})
}

func (s *Suite) TestInitSettings() {
	testCases := []struct {
		name                     string
		settingsFile             string
		envConns                 map[string]astrov1.EnvironmentObjectConnection
		airflowMajorVersion      uint64
		project                  *types.Project
		containerSummary         []api.ContainerSummary
		expectInitSettingsCalled bool
	}{
		{
			name:                "initSettings called when only settings file exists",
			settingsFile:        "./testfiles/airflow_settings.yaml",
			envConns:            map[string]astrov1.EnvironmentObjectConnection{},
			airflowMajorVersion: airflowMajorVersion2,
			project: &types.Project{
				Name: "test-project",
				Services: map[string]types.ServiceConfig{
					"webserver": {Name: "test-project-webserver"},
				},
			},
			containerSummary: []api.ContainerSummary{
				{ID: "test-webserver-id", Name: "test-project-webserver", State: "running"},
			},
			expectInitSettingsCalled: true,
		},
		{
			name:         "initSettings called when only env connections exist",
			settingsFile: "./testfiles/non_existent_settings.yaml",
			envConns: map[string]astrov1.EnvironmentObjectConnection{
				"test-conn": {},
			},
			airflowMajorVersion: airflowMajorVersion2,
			project: &types.Project{
				Name: "test-project",
				Services: map[string]types.ServiceConfig{
					"webserver": {Name: "test-project-webserver"},
				},
			},
			containerSummary: []api.ContainerSummary{
				{ID: "test-webserver-id", Name: "test-project-webserver", State: "running"},
			},
			expectInitSettingsCalled: true,
		},
		{
			name:         "initSettings called when both settings file and env connections exist",
			settingsFile: "./testfiles/airflow_settings.yaml",
			envConns: map[string]astrov1.EnvironmentObjectConnection{
				"test-conn": {},
			},
			airflowMajorVersion: airflowMajorVersion2,
			project: &types.Project{
				Name: "test-project",
				Services: map[string]types.ServiceConfig{
					"webserver": {Name: "test-project-webserver"},
				},
			},
			containerSummary: []api.ContainerSummary{
				{ID: "test-webserver-id", Name: "test-project-webserver", State: "running"},
			},
			expectInitSettingsCalled: true,
		},
		{
			name:                "initSettings NOT called when neither settings file nor env connections exist",
			settingsFile:        "./testfiles/non_existent_settings.yaml",
			envConns:            map[string]astrov1.EnvironmentObjectConnection{},
			airflowMajorVersion: airflowMajorVersion2,
			project: &types.Project{
				Name: "test-project",
				Services: map[string]types.ServiceConfig{
					"webserver": {Name: "test-project-webserver"},
				},
			},
			containerSummary: []api.ContainerSummary{
				{ID: "test-webserver-id", Name: "test-project-webserver", State: "running"},
			},
			expectInitSettingsCalled: false,
		},
		{
			name:                "initSettings called for API server container in Airflow 3",
			settingsFile:        "./testfiles/airflow_settings.yaml",
			envConns:            map[string]astrov1.EnvironmentObjectConnection{},
			airflowMajorVersion: airflowMajorVersion3,
			project: &types.Project{
				Name: "test-project",
				Services: map[string]types.ServiceConfig{
					"api-server": {Name: "test-project-api-server"},
				},
			},
			containerSummary: []api.ContainerSummary{
				{ID: "test-api-server-id", Name: "test-project-api-server", State: "running"},
			},
			expectInitSettingsCalled: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			initSettingsCalled := false

			// Mock initSettings to track if it's called
			originalInitSettings := initSettings
			initSettings = func(airflowURL, authHeader, settingsFile string, envConns map[string]astrov1.EnvironmentObjectConnection, connections, variables, pools bool) error {
				initSettingsCalled = true
				return nil
			}
			defer func() { initSettings = originalInitSettings }()

			// Mock openURL to avoid opening browser
			originalOpenURL := openURL
			openURL = func(url string) error {
				return nil
			}
			defer func() { openURL = originalOpenURL }()

			err := printStatus(tc.settingsFile, tc.envConns, tc.airflowMajorVersion, true, nil)
			s.NoError(err)
			s.Equal(tc.expectInitSettingsCalled, initSettingsCalled)
		})
	}
}

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

	s.Run("bare host-env passthrough in an override resolves from the process env", func() {
		s.T().Setenv("ASTRO_TEST_PASSTHROUGH", "from-host")
		prev := composeOverrideFilename
		composeOverrideFilename = "./testfiles/docker-compose.passthrough.override.yml"
		defer func() { composeOverrideFilename = prev }()
		prj, err := createDockerProject("test", "", "", "test-image:latest", "", "", labels)
		s.NoError(err)
		got := prj.Services["webserver"].Environment["ASTRO_TEST_PASSTHROUGH"]
		s.Require().NotNil(got)
		s.Equal("from-host", *got)
	})
}
