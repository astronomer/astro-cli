package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
)

var errMock = errors.New("mock error")

func (s *Suite) TestDevRootCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("dev")
	s.NoError(err)
	s.Contains(output, "astro dev", output)
}

func (s *Suite) TestDevInitCommand() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	output, err := executeCommand("dev", "init", "--help")
	s.NoError(err)
	s.Contains(output, "astro dev", output)
	s.NotContains(output, "--use-astronomer-certified")

	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err = executeCommand("dev", "init", "--help")
	s.NoError(err)
	s.Contains(output, "astro dev", output)
	s.Contains(output, "--use-astronomer-certified")
}

func (s *Suite) TestDevInitCommandSoftware() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("unknown software version", func() {
		houstonVersion = ""
		cmd := newAirflowInitCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"dev", "init", "--help"})
		testUtil.SetupOSArgsForGinkgo()
		_, err := cmd.ExecuteC()
		output := buf.String()
		s.NoError(err)
		s.Contains(output, "astro dev", output)
		s.Contains(output, "--use-astronomer-certified")
		s.Contains(output, "--runtime-version string")
	})

	s.Run("0.28.0 software version", func() {
		houstonVersion = "0.28.0"
		cmd := newAirflowInitCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--help"})
		testUtil.SetupOSArgsForGinkgo()
		_, err := cmd.ExecuteC()
		output := buf.String()
		s.NoError(err)
		s.Contains(output, "astro dev", output)
		s.NotContains(output, "--use-astronomer-certified")
		s.NotContains(output, "--runtime-version string")
	})

	s.Run("0.29.0 software version", func() {
		houstonVersion = "0.29.0"
		cmd := newAirflowInitCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"dev", "init", "--help"})
		testUtil.SetupOSArgsForGinkgo()
		_, err := cmd.ExecuteC()
		output := buf.String()
		s.NoError(err)
		s.Contains(output, "astro dev", output)
		s.Contains(output, "--use-astronomer-certified")
		s.Contains(output, "--runtime-version string")
	})
}

func (s *Suite) TestNewAirflowInitCmd() {
	cmd := newAirflowInitCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) TestNewAirflowStartCmd() {
	cmd := newAirflowStartCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) TestNewAirflowRunCmd() {
	cmd := newAirflowRunCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) TestNewAirflowPSCmd() {
	cmd := newAirflowPSCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) TestNewAirflowLogsCmd() {
	cmd := newAirflowLogsCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) TestNewAirflowKillCmd() {
	cmd := newAirflowKillCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) TestNewAirflowUpgradeCheckCmd() {
	cmd := newAirflowUpgradeCheckCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *Suite) Test_airflowInitNonEmptyDir() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cmd := newAirflowInitCmd()
	var args []string

	defer testUtil.MockUserInput(s.T(), "y")()
	err := airflowInit(cmd, args)
	s.NoError(err)

	b, _ := os.ReadFile("Dockerfile")
	dockerfileContents := string(b)
	s.True(strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))

	// Clean up init files after test
	s.cleanUpInitFiles()
}

func (s *Suite) Test_airflowInitNoDefaultImageTag() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cmd := newAirflowInitCmd()
	var args []string

	defer testUtil.MockUserInput(s.T(), "y")()

	err := airflowInit(cmd, args)
	s.NoError(err)
	// assert contents of Dockerfile
	b, _ := os.ReadFile("Dockerfile")
	dockerfileContents := string(b)
	s.True(strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))

	// Clean up init files after test
	s.cleanUpInitFiles()
}

func (s *Suite) cleanUpInitFiles() {
	files := []string{
		".dockerignore",
		".gitignore",
		".env",
		"Dockerfile",
		"airflow_settings.yaml",
		"packages.txt",
		"requirements.txt",
		"dags/example_dag_advanced.py",
		"dags/example_dag_basic.py",
		"plugins/example-plugin.py",
		"dags",
		"include",
		"plugins",
		"README.md",
		".astro/config.yaml",
		"./astro",
		"tests/dags/test_dag_integrity.py",
		"tests/dags",
		"tests",
	}
	for _, f := range files {
		e := os.Remove(f)
		if e != nil {
			s.T().Log(e)
		}
	}
}

