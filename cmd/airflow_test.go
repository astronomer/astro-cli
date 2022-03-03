package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/pkg/errors"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errSomeContainerIssue = errors.New("some container issue")

func executeCommandC(client *houston.Client, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	rootCmd := NewRootCmd(client, buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(args)
	_, err = rootCmd.ExecuteC()
	client.HTTPClient.HTTPClient.CloseIdleConnections()
	return buf.String(), err
}

func executeCommand(args ...string) (output string, err error) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	output, err = executeCommandC(client, args...)
	return output, err
}

func TestDevRootCommand(t *testing.T) {
	testUtils.InitTestConfig()
	output, err := executeCommand("dev")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
}

func TestNewAirflowInitCmd(t *testing.T) {
	client := houston.NewHoustonClient(httputil.NewHTTPClient())
	cmd := newAirflowInitCmd(client, os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStartCmd(t *testing.T) {
	cmd := newAirflowStartCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowKillCmd(t *testing.T) {
	cmd := newAirflowKillCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowLogsCmd(t *testing.T) {
	cmd := newAirflowLogsCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStopCmd(t *testing.T) {
	cmd := newAirflowStopCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowPSCmd(t *testing.T) {
	cmd := newAirflowPSCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowRunCmd(t *testing.T) {
	cmd := newAirflowRunCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowUpgradeCheckCmd(t *testing.T) {
	cmd := newAirflowUpgradeCheckCmd(os.Stdout)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestAirflowKillSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Kill").Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowKillCmd(os.Stdout)
	err := airflowKill(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowKillFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Kill").Return(errSomeContainerIssue)
		return mockContainer, nil
	}

	cmd := newAirflowKillCmd(os.Stdout)
	err := airflowKill(cmd, []string{})
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockContainer.AssertExpectations(t)
}

func TestAirflowLogsDockerSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.WebserverContainerName.SetHomeString("webserver_tmp")
	config.CFG.SchedulerContainerName.SetHomeString("scheduler_tmp")
	config.CFG.TriggererContainerName.SetHomeString("triggerer_tmp")
	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Logs", mock.Anything, "scheduler", "webserver", "triggerer").Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowLogsCmd(os.Stdout)
	cmd.Flags().Set("scheduler", "true")
	cmd.Flags().Set("webserver", "true")
	cmd.Flags().Set("triggerer", "true")
	err := airflowLogs(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowLogsPodmanSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)
	config.CFG.WebserverContainerName.SetHomeString("webserver_tmp")
	config.CFG.SchedulerContainerName.SetHomeString("scheduler_tmp")
	config.CFG.TriggererContainerName.SetHomeString("triggerer_tmp")
	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Logs", mock.Anything, "scheduler_tmp", "webserver_tmp", "triggerer_tmp").Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowLogsCmd(os.Stdout)
	cmd.Flags().Set("scheduler", "true")
	cmd.Flags().Set("webserver", "true")
	cmd.Flags().Set("triggerer", "true")
	err := airflowLogs(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowLogsFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Logs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockContainer, nil
	}

	cmd := newAirflowLogsCmd(os.Stdout)
	err := airflowLogs(cmd, []string{})
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockContainer.AssertExpectations(t)
}

func TestAirflowStopSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Stop").Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowStopCmd(os.Stdout)
	err := airflowStop(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowStopFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Stop").Return(errSomeContainerIssue)
		return mockContainer, nil
	}

	cmd := newAirflowStopCmd(os.Stdout)
	err := airflowStop(cmd, []string{})
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockContainer.AssertExpectations(t)
}

func TestAirflowPSSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("PS").Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowPSCmd(os.Stdout)
	err := airflowPS(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowPSFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("PS").Return(errSomeContainerIssue)
		return mockContainer, nil
	}

	cmd := newAirflowPSCmd(os.Stdout)
	err := airflowPS(cmd, []string{})
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockContainer.AssertExpectations(t)
}

func TestAirflowRunSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Run", mock.Anything, mock.Anything).Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowRunCmd(os.Stdout)
	err := airflowRun(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowRunFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Run", mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockContainer, nil
	}

	cmd := newAirflowRunCmd(os.Stdout)
	err := airflowRun(cmd, []string{})
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockContainer.AssertExpectations(t)
}

func TestAirflowUpgradeCheckSuccess(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("podman")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Run", mock.Anything, mock.Anything).Return(nil)
		return mockContainer, nil
	}

	cmd := newAirflowUpgradeCheckCmd(os.Stdout)
	err := airflowUpgradeCheck(cmd, []string{})
	assert.NoError(t, err)
	mockContainer.AssertExpectations(t)
}

func TestAirflowUpgradeCheckFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0o777)
	config.InitConfig(fs)

	mockContainer := new(mocks.ContainerHandler)
	containerHandlerInit = func(airflowHome, envFile string) (airflow.ContainerHandler, error) {
		mockContainer.On("Run", mock.Anything, mock.Anything).Return(errSomeContainerIssue)
		return mockContainer, nil
	}

	cmd := newAirflowUpgradeCheckCmd(os.Stdout)
	err := airflowUpgradeCheck(cmd, []string{})
	assert.Error(t, err, errSomeContainerIssue.Error())
	mockContainer.AssertExpectations(t)
}
