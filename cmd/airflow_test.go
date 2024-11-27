package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	coreMocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var errMock = errors.New("mock error")

type AirflowSuite struct {
	suite.Suite
	tempDir string
}

func TestAirflow(t *testing.T) {
	suite.Run(t, new(AirflowSuite))
}

func (s *AirflowSuite) SetupSubTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir, err := os.MkdirTemp("", "test_temp_dir_*")
	if err != nil {
		s.T().Fatalf("failed to create temp dir: %v", err)
	}
	s.tempDir = dir
	config.WorkingPath = s.tempDir
}

func (s *AirflowSuite) TearDownTest() {
	// Clean up init files after test
	s.cleanUpInitFiles()
}

func (s *AirflowSuite) TearDownSubTest() {
	// Clean up init files after test
	s.cleanUpInitFiles()
}

var (
	_ suite.SetupSubTest    = (*AirflowSuite)(nil)
	_ suite.TearDownSubTest = (*AirflowSuite)(nil)
)

func (s *AirflowSuite) TestDevRootCommand() {
	output, err := executeCommand("dev")
	s.NoError(err)
	s.Contains(output, "astro dev", output)
}

func (s *AirflowSuite) TestDevInitCommand() {
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

func (s *AirflowSuite) TestDevInitCommandSoftware() {
	s.Run("unknown software version", func() {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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

func (s *AirflowSuite) TestNewAirflowDevRootCmd() {
	cmd := newDevRootCmd(nil, nil)
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) TestNewAirflowInitCmd() {
	cmd := newAirflowInitCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) TestNewAirflowRunCmd() {
	cmd := newAirflowRunCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) TestNewAirflowPSCmd() {
	cmd := newAirflowPSCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) TestNewAirflowLogsCmd() {
	cmd := newAirflowLogsCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) TestNewAirflowKillCmd() {
	cmd := newAirflowKillCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) TestNewAirflowUpgradeCheckCmd() {
	cmd := newAirflowUpgradeCheckCmd()
	s.Nil(cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func (s *AirflowSuite) Test_airflowInitNonEmptyDir() {
	s.Run("test airflow init with non empty dir", func() {
		cmd := newAirflowInitCmd()
		var args []string

		defer testUtil.MockUserInput(s.T(), "y")()
		err := airflowInit(cmd, args)
		s.NoError(err)

		b, _ := os.ReadFile(filepath.Join(s.tempDir, "Dockerfile"))
		dockerfileContents := string(b)
		s.True(strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))
	})
}

func (s *AirflowSuite) Test_airflowInitNoDefaultImageTag() {
	s.Run("test airflow init with non empty dir", func() {
		cmd := newAirflowInitCmd()
		var args []string

		defer testUtil.MockUserInput(s.T(), "y")()

		err := airflowInit(cmd, args)
		s.NoError(err)
		// assert contents of Dockerfile
		b, _ := os.ReadFile(filepath.Join(s.tempDir, "Dockerfile"))
		dockerfileContents := string(b)
		s.True(strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))
	})
}

func (s *AirflowSuite) cleanUpInitFiles() {
	s.T().Helper()
	if s.tempDir != "" {
		err := os.RemoveAll(s.tempDir)
		if err != nil {
			s.T().Fatalf("failed to remove temp dir: %v", err)
		}
	}
}

func (s *AirflowSuite) mockUserInput(i string) (r, stdin *os.File) {
	input := []byte(i)
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	_, err = w.Write(input)
	s.NoError(err)
	w.Close()
	stdin = os.Stdin
	return r, stdin
}

func (s *AirflowSuite) TestAirflowInit() {
	TemplateList = func() ([]string, error) { return []string{"A", "B", "C", "D"}, nil }
	s.Run("initialize template based project via select-template flag", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("from-template").Value.Set("select-template")
		var args []string

		input := []byte("1")
		r, w, inputErr := os.Pipe()
		s.Require().NoError(inputErr)
		_, writeErr := w.Write(input)
		s.NoError(writeErr)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.NoError(err)
	})

	s.Run("invalid template name", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("from-template").Value.Set("E")
		var args []string

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.EqualError(err, "E is not a valid template name. Available templates are: [A B C D]")
	})

	s.Run("successfully initialize template based project ", func() {
		airflow.ExtractTemplate = func(templateDir, destDir string) error {
			err := os.MkdirAll(destDir, os.ModePerm)
			s.NoError(err)
			mockFile := filepath.Join(destDir, "requirements.txt")
			file, err := os.Create(mockFile)
			s.NoError(err)
			defer file.Close()
			_, err = file.WriteString("test requirements file.")
			s.NoError(err)
			return nil
		}
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("from-template").Value.Set("A")
		var args []string

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.NoError(err)

		b, _ := os.ReadFile(filepath.Join(s.tempDir, "requirements.txt"))
		fileContents := string(b)
		s.True(strings.Contains(fileContents, "test requirements file"))
	})

	s.Run("success", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		var args []string

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.NoError(err)

		b, _ := os.ReadFile(filepath.Join(s.tempDir, "Dockerfile"))
		dockerfileContents := string(b)
		s.True(strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))
	})

	s.Run("invalid args", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{"invalid-arg"}

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.ErrorIs(err, errProjectNameSpaces)
	})

	s.Run("invalid project name", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test@project-name")
		args := []string{}

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.ErrorIs(err, errConfigProjectName)
	})

	s.Run("both runtime & AC version passed", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("airflow-version").Value.Set("2.2.5")
		cmd.Flag("runtime-version").Value.Set("4.2.4")
		args := []string{}

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		s.ErrorIs(err, errInvalidBothAirflowAndRuntimeVersions)
	})

	s.Run("runtime version passed alongside AC flag", func() {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("use-astronomer-certified").Value.Set("true")
		cmd.Flag("runtime-version").Value.Set("4.2.4")
		args := []string{}

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "You provided a runtime version with the --use-astronomer-certified flag. Thus, this command will ignore the --runtime-version value you provided.")
	})

	s.Run("use AC flag", func() {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("use-astronomer-certified").Value.Set("true")
		args := []string{}

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "Pulling Airflow development files from Astronomer Certified Airflow Version")
	})

	s.Run("cancel non empty dir warning", func() {
		config.WorkingPath = ""
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{}

		r, stdin := s.mockUserInput("n")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "Canceling project initialization...")
	})

	s.Run("reinitialize the same project", func() {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{}

		r, stdin := s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err := airflowInit(cmd, args)
		s.NoError(err)

		r, stdin = s.mockUserInput("y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = airflowInit(cmd, args)

		w.Close()
		out, _ := io.ReadAll(r)

		s.NoError(err)
		s.Contains(string(out), "Reinitialized existing Astro project in")
	})
}