func (s *Suite) TestAirflowInit() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("success", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		var args []string

		defer testUtil.MockUserInput(s.T(), "y")()

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()
		s.NoError(err)

		b, _ := os.ReadFile("Dockerfile")
		dockerfileContents := string(b)
		s.True(strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))
	})

	s.Run("invalid args", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{"invalid-arg"}

		defer testUtil.MockUserInput(s.T(), "y")()

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()
		s.ErrorIs(err, errProjectNameSpaces)
	})

	s.Run("invalid project name", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test@project-name")
		args := []string{}

		defer testUtil.MockUserInput(s.T(), "y")()

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()
		s.ErrorIs(err, errConfigProjectName)
	})

	s.Run("both runtime & AC version passed", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("airflow-version").Value.Set("2.2.5")
		cmd.Flag("runtime-version").Value.Set("4.2.4")
		args := []string{}

		defer testUtil.MockUserInput(s.T(), "y")()

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()
		s.ErrorIs(err, errInvalidBothAirflowAndRuntimeVersions)
	})

	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	s.Run("runtime version passed alongside AC flag", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("use-astronomer-certified").Value.Set("true")
		cmd.Flag("runtime-version").Value.Set("4.2.4")
		args := []string{}

		defer testUtil.MockUserInput(s.T(), "y")()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "You provided a runtime version with the --use-astronomer-certified flag. Thus, this command will ignore the --runtime-version value you provided.")
	})

	s.Run("use AC flag", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("use-astronomer-certified").Value.Set("true")
		args := []string{}

		defer testUtil.MockUserInput(s.T(), "y")()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "Pulling Airflow development files from Astronomer Certified Airflow Version")
	})

	s.Run("cancel non empty dir warning", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{}

		defer testUtil.MockUserInput(s.T(), "n")()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "Canceling project initialization...")
	})

	s.Run("reinitialize the same project", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{}

		defer testUtil.MockUserInput(s.T(), "y")()

		err := airflowInit(cmd, args)
		s.NoError(err)

		defer testUtil.MockUserInput(s.T(), "y")()

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = airflowInit(cmd, args)
		// Clean up init files after test
		defer s.cleanUpInitFiles()

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "Reinitialized existing Astro project in")
	})
}

