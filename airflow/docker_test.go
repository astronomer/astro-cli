package airflow

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command/inspect"
	"github.com/docker/compose/v2/pkg/api"
	docker_types "github.com/docker/docker/api/types"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errMockDocker = errors.New("mock docker compose error")

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
	cfg, err := generateConfig("test-project-name", "airflow_home", ".env", "", "", map[string]string{}, false)
	assert.NoError(t, err)
	expectedCfg := `version: '3.1'

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
    image: postgres:12.6
    restart: unless-stopped
    networks:
      - airflow
    labels:
      io.astronomer.docker: "true"
      io.astronomer.docker.cli: "true"
    ports:
      - 5432:5432
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
    environment:
      AIRFLOW__CORE__EXECUTOR: LocalExecutor
      AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
      ASTRONOMER_ENVIRONMENT: local
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:ro
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
    environment:
      AIRFLOW__CORE__EXECUTOR: LocalExecutor
      AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
      AIRFLOW__WEBSERVER__RBAC: "True"
      ASTRONOMER_ENVIRONMENT: local
    ports:
      - 8080:8080
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
	assert.Equal(t, cfg, expectedCfg)
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
	_, err := DockerComposeInit("./testfiles", "", "Dockerfile", "", false)
	assert.NoError(t, err)
}

func TestDockerComposeStart(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	mockDockerCompose := DockerCompose{projectName: "test"}
	t.Run("success", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Twice()
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Twice()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Twice()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", noCache)
		assert.NoError(t, err)

		err = mockDockerCompose.Start("custom-image", "", noCache)
		assert.NoError(t, err)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("project already running", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Start("", "", false)
		assert.Contains(t, err.Error(), "cannot start, project already running")

		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Start("", "", false)
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
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", noCache)
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
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", noCache)
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
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64) error {
			return nil
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", noCache)
		assert.ErrorIs(t, err, errMockDocker)

		imageHandler.AssertExpectations(t)
		composeMock.AssertExpectations(t)
	})

	t.Run("webserver health check failure", func(t *testing.T) {
		noCache := false
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: noCache}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()
		composeMock.On("Up", mock.Anything, mock.Anything, api.UpOptions{Create: api.CreateOptions{}}).Return(nil).Once()

		orgCheckWebserverHealthFunc := checkWebserverHealth
		checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64) error {
			return errMockDocker
		}
		defer func() { checkWebserverHealth = orgCheckWebserverHealthFunc }()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Start("", "", noCache)
		assert.ErrorIs(t, err, errMockDocker)

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
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Twice()
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Twice()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Twice()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-1", Name: "test-1"}}, nil).Twice()

		mockResponse := "0"
		inspectContainer = func(out io.Writer, references []string, tmplStr string, getRef inspect.GetRefFunc) error {
			io.WriteString(out, mockResponse)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "test", "")
		assert.NoError(t, err)
		assert.Equal(t, "", resp)

		resp, err = mockDockerCompose.Pytest("custom-image", "test", "")
		assert.NoError(t, err)
		assert.Equal(t, "", resp)

		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("unexpected exit code", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-1", Name: "test-1"}}, nil).Once()

		mockResponse := "1"
		inspectContainer = func(out io.Writer, references []string, tmplStr string, getRef inspect.GetRefFunc) error {
			io.WriteString(out, mockResponse)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		resp, err := mockDockerCompose.Pytest("", "test", "")
		assert.Contains(t, err.Error(), "something went wrong while Pytesting your DAGs")
		assert.Equal(t, mockResponse, resp)
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("inspect container failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-1", Name: "test-1"}}, nil).Once()

		mockResponse := "1"
		inspectContainer = func(out io.Writer, references []string, tmplStr string, getRef inspect.GetRefFunc) error {
			io.WriteString(out, mockResponse)
			return errMockDocker
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "test", "")
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("no test containers", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(errMockDocker).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "test", "")
		assert.Contains(t, err.Error(), "error finding the testing container")
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "test", "")
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("compose up failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "test", "")
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("list labels failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(nil).Once()
		imageHandler.On("ListLabels").Return(map[string]string{}, errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "test", "")
		assert.ErrorIs(t, err, errMockDocker)
		imageHandler.AssertExpectations(t)
	})

	t.Run("image build failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", airflowTypes.ImageBuildConfig{Path: mockDockerCompose.airflowHome, Output: true, NoCache: false}).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("", "test", "")
		assert.ErrorIs(t, err, errMockDocker)
		imageHandler.AssertExpectations(t)
	})

	t.Run("image Tag local image failure", func(t *testing.T) {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil).Return(errMockDocker).Once()

		mockDockerCompose.imageHandler = imageHandler

		_, err := mockDockerCompose.Pytest("custom-image", "test", "")
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
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-1", Name: "test-1"}}, nil).Once()

		mockResponse := "0"
		inspectContainer = func(out io.Writer, references []string, tmplStr string, getRef inspect.GetRefFunc) error {
			io.WriteString(out, mockResponse)
			return nil
		}

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
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-1", Name: "test-1"}}, nil).Once()

		mockResponse := "exit code 1"
		inspectContainer = func(out io.Writer, references []string, tmplStr string, getRef inspect.GetRefFunc) error {
			io.WriteString(out, mockResponse)
			return nil
		}

		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Parse("", "test")
		assert.Contains(t, err.Error(), "errors detected in your local DAGs are listed above")
		composeMock.AssertExpectations(t)
		imageHandler.AssertExpectations(t)
	})

	t.Run("exit code 2", func(t *testing.T) {
		DefaultTestPath = "test_dag_integrity_file.py"

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Up", mock.Anything, mock.Anything, mock.AnythingOfType("api.UpOptions")).Return(nil).Once()
		composeMock.On("Down", mock.Anything, mock.Anything, api.DownOptions{Volumes: true, RemoveOrphans: true}).Return(nil).Once()
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-1", Name: "test-1"}}, nil).Once()

		mockResponse := "exit code 2"
		inspectContainer = func(out io.Writer, references []string, tmplStr string, getRef inspect.GetRefFunc) error {
			io.WriteString(out, mockResponse)
			return nil
		}

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
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()

		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()

		initSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return nil
		}
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, false, false)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("import failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		initSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return errMock
		}
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, false, false)
		assert.ErrorIs(t, err, errMock)
		composeMock.AssertExpectations(t)
	})

	t.Run("export success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		exportSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return nil
		}
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, true, false)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("export failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		exportSettings = func(id, settingsFile string, version uint64, connections, variables, pools bool) error {
			return errMock
		}
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, true, false)
		assert.ErrorIs(t, err, errMock)
		composeMock.AssertExpectations(t)
	})

	t.Run("env export success", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		envExportSettings = func(id, settingsFile string, version uint64, connections, variables bool) error {
			return nil
		}
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, true, true)
		assert.NoError(t, err)
		composeMock.AssertExpectations(t)
	})

	t.Run("env export failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{airflowVersionLabelName: airflowVersionLabel}, nil).Once()
		envExportSettings = func(id, settingsFile string, version uint64, connections, variables bool) error {
			return errMock
		}
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, true, true)
		assert.ErrorIs(t, err, errMock)
		composeMock.AssertExpectations(t)
	})

	t.Run("list lables error", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", State: "running"}}, nil).Once()
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("ListLabels").Return(map[string]string{}, errMock).Once()
		
		mockDockerCompose.composeService = composeMock
		mockDockerCompose.imageHandler = imageHandler

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, false, false)
		assert.Contains(t, err.Error(), errMock.Error())
		composeMock.AssertExpectations(t)
	})

	t.Run("compose ps failure", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()

		mockDockerCompose.composeService = composeMock
		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, false, false)
		assert.ErrorIs(t, err, errMockDocker)
		composeMock.AssertExpectations(t)
	})

	t.Run("project not running", func(t *testing.T) {
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mockDockerCompose.projectName, api.PsOptions{All: true}).Return([]api.ContainerSummary{}, nil).Once()

		mockDockerCompose.composeService = composeMock

		err := mockDockerCompose.Settings("airflow_settings.yaml", ".env", true, true, true, false, false)
		assert.Contains(t, err.Error(), "project not running, run docker dev start to start project")
		composeMock.AssertExpectations(t)
	})
}

func TestCheckWebserverHealth(t *testing.T) {
	testUtils.InitTestConfig(testUtils.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		airflowSettingsFile = "docker_test.go" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{{ID: "test-webserver-id", Name: fmt.Sprintf("test-%s", WebserverDockerContainerName), State: "running"}}, nil).Once()
		mockEventsCall := composeMock.On("Events", context.Background(), "test", mock.Anything)
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

		err := checkWebserverHealth("", &types.Project{Name: "test"}, composeMock, 2)
		assert.NoError(t, err)

		w.Close()
		out, _ := io.ReadAll(r)
		assert.Contains(t, string(out), "Project is running! All components are now available.")
	})

	t.Run("compose ps failure", func(t *testing.T) {
		airflowSettingsFile = "./testfiles/test_dag_inegrity_file.py" // any file which exists
		composeMock := new(mocks.DockerComposeAPI)
		composeMock.On("Ps", mock.Anything, mock.AnythingOfType("string"), api.PsOptions{All: true}).Return([]api.ContainerSummary{}, errMockDocker).Once()
		mockEventsCall := composeMock.On("Events", context.Background(), "test", mock.Anything)
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

		err := checkWebserverHealth("", &types.Project{Name: "test"}, composeMock, 2)
		assert.ErrorIs(t, err, errMockDocker)
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
