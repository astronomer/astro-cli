package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	coreMocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errMock = errors.New("mock error")

func TestDevRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("dev")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
}

func TestDevInitCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("dev", "init", "--help")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
	assert.NotContains(t, output, "--use-astronomer-certified")

	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err = executeCommand("dev", "init", "--help")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro dev", output)
	assert.Contains(t, output, "--use-astronomer-certified")
}

func TestDevInitCommandSoftware(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("unknown software version", func(t *testing.T) {
		houstonVersion = ""
		cmd := newAirflowInitCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"dev", "init", "--help"})
		testUtil.SetupOSArgsForGinkgo()
		_, err := cmd.ExecuteC()
		output := buf.String()
		assert.NoError(t, err)
		assert.Contains(t, output, "astro dev", output)
		assert.Contains(t, output, "--use-astronomer-certified")
		assert.Contains(t, output, "--runtime-version string")
	})

	t.Run("0.28.0 software version", func(t *testing.T) {
		houstonVersion = "0.28.0"
		cmd := newAirflowInitCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--help"})
		testUtil.SetupOSArgsForGinkgo()
		_, err := cmd.ExecuteC()
		output := buf.String()
		assert.NoError(t, err)
		assert.Contains(t, output, "astro dev", output)
		assert.NotContains(t, output, "--use-astronomer-certified")
		assert.NotContains(t, output, "--runtime-version string")
	})

	t.Run("0.29.0 software version", func(t *testing.T) {
		houstonVersion = "0.29.0"
		cmd := newAirflowInitCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"dev", "init", "--help"})
		testUtil.SetupOSArgsForGinkgo()
		_, err := cmd.ExecuteC()
		output := buf.String()
		assert.NoError(t, err)
		assert.Contains(t, output, "astro dev", output)
		assert.Contains(t, output, "--use-astronomer-certified")
		assert.Contains(t, output, "--runtime-version string")
	})
}

func TestNewAirflowInitCmd(t *testing.T) {
	cmd := newAirflowInitCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowStartCmd(t *testing.T) {
	cmd := newAirflowStartCmd(nil)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowRunCmd(t *testing.T) {
	cmd := newAirflowRunCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowPSCmd(t *testing.T) {
	cmd := newAirflowPSCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowLogsCmd(t *testing.T) {
	cmd := newAirflowLogsCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowKillCmd(t *testing.T) {
	cmd := newAirflowKillCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestNewAirflowUpgradeCheckCmd(t *testing.T) {
	cmd := newAirflowUpgradeCheckCmd()
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func Test_airflowInitNonEmptyDir(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cmd := newAirflowInitCmd()
	var args []string

	defer testUtil.MockUserInput(t, "y")()
	err := airflowInit(cmd, args)
	assert.Nil(t, err)

	b, _ := os.ReadFile("Dockerfile")
	dockerfileContents := string(b)
	assert.True(t, strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))

	// Clean up init files after test
	cleanUpInitFiles(t)
}

func Test_airflowInitNoDefaultImageTag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cmd := newAirflowInitCmd()
	var args []string

	defer testUtil.MockUserInput(t, "y")()

	err := airflowInit(cmd, args)
	assert.Nil(t, err)
	// assert contents of Dockerfile
	b, _ := os.ReadFile("Dockerfile")
	dockerfileContents := string(b)
	assert.True(t, strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))

	// Clean up init files after test
	cleanUpInitFiles(t)
}

func cleanUpInitFiles(t *testing.T) {
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
		".astro/test_dag_integrity.py",
		"./astro",
		"tests/dags/test_dag_example.py",
		"tests/dags",
		"tests",
	}
	for _, f := range files {
		e := os.Remove(f)
		if e != nil {
			t.Log(e)
		}
	}
}

func mockUserInput(t *testing.T, i string) (r, stdin *os.File) {
	input := []byte(i)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin = os.Stdin
	return r, stdin
}

func TestAirflowInit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		var args []string

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()
		assert.Nil(t, err)

		b, _ := os.ReadFile("Dockerfile")
		dockerfileContents := string(b)
		assert.True(t, strings.Contains(dockerfileContents, "FROM quay.io/astronomer/astro-runtime:"))
	})

	t.Run("invalid args", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{"invalid-arg"}

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()
		assert.ErrorIs(t, err, errProjectNameSpaces)
	})

	t.Run("invalid project name", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test@project-name")
		args := []string{}

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()
		assert.ErrorIs(t, err, errConfigProjectName)
	})

	t.Run("both runtime & AC version passed", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("airflow-version").Value.Set("2.2.5")
		cmd.Flag("runtime-version").Value.Set("4.2.4")
		args := []string{}

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()
		assert.ErrorIs(t, err, errInvalidBothAirflowAndRuntimeVersions)
	})

	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	t.Run("runtime version passed alongside AC flag", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("use-astronomer-certified").Value.Set("true")
		cmd.Flag("runtime-version").Value.Set("4.2.4")
		args := []string{}

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()

		w.Close()
		out, _ := io.ReadAll(r)

		assert.NoError(t, err)
		assert.Contains(t, string(out), "You provided a runtime version with the --use-astronomer-certified flag. Thus, this command will ignore the --runtime-version value you provided.")
	})

	t.Run("use AC flag", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		cmd.Flag("use-astronomer-certified").Value.Set("true")
		args := []string{}

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()

		w.Close()
		out, _ := io.ReadAll(r)

		assert.NoError(t, err)
		assert.Contains(t, string(out), "Pulling Airflow development files from Astronomer Certified Airflow Version")
	})

	t.Run("cancel non empty dir warning", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{}

		r, stdin := mockUserInput(t, "n")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()

		w.Close()
		out, _ := io.ReadAll(r)

		assert.NoError(t, err)
		assert.Contains(t, string(out), "Canceling project initialization...")
	})

	t.Run("reinitialize the same project", func(t *testing.T) {
		cmd := newAirflowInitCmd()
		cmd.Flag("name").Value.Set("test-project-name")
		args := []string{}

		r, stdin := mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err := airflowInit(cmd, args)
		assert.NoError(t, err)

		r, stdin = mockUserInput(t, "y")

		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		orgStdout := os.Stdout
		defer func() { os.Stdout = orgStdout }()
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = airflowInit(cmd, args)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()

		w.Close()
		out, _ := io.ReadAll(r)

		assert.NoError(t, err)
		assert.Contains(t, string(out), "Reinitialized existing Astro project in")
	})
}

