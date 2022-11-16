package cmd

import (
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestRunCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("run", "--help")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro run", output)
}

func TestRun(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		cmd := newRunCommand()
		args := []string{"test-dag"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("RunDAG", "test-dag", "airflow_settings.yaml", false, true).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := run(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newRunCommand()
		args := []string{"test-dag"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("RunDAG", "test-dag", "airflow_settings.yaml", false, true).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := run(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newRunCommand()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := run(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}
