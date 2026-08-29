package cmd

import (
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/config"
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

// astro run is v1 machinery: it builds a Docker worker against a v1 project.
// Inside a v2 project the v1 project check failed with advice to run
// astro dev init, a command v2 removed, so there was no way forward at all.
func (s *CmdSuite) TestRunInV2ProjectNamesItsReplacement() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := s.T().TempDir()
	manifest := "[project]\nname = \"demo\"\n\n[tool.astro]\n"
	s.NoError(os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600))

	previous := config.WorkingPath
	config.WorkingPath = dir
	defer func() { config.WorkingPath = previous }()

	_, err := executeCommand("run", "my_dag")
	s.Error(err)
	s.Contains(err.Error(), "astro local run airflow dags test my_dag")
	s.NotContains(err.Error(), "astro dev init")
}
