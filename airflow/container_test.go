package airflow

import (
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/afero"

	"github.com/stretchr/testify/assert"
)

func TestGetFmtEnvFileSuccess(t *testing.T) {
	resp, err := getFmtEnvFile("testfiles/env.test.valid", "podman")
	validLines := []string{"- name: test1", "value: \"1\"", "- name: test2", "value: astro", "- name: test3", "value: astro123"}
	for idx := range validLines {
		assert.Contains(t, resp, validLines[idx])
	}
	assert.NoError(t, err)

	resp, err = getFmtEnvFile("testfiles/env.test.valid", "docker")
	assert.NoError(t, err)
	assert.Equal(t, "env_file: testfiles/env.test.valid", resp)
}

func TestGetWebserverServiceNameDocker(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.WebserverContainerName.SetHomeString("webserver_tmp")
	webserverName := GetWebserverServiceName()
	assert.Equal(t, webserverName, webserverServiceName)
}

func TestGetWebserverServiceNamePodman(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.WebserverContainerName.SetHomeString("webserver_tmp")
	webserverName := GetWebserverServiceName()
	assert.Equal(t, webserverName, "webserver_tmp")
}

func TestGetSchedulerServiceNameDocker(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.SchedulerContainerName.SetHomeString("scheduler_tmp")
	schedulerName := GetSchedulerServiceName()
	assert.Equal(t, schedulerName, schedulerServiceName)
}

func TestGetSchedulerServiceNamePodman(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.SchedulerContainerName.SetHomeString("scheduler_tmp")
	schedulerName := GetSchedulerServiceName()
	assert.Equal(t, schedulerName, "scheduler_tmp")
}

func TestGetTriggererServiceNameDocker(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.TriggererContainerName.SetHomeString("triggerer_tmp")
	triggererName := GetTriggererServiceName()
	assert.Equal(t, triggererName, triggererServiceName)
}

func TestGetTriggererServiceNamePodman(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.TriggererContainerName.SetHomeString("triggerer_tmp")
	triggererName := GetTriggererServiceName()
	assert.Equal(t, triggererName, "triggerer_tmp")
}

func TestCheckTriggererEnabled(t *testing.T) {
	t.Run("astro-runtime supported version", func(t *testing.T) {
		labels := map[string]string{
			runtimeVersionLabelName: triggererAllowedRuntimeVersion,
		}
		triggererEnabled, err := CheckTriggererEnabled(labels)
		assert.NoError(t, err)
		assert.True(t, triggererEnabled)
	})

	t.Run("astro-runtime unsupported version", func(t *testing.T) {
		labels := map[string]string{
			runtimeVersionLabelName: "3.0.0",
		}
		triggererEnabled, err := CheckTriggererEnabled(labels)
		assert.NoError(t, err)
		assert.False(t, triggererEnabled)
	})

	t.Run("astronomer-certified supported version", func(t *testing.T) {
		labels := map[string]string{
			airflowVersionLabelName: "2.4.0",
		}
		triggererEnabled, err := CheckTriggererEnabled(labels)
		assert.NoError(t, err)
		assert.True(t, triggererEnabled)
	})

	t.Run("astronomer-certified unsupported version", func(t *testing.T) {
		labels := map[string]string{
			airflowVersionLabelName: "2.1.0",
		}

		triggererEnabled, err := CheckTriggererEnabled(labels)
		assert.NoError(t, err)
		assert.False(t, triggererEnabled)
	})
}
