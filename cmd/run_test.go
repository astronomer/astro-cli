package cmd

import (
	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestRunCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("run", "--help")
	s.NoError(err)
	s.Contains(output, "astro run", output)
}

func (s *CmdSuite) TestRun() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("success", func() {
		cmd := newRunCommand()
		args := []string{"test-dag"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("RunDAG", "test-dag", "airflow_settings.yaml", "", "", false, false).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := run(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newRunCommand()
		args := []string{"test-dag"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("RunDAG", "test-dag", "airflow_settings.yaml", "", "", false, false).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := run(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newRunCommand()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := run(cmd, args)
		s.ErrorIs(err, errMock)
	})
}