func (s *Suite) TestAirflowStart() {
	s.Run("success", func() {
		cmd := newAirflowStartCmd()
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", false, false, 1*time.Minute).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowStartCmd()
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", false, false, 1*time.Minute).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowStartCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowStart(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowRun() {
	s.Run("success", func() {
		cmd := newAirflowRunCmd()
		args := []string{"test", "command"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", append([]string{"airflow"}, args...), "").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRun(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowRunCmd()
		args := []string{"test", "command"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", append([]string{"airflow"}, args...), "").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRun(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowRunCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowRun(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowPS() {
	s.Run("success", func() {
		cmd := newAirflowPSCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("PS").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowPS(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowPSCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("PS").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowPS(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowPSCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowPS(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowLogs() {
	s.Run("success", func() {
		cmd := newAirflowLogsCmd()
		cmd.Flag("webserver").Value.Set("true")
		cmd.Flag("scheduler").Value.Set("true")
		cmd.Flag("triggerer").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Logs", false, "webserver", "scheduler", "triggerer").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowLogs(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("without any component flag", func() {
		cmd := newAirflowLogsCmd()
		cmd.Flag("follow").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Logs", true, "webserver", "scheduler", "triggerer").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowLogs(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowLogsCmd()
		cmd.Flag("webserver").Value.Set("true")
		cmd.Flag("scheduler").Value.Set("true")
		cmd.Flag("triggerer").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Logs", false, "webserver", "scheduler", "triggerer").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowLogs(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowLogsCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowLogs(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowKill() {
	s.Run("success", func() {
		cmd := newAirflowKillCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Kill").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowKill(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowKillCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Kill").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowKill(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowKillCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowKill(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowStop() {
	s.Run("success", func() {
		cmd := newAirflowStopCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStop(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowStopCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowStop(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowStopCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowStop(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowRestart() {
	s.Run("success", func() {
		cmd := newAirflowRestartCmd()
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop").Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", true, true, 1*time.Minute).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("stop failure", func() {
		cmd := newAirflowRestartCmd()
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("start failure", func() {
		cmd := newAirflowRestartCmd()
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop").Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", true, true, 1*time.Minute).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowRestartCmd()
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowRestart(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowPytest() {
	s.Run("success", func() {
		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", []string{"test-pytest-file"}, "", "").Return("0", nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("exit code 1", func() {
		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", []string{"test-pytest-file"}, "", "").Return("exit code 1", errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		s.Contains(err.Error(), "pytests failed")
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("pytest file doesnot exists", func() {
		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = "/testfile-not-exists"

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", []string{"test-pytest-file"}, "", "").Return("0", nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		s.Contains(err.Error(), "directory does not exist, please run `astro dev init` to create it")
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", []string{"test-pytest-file"}, "", "").Return("0", errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowPytest(cmd, args)
		s.ErrorIs(err, errMock)
	})

	s.Run("projectNameUnique failure", func() {
		cmd := newAirflowParseCmd()
		args := []string{}

		projectNameUnique = func() (string, error) {
			return "", errMock
		}
		defer func() { projectNameUnique = airflow.ProjectNameUnique }()

		err := airflowPytest(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowParse() {
	s.Run("success", func() {
		cmd := newAirflowParseCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Parse", "", "").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowParse(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowParseCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Parse", "", "").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowParse(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowParseCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowParse(cmd, args)
		s.ErrorIs(err, errMock)
	})

	s.Run("projectNameUnique failure", func() {
		cmd := newAirflowParseCmd()
		args := []string{}

		projectNameUnique = func() (string, error) {
			return "", errMock
		}
		defer func() { projectNameUnique = airflow.ProjectNameUnique }()

		err := airflowParse(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowUpgradeCheck() {
	s.Run("success", func() {
		cmd := newAirflowUpgradeCheckCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", airflowUpgradeCheckCmd, "root").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeCheck(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowUpgradeCheckCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", airflowUpgradeCheckCmd, "root").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeCheck(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowUpgradeCheckCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowUpgradeCheck(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowBash() {
	s.Run("success", func() {
		cmd := newAirflowBashCmd()
		cmd.Flag("webserver").Value.Set("true")
		cmd.Flag("scheduler").Value.Set("true")
		cmd.Flag("triggerer").Value.Set("true")
		cmd.Flag("postgres").Value.Set("true")

		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Bash", "scheduler").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowBash(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("without any component flag", func() {
		cmd := newAirflowBashCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Bash", "scheduler").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowBash(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowBashCmd()
		cmd.Flag("webserver").Value.Set("true")
		cmd.Flag("scheduler").Value.Set("true")
		cmd.Flag("triggerer").Value.Set("true")
		cmd.Flag("postgres").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Bash", "scheduler").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowBash(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowBashCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowBash(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowObjectImport() {
	s.Run("success", func() {
		cmd := newObjectImportCmd()
		cmd.Flag("connections").Value.Set("true")
		cmd.Flag("pools").Value.Set("true")
		cmd.Flag("variables").Value.Set("true")

		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ImportSettings", "airflow_settings.yaml", ".env", connections, variables, pools).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsImport(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("without any object flag", func() {
		cmd := newObjectImportCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ImportSettings", "airflow_settings.yaml", ".env", connections, variables, pools).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsImport(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newObjectImportCmd()
		cmd.Flag("connections").Value.Set("true")
		cmd.Flag("pools").Value.Set("true")
		cmd.Flag("variables").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ImportSettings", "airflow_settings.yaml", ".env", connections, variables, pools).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsImport(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newObjectImportCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowSettingsImport(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestAirflowObjectExport() {
	s.Run("success", func() {
		cmd := newObjectExportCmd()
		cmd.Flag("connections").Value.Set("true")
		cmd.Flag("pools").Value.Set("true")
		cmd.Flag("variables").Value.Set("true")

		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ExportSettings", "airflow_settings.yaml", ".env", connections, variables, pools, envExport).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("success compose export", func() {
		cmd := newObjectExportCmd()
		cmd.Flag("compose").Value.Set("true")

		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ComposeExport", "airflow_settings.yaml", exportComposeFile).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("without any object flag", func() {
		cmd := newObjectExportCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ExportSettings", "airflow_settings.yaml", ".env", connections, variables, pools, envExport).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newObjectExportCmd()
		cmd.Flag("connections").Value.Set("true")
		cmd.Flag("pools").Value.Set("true")
		cmd.Flag("variables").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ExportSettings", "airflow_settings.yaml", ".env", connections, variables, pools, envExport).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newObjectExportCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowSettingsExport(cmd, args)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestPrepareDefaultAirflowImageTag() {
	getDefaultImageTag = func(httpClient *airflowversions.Client, airflowVersion string) (string, error) {
		return "", nil
	}
	s.Run("default airflow version", func() {
		useAstronomerCertified = true
		resp := prepareDefaultAirflowImageTag("", nil)
		s.Equal(airflowversions.DefaultAirflowVersion, resp)
	})

	s.Run("default runtime version", func() {
		useAstronomerCertified = false
		resp := prepareDefaultAirflowImageTag("", nil)
		s.Equal(airflowversions.DefaultRuntimeVersion, resp)
	})
}
