package airflow

import (
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/houston"
)

func TestDeploymentNameExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ReleaseName: "dev"},
		{ReleaseName: "dev1"},
	}
	exists := deploymentNameExists("dev", deployments)
	if !exists {
		t.Errorf("deploymentNameExists(dev) = %t; want true", exists)
	}
}

func TestDeploymentNameDoesntExists(t *testing.T) {
	deployments := []houston.Deployment{
		{ReleaseName: "dummy"},
	}
	exists := deploymentNameExists("dev", deployments)
	if exists {
		t.Errorf("deploymentNameExists(dev) = %t; want false", exists)
	}
}

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
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, []byte(configYaml), 0777)
	config.InitConfig(fs)
	cfg, err := generateConfig("test-project-name", "airflow_home", ".env")
	assert.NoError(t, err)
	expectedCfg := `version: '2'

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
    image: postgres:12.2
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
    `
	assert.Equal(t, cfg, expectedCfg)
}

func TestCreateProject(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, []byte(configYaml), 0777)
	config.InitConfig(fs)
	project, err := createProject("test-project-name", "airflow_home", ".env")
	assert.NoError(t, err)
	assert.NotNil(t, project)
}

func Test_validImageRepo(t *testing.T) {
	assert.True(t, validImageRepo("quay.io/astronomer/ap-airflow"))
	assert.True(t, validImageRepo("astronomerinc/ap-airflow"))
	assert.False(t, validImageRepo("personal-repo/ap-airflow"))
}

func Test_airflowVersionFromDockerFile(t *testing.T) {
	airflowHome := config.WorkingPath + "/testfiles"

	// Version 1
	expected := uint64(0x1)
	dockerfile := "Dockerfile.Airflow1.ok"
	version, err := airflowVersionFromDockerFile(airflowHome, dockerfile)

	assert.NoError(t, err)
	assert.Equal(t, expected, version)

	// Version 2
	expected = uint64(0x2)
	dockerfile = "Dockerfile.Airflow2.ok"
	version, err = airflowVersionFromDockerFile(airflowHome, dockerfile)

	assert.NoError(t, err)
	assert.Equal(t, expected, version)

	// Default to Airflow 1 when there is an invalid Tag
	expected = uint64(0x1)
	dockerfile = "Dockerfile.tag.invalid"
	version, err = airflowVersionFromDockerFile(airflowHome, dockerfile)

	assert.NoError(t, err)
	assert.Equal(t, expected, version)

	// Invalid Dockerfile
	dockerfile = "Dockerfile.not.real"
	version, err = airflowVersionFromDockerFile(airflowHome, dockerfile)

	assert.Error(t, err)

}