func TestAirflowStart(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowStartCmd(nil)
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, nil)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("success with deployment id flag set but environment objects disabled", func(t *testing.T) {
		cmd := newAirflowStartCmd(nil)
		deploymentID = "test-deployment-id"
		cmd.Flag("deployment-id").Value.Set(deploymentID)
		args := []string{"test-env-file"}
		config.CFG.DisableEnvObjects.SetHomeString("true")
		defer config.CFG.DisableEnvObjects.SetHomeString("false")

		mockCoreClient := new(coreMocks.ClientWithResponsesInterface)

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, mockCoreClient)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with deployment id flag set", func(t *testing.T) {
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
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, mockCoreClient)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with workspace id flag set", func(t *testing.T) {
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
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, mockCoreClient)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowStartCmd(nil)
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", false, false, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowStart(cmd, args, nil)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowStartCmd(nil)
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowStart(cmd, args, nil)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowUpgradeTest(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("UpgradeTest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false, false, nil).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeTest(cmd, nil)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("UpgradeTest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false, false, nil).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
		// Clean up init files after test
		defer func() { cleanUpInitFiles(t) }()
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("Both airflow and runtime version used", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		airflowVersion = "something"
		runtimeVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errInvalidBothAirflowAndRuntimeVersionsUpgrade)
	})

	t.Run("Both custom image and deployment id used", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		deploymentID = "something"
		customImageName = "something"

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errInvalidBothDeploymentIDandCustomImage)
	})

	t.Run("Both airflow version and deployment id used", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		deploymentID = "something"
		airflowVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errInvalidBothDeploymentIDandVersion)
	})

	t.Run("Both runtime version and deployment id used", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		deploymentID = "something"
		runtimeVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errInvalidBothDeploymentIDandVersion)
	})

	t.Run("Both runtime version and custom image used", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		customImageName = "something"
		runtimeVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errInvalidBothCustomImageandVersion)
	})

	t.Run("Both airflow version and custom image used", func(t *testing.T) {
		cmd := newAirflowUpgradeTestCmd(nil)

		customImageName = "something"
		airflowVersion = "something"

		err := airflowUpgradeTest(cmd, nil)
		assert.ErrorIs(t, err, errInvalidBothCustomImageandVersion)
	})
}

func TestAirflowRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowRunCmd()
		args := []string{"test", "command"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", append([]string{"airflow"}, args...), "").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRun(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowRunCmd()
		args := []string{"test", "command"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", append([]string{"airflow"}, args...), "").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRun(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowRunCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowRun(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowPS(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowPSCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("PS").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowPS(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowPSCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("PS").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowPS(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowPSCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowPS(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowLogs(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("without any component flag", func(t *testing.T) {
		cmd := newAirflowLogsCmd()
		cmd.Flag("follow").Value.Set("true")
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Logs", true, "webserver", "scheduler", "triggerer").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowLogs(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowLogsCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowLogs(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowKill(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowKillCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Kill").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowKill(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowKillCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Kill").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowKill(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowKillCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowKill(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowStop(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowStopCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", false).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowStop(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowStopCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", false).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowStop(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowStopCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowStop(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowRestart(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("success", func(t *testing.T) {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, 0, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, nil)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("success with deployment id flag set", func(t *testing.T) {
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
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, mockCoreClient)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("success with workspace id flag set", func(t *testing.T) {
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
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection{envObj.ObjectKey: *envObj.Connection}).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, mockCoreClient)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("stop failure", func(t *testing.T) {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, nil)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("start failure", func(t *testing.T) {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Stop", true).Return(nil).Once()
			mockContainerHandler.On("Start", "", "airflow_settings.yaml", "", "", true, true, 1*time.Minute, map[string]astrocore.EnvironmentObjectConnection(nil)).Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowRestart(cmd, args, nil)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowRestartCmd(nil)
		cmd.Flag("no-cache").Value.Set("true")
		args := []string{"test-env-file"}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowRestart(cmd, args, nil)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowPytest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("0", nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("exit code 1", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("exit code 1", errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		assert.Contains(t, err.Error(), "pytests failed")
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("pytest file doesnot exists", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = "/testfile-not-exists"

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("0", nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		assert.Contains(t, err.Error(), "directory does not exist, please run `astro dev init` to create it")
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Pytest", "test-pytest-file", "", "", "", "").Return("0", errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowPytest(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		cmd := newAirflowPytestCmd()
		args := []string{"test-pytest-file"}
		pytestDir = ""

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowPytest(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("projectNameUnique failure", func(t *testing.T) {
		cmd := newAirflowParseCmd()
		args := []string{}
		pytestDir = ""

		projectNameUnique = func() (string, error) {
			return "", errMock
		}
		defer func() { projectNameUnique = airflow.ProjectNameUnique }()

		err := airflowPytest(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowParse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowParseCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Parse", "", "", "").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowParse(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowParseCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Parse", "", "", "").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowParse(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowParseCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowParse(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("projectNameUnique failure", func(t *testing.T) {
		cmd := newAirflowParseCmd()
		args := []string{}

		projectNameUnique = func() (string, error) {
			return "", errMock
		}
		defer func() { projectNameUnique = airflow.ProjectNameUnique }()

		err := airflowParse(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowUpgradeCheck(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := newAirflowUpgradeCheckCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", airflowUpgradeCheckCmd, "root").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeCheck(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		cmd := newAirflowUpgradeCheckCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Run", airflowUpgradeCheckCmd, "root").Return(errMock).Once()
			return mockContainerHandler, nil
		}

		err := airflowUpgradeCheck(cmd, args)
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowUpgradeCheckCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowUpgradeCheck(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowBash(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("without any component flag", func(t *testing.T) {
		cmd := newAirflowBashCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("Bash", "scheduler").Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowBash(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newAirflowBashCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowBash(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowObjectImport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("without any object flag", func(t *testing.T) {
		cmd := newObjectImportCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ImportSettings", "airflow_settings.yaml", ".env", connections, variables, pools).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsImport(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newObjectImportCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowSettingsImport(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestAirflowObjectExport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("success compose export", func(t *testing.T) {
		cmd := newObjectExportCmd()
		cmd.Flag("compose").Value.Set("true")

		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ComposeExport", "airflow_settings.yaml", exportComposeFile).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("without any object flag", func(t *testing.T) {
		cmd := newObjectExportCmd()
		args := []string{}

		mockContainerHandler := new(mocks.ContainerHandler)
		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			mockContainerHandler.On("ExportSettings", "airflow_settings.yaml", ".env", connections, variables, pools, envExport).Return(nil).Once()
			return mockContainerHandler, nil
		}

		err := airflowSettingsExport(cmd, args)
		assert.NoError(t, err)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
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
		assert.ErrorIs(t, err, errMock)
		mockContainerHandler.AssertExpectations(t)
	})

	t.Run("containerHandlerInit failure", func(t *testing.T) {
		cmd := newObjectExportCmd()
		args := []string{}

		containerHandlerInit = func(airflowHome, envFile, dockerfile, imageName string) (airflow.ContainerHandler, error) {
			return nil, errMock
		}

		err := airflowSettingsExport(cmd, args)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestPrepareDefaultAirflowImageTag(t *testing.T) {
	getDefaultImageTag = func(httpClient *airflowversions.Client, airflowVersion string) (string, error) {
		return "", nil
	}
	t.Run("default airflow version", func(t *testing.T) {
		useAstronomerCertified = true
		resp := prepareDefaultAirflowImageTag("", nil)
		assert.Equal(t, airflowversions.DefaultAirflowVersion, resp)
	})

	t.Run("default runtime version", func(t *testing.T) {
		useAstronomerCertified = false
		resp := prepareDefaultAirflowImageTag("", nil)
		assert.Equal(t, airflowversions.DefaultRuntimeVersion, resp)
	})
}
