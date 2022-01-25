package airflow

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/messages"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var ErrSomeDockerIssue = errors.New("some docker error")

func TestRepositoryName(t *testing.T) {
	assert.Equal(t, repositoryName("test-repo"), "test-repo/airflow")
}

func TestImageName(t *testing.T) {
	assert.Equal(t, imageName("test-repo", "0.15.0"), "test-repo/airflow:0.15.0")
}

func TestCheckServiceStateTrue(t *testing.T) {
	assert.True(t, checkServiceState("RUNNING test", "RUNNING"))
}

func TestCheckServiceStateFalse(t *testing.T) {
	assert.False(t, checkServiceState("RUNNING test", "FAILED"))
}

func TestGenerateConfig(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0777)
	config.InitConfig(fs)
	cfg, err := generateConfig("test-project-name", "airflow_home", ".env", map[string]string{airflowVersionLabelName: triggererAllowedAirflowVersion}, DockerEngine)
	assert.NoError(t, err)
	expectedCfg := `version: '3.1'

networks:
  airflow:
    driver: bridge

volumes:
  postgres_data:
    driver: local
    name: test-project-name_postgres_data
  airflow_logs:
    driver: local
    name: test-project-name_airflow_logs

services:
  postgres:
    image: postgres:12.2
    container_name: test-project-name-postgres
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
    container_name: scheduler
    command: >
      bash -c "(airflow upgradedb || airflow db upgrade) && airflow scheduler"
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
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:ro
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_logs:/usr/local/airflow/logs
    

  webserver:
    image: test-project-name/airflow:latest
    container_name: webserver
    command: >
      bash -c 'if [[ -z "$$AIRFLOW__API__AUTH_BACKEND" ]] && [[ $$(pip show -f apache-airflow | grep basic_auth.py) ]];
        then export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.basic_auth ;
        else export AIRFLOW__API__AUTH_BACKEND=airflow.api.auth.backend.default ; fi &&
        { airflow create_user "$$@" || airflow users create "$$@" ; } &&
        { airflow sync_perm || airflow sync-perm ;} &&
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
    ports:
      - 8080:8080
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_logs:/usr/local/airflow/logs
    healthcheck:
      test: curl --fail http://webserver:8080/health || exit 1
      interval: 2s
      retries: 50
      start_period: 10s
      timeout: 10s
    

  triggerer:
    image: test-project-name/airflow:latest
    container_name: triggerer
    command: >
      bash -c "(airflow upgradedb || airflow db upgrade) && airflow triggerer"
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
    environment:
      AIRFLOW__CORE__EXECUTOR: LocalExecutor
      AIRFLOW__CORE__SQL_ALCHEMY_CONN: postgresql://postgres:postgres@postgres:5432
      AIRFLOW__CORE__LOAD_EXAMPLES: "False"
      AIRFLOW__CORE__FERNET_KEY: "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="
      AIRFLOW__WEBSERVER__RBAC: "True"
    volumes:
      - airflow_home/dags:/usr/local/airflow/dags:z
      - airflow_home/plugins:/usr/local/airflow/plugins:z
      - airflow_home/include:/usr/local/airflow/include:z
      - airflow_logs:/usr/local/airflow/logs
    
`
	assert.Equal(t, cfg, expectedCfg)
}

func TestExecVersion(t *testing.T) {
	err := dockerExec(nil, nil, "version")
	if err != nil {
		t.Error(err)
	}
}

func TestExecPipe(t *testing.T) {
	var buf bytes.Buffer
	data := ""
	resp := &types.HijackedResponse{Reader: bufio.NewReader(strings.NewReader(data))}
	err := execPipe(*resp, &buf, &buf, &buf)
	fmt.Println(buf.String())
	if err != nil {
		t.Error(err)
	}
}