func (s *AirflowSuite) TestAirflowStart() {
	s.Run("success", func() {
		cmd := newAirflowStartCmd(nil)
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, nil)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("success with deployment id flag set but environment objects disabled", func() {
		cmd := newAirflowStartCmd(nil)
		deploymentID = "test-deployment-id"
		cmd.Flag("deployment-id").Value.Set(deploymentID)
		args := []string{"test-env-file"}
		config.CFG.DisableEnvObjects.SetHomeString("true")
		defer config.CFG.DisableEnvObjects.SetHomeString("false")

		mockCoreClient := new(coreMocks.ClientWithResponsesInterface)

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, mockCoreClient)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with deployment id flag set", func() {
		cmd := newAirflowStartCmd(nil)
		deploymentID = "test-deployment-id"
		cmd.Flag("deployment-id").Value.Set(deploymentID)
		args := []string{"test-env-file"}

		envObj := astrocore.EnvironmentObject{
			ObjectKey: "test-object-key",
			Connection: &astrocore.EnvironmentObjectConnection{
				Type: "test-conn-type",
			},
		}
		mockCoreClient := new(coreMocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(params *astrocore.ListEnvironmentObjectsParams) bool {
			return *params.DeploymentId == deploymentID
		})).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.EnvironmentObjectsPaginated{
				EnvironmentObjects: []astrocore.EnvironmentObject{envObj},
			},
		}, nil).Once()

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, mockCoreClient)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with workspace id flag set", func() {
		cmd := newAirflowStartCmd(nil)
		workspaceID = "test-workspace-id"
		cmd.Flag("workspace-id").Value.Set(workspaceID)
		args := []string{"test-env-file"}

		envObj := astrocore.EnvironmentObject{
			ObjectKey: "test-object-key",
			Connection: &astrocore.EnvironmentObjectConnection{
				Type: "test-conn-type",
			},
		}
		mockCoreClient := new(coreMocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(params *astrocore.ListEnvironmentObjectsParams) bool {
			return *params.WorkspaceId == workspaceID
		})).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.EnvironmentObjectsPaginated{
				EnvironmentObjects: []astrocore.EnvironmentObject{envObj},
			},
		}, nil).Once()

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, mockCoreClient)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowStartCmd(nil)
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, nil)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowStartCmd(nil)
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowStart(cmd, args, nil)
		s.ErrorIs(err, errMock)
	})
}

func (s *AirflowSuite) TestAirflowUpgradeTest() {
	s.Run("success", func() {
		cmd := newAirflowUpgradeTestCmd(nil)

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("UpgradeTest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false, false, nil).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeTest(cmd, nil)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		cmd := newAirflowUpgradeTestCmd(nil)

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("UpgradeTest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false, false, nil).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeTest(cmd, nil)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowUpgradeTestCmd(nil)

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowUpgradeTest(cmd, nil)
		s.ErrorIs(err, errMock)
	})

	s.Run("Both airflow and runtime version used", func() {
		cmd := newAirflowUpgradeTestCmd(nil)

		airflowVersion = "something"
		runtimeVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		s.ErrorIs(err, errInvalidBothAirflowAndRuntimeVersionsUpgrade)
	})

	s.Run("Both runtime version and custom image used", func() {
		cmd := newAirflowUpgradeTestCmd(nil)

		customImageName = "something"
		runtimeVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		s.ErrorIs(err, errInvalidBothCustomImageandVersion)
	})

	s.Run("Both airflow version and custom image used", func() {
		cmd := newAirflowUpgradeTestCmd(nil)

		customImageName = "something"
		airflowVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		s.ErrorIs(err, errInvalidBothCustomImageandVersion)
	})
}

func (s *AirflowSuite) TestAirflowRun() {
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

func (s *AirflowSuite) TestAirflowPS() {
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

func (s *AirflowSuite) TestAirflowLogs() {
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

func (s *AirflowSuite) TestAirflowKill() {
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

func (s *AirflowSuite) TestAirflowStop() {
	s.Run("success", func() {
		cmd := newAirflowStopCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", false).Return(nil).Once()
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
			mockContainerHandler.On("Stop", false).Return(errMock).Once()
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

func (s *AirflowSuite) TestAirflowRestart() {
	s.Run("success", func() {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, nil)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("success with deployment id flag set", func() {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		deploymentID := "test-deployment-id"
		cmd.Flag("deployment-id").Value.Set(deploymentID)
		args := []string{"test-env-file"}

		envObj := astrocore.EnvironmentObject{
			ObjectKey:  "test-object-key",
			Connection: &astrocore.EnvironmentObjectConnection{Type: "test-conn-type"},
		}
		mockCoreClient := new(coreMocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(params *astrocore.ListEnvironmentObjectsParams) bool {
			return *params.DeploymentId == deploymentID
		})).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.EnvironmentObjectsPaginated{
				EnvironmentObjects: []astrocore.EnvironmentObject{envObj},
			},
		}, nil).Once()

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, mockCoreClient)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("success with workspace id flag set", func() {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		workspaceID := "test-workspace-id"
		cmd.Flag("workspace-id").Value.Set(workspaceID)
		args := []string{"test-env-file"}

		envObj := astrocore.EnvironmentObject{
			ObjectKey:  "test-object-key",
			Connection: &astrocore.EnvironmentObjectConnection{Type: "test-conn-type"},
		}
		mockCoreClient := new(coreMocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(params *astrocore.ListEnvironmentObjectsParams) bool {
			return *params.WorkspaceId == workspaceID
		})).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.EnvironmentObjectsPaginated{
				EnvironmentObjects: []astrocore.EnvironmentObject{envObj},
			},
		}, nil).Once()

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, mockCoreClient)
		s.NoError(err)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("stop failure", func() {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, nil)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("start failure", func() {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, defaultWaitTime, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, nil)
		s.ErrorIs(err, errMock)
		mockContainerHandler.AssertExpectations(s.T())
	})

	s.Run("containerHandlerInit failure", func() {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowRestart(cmd, args, nil)
		s.ErrorIs(err, errMock)
	})
}

func (s *AirflowSuite) TestAirflowPytest() {
	s.Run("success", func() {
		cmd := newAirflowPytestCmd()
		cmd.Flag("args").Value.Set("args-string")
		cmd.Flag("build-secrets").Value.Set("id=mysecret,src=secrets.txt")
		cmd.Flag("image-name").Value.Set("custom-image")
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", "test-pytest-file", "custom-image", "", "args-string", "id=mysecret,src=secrets.txt").Return("0", nil).Once()
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
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("exit code 1", errMock).Once()
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
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("0", nil).Once()
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
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("0", errMock).Once()
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
		pytestDir = ""

		projectNameUnique = func() (string, error) {
			return "", errMock
		}
		defer func() { projectNameUnique = airflow.ProjectNameUnique }()

		err := airflowPytest(cmd, args)
		s.ErrorIs(err, errMock)
	})

	s.Run("too many args failure failure", func() {
		cmd := newAirflowParseCmd()
		args := []string{"arg1", "arg2"}

		err := airflowPytest(cmd, args)
		s.ErrorIs(err, errPytestArgs)
	})
}

func (s *AirflowSuite) TestAirflowParse() {
	s.Run("success", func() {
		cmd := newAirflowParseCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Parse", "", "", "").Return(nil).Once()
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
			mockContainerHandler.On("Parse", "", "", "").Return(errMock).Once()
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

func (s *AirflowSuite) TestAirflowUpgradeCheck() {
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

func (s *AirflowSuite) TestAirflowBash() {
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

func (s *AirflowSuite) TestAirflowObjectImport() {
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

func (s *AirflowSuite) TestAirflowObjectExport() {
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

	s.Run("error compose-file used without compose", func() {
		cmd := newObjectExportCmd()
		cmd.SetArgs([]string{"dev", "object", "export", "--compose-file", "file.yaml"})

		// Set the "compose-file" flag explicitly to mark it as changed
		cmd.Flags().Set("compose-file", "file.yaml")

		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		s.ErrorIs(err, errNoCompose)
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

func (s *AirflowSuite) TestPrepareDefaultAirflowImageTag() {
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