func TestExecPipeNils(t *testing.T) {
	data := ""
	resp := &types.HijackedResponse{Reader: bufio.NewReader(strings.NewReader(data))}
	err := execPipe(*resp, nil, nil, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestDockerStartFailure(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{{ID: "testID", Name: "test", State: "running"}}, nil)
	composeMock.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := docker.Start("./testfiles/Dockerfile.Airflow1.ok")
	assert.Contains(t, err.Error(), "cannot start, project already running")
}

func TestDockerKillSuccess(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Down", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := docker.Kill()
	assert.NoError(t, err)
}

func TestDockerKillFailure(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Down", mock.Anything, mock.Anything, mock.Anything).Return(ErrSomeDockerIssue)
	err := docker.Kill()
	assert.Contains(t, err.Error(), messages.ErrContainerStop)
}

func TestDockerLogsSuccess(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{{ID: "test", Name: "test"}}, nil)
	composeMock.On("Logs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := docker.Logs(false, []string{"test"}...)
	assert.NoError(t, err)
}

func TestDockerLogsFailure(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{}, nil).Once()
	err := docker.Logs(false, []string{"test"}...)
	assert.Contains(t, err.Error(), "cannot view logs, project not running")
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{{ID: "test", Name: "test"}}, nil)
	composeMock.On("Logs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(ErrSomeDockerIssue)
	err = docker.Logs(false, []string{"test"}...)
	assert.Contains(t, err.Error(), ErrSomeDockerIssue.Error())
}

func TestDockerStopSuccess(t *testing.T) {
	composeMock, docker, imageMock := getComposeMocks()
	imageMock.On("GetImageLabels").Return(map[string]string{}, nil)
	composeMock.On("Stop", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := docker.Stop()
	assert.NoError(t, err)
}

func TestDockerStopFailure(t *testing.T) {
	composeMock, docker, imageMock := getComposeMocks()
	imageMock.On("GetImageLabels").Return(map[string]string{}, nil)
	composeMock.On("Stop", mock.Anything, mock.Anything, mock.Anything).Return(ErrSomeDockerIssue)
	err := docker.Stop()
	assert.Contains(t, err.Error(), messages.ErrContainerPause)
}

func TestDockerPSSuccess(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{{Name: "test", State: "running", Publishers: api.PortPublishers{api.PortPublisher{PublishedPort: 8888}}}}, nil)
	err := docker.PS()
	assert.NoError(t, err)
}

func TestDockerPSFailure(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{}, ErrSomeDockerIssue)
	err := docker.PS()
	assert.Contains(t, err.Error(), messages.ErrContainerStatusCheck)
}

func TestDockerGetContainerIDSuccess(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{{ID: "testID", Name: "test", State: "running", Publishers: api.PortPublishers{api.PortPublisher{PublishedPort: 8888}}}}, nil)
	id, err := docker.GetContainerID("test")
	assert.NoError(t, err)
	assert.Contains(t, id, "testID")
}

func TestDockerGetContainerIDFailure(t *testing.T) {
	composeMock, docker, _ := getComposeMocks()
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{}, ErrSomeDockerIssue).Once()
	id, err := docker.GetContainerID("testFail")
	assert.Contains(t, err.Error(), messages.ErrContainerStatusCheck)
	assert.Contains(t, id, "")
	composeMock.On("Ps", mock.Anything, mock.Anything, mock.Anything).Return([]api.ContainerSummary{{ID: "testID", Name: "test", State: "running", Publishers: api.PortPublishers{api.PortPublisher{PublishedPort: 8888}}}}, nil)
	id, err = docker.GetContainerID("testFail")
	assert.NoError(t, err)
	assert.Contains(t, id, "")
}

func getComposeMocks() (*mocks.Service, *DockerCompose, *mocks.ImageHandler) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	projectDir, _ := os.Getwd()

	componseMock := new(mocks.Service)
	imageMock := new(mocks.ImageHandler)
	docker := &DockerCompose{
		airflowHome:    projectDir,
		projectName:    "test",
		envFile:        "",
		composeService: componseMock,
		imageHandler:   imageMock,
	}
	return componseMock, docker, imageMock
}
