package e2e

import (
	"fmt"
	"os"
	"testing"

	sql "github.com/astronomer/astro-cli/cmd/sql"

	"github.com/stretchr/testify/assert"
)

// chdir changes the current working directory to the named directory and
// returns a function that, when called, restores the original working
// directory.
func chdir(t *testing.T, dir string) func() {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restoring working directory: %v", err)
		}
	}
}

func execFlowCmd(args ...string) error {
	cmd := sql.NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestE2EFlowCmd(t *testing.T) {
	err := execFlowCmd()
	assert.NoError(t, err)
}

func TestE2EFlowVersionCmd(t *testing.T) {
	err := execFlowCmd("version")
	assert.NoError(t, err)
}

func TestE2EFlowAboutCmd(t *testing.T) {
	err := execFlowCmd("about")
	assert.NoError(t, err)
}

func TestE2EFlowInitCmd(t *testing.T) {
	projectDir := t.TempDir()
	defer chdir(t, projectDir)()
	err := execFlowCmd("init")
	assert.NoError(t, err)
}

func TestE2EFlowInitCmdWithFlags(t *testing.T) {
	projectDir := t.TempDir()
	airflowHome := t.TempDir()
	airflowDagsFolder := t.TempDir()
	err := execFlowCmd("init", projectDir, "--airflow-home", airflowHome, "--airflow-dags-folder", airflowDagsFolder)
	assert.NoError(t, err)
}

func TestE2EFlowConfigCmd(t *testing.T) {
	projectDir := t.TempDir()
	airflowHome := t.TempDir()
	airflowDagsFolder := t.TempDir()
	err := execFlowCmd("init", projectDir, "--airflow-home", airflowHome, "--airflow-dags-folder", airflowDagsFolder)
	assert.NoError(t, err)

	err = execFlowCmd("config", "--project-dir", projectDir, "airflow_home")
	assert.NoError(t, err)
}

func TestE2EFlowConfigCmdArgumentNotSetError(t *testing.T) {
	projectDir := t.TempDir()
	airflowHome := t.TempDir()
	airflowDagsFolder := t.TempDir()
	err := execFlowCmd("init", projectDir, "--airflow-home", airflowHome, "--airflow-dags-folder", airflowDagsFolder)
	assert.NoError(t, err)

	err = execFlowCmd("config", "--project-dir", projectDir)
	assert.EqualError(t, err, "argument not set:key")
}

func TestE2EFlowValidateCmd(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("validate", projectDir, "--connection", "sqlite_conn")
	assert.NoError(t, err)
}

func TestE2EFlowValidateAllCmd(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("validate", projectDir)
	assert.NoError(t, err)
}

func TestE2EFlowGenerateCmd(t *testing.T) {
	testCases := []struct {
		workflowName  string
		env           string
		generateTasks string
	}{
		{"example_basic_transform", "default", "--generate-tasks"},
		{"example_basic_transform", "default", "--no-generate-tasks"},
		{"example_templating", "dev", "--generate-tasks"},
		{"example_templating", "dev", "--no-generate-tasks"},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s in %s with %s", tc.workflowName, tc.env, tc.generateTasks), func(t *testing.T) {
			projectDir := t.TempDir()
			err := execFlowCmd("init", projectDir)
			assert.NoError(t, err)

			err = execFlowCmd("generate", tc.workflowName, "--project-dir", projectDir, "--env", tc.env, tc.generateTasks)
			assert.NoError(t, err)
		})
	}
}

func TestE2EFlowGenerateCmdWorkflowNameNotSetError(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("generate", "--project-dir", projectDir)
	assert.EqualError(t, err, "argument not set:workflow_name")
}

func TestE2EFlowRunCmd(t *testing.T) {
	testCases := []struct {
		workflowName  string
		env           string
		generateTasks string
	}{
		{"example_basic_transform", "default", "--generate-tasks"},
		{"example_basic_transform", "default", "--no-generate-tasks"},
		{"example_templating", "dev", "--generate-tasks"},
		{"example_templating", "dev", "--no-generate-tasks"},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s in %s with %s", tc.workflowName, tc.env, tc.generateTasks), func(t *testing.T) {
			projectDir := t.TempDir()
			err := execFlowCmd("init", projectDir)
			assert.NoError(t, err)

			err = execFlowCmd("run", tc.workflowName, "--project-dir", projectDir, "--env", tc.env, tc.generateTasks)
			assert.NoError(t, err)
		})
	}
}

func TestE2EFlowRunVerboseCmd(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("run", "example_basic_transform", "--project-dir", projectDir, "--verbose")
	assert.NoError(t, err)
}

func TestE2EFlowRunCmdWorkflowNameNotSetError(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("run", "--project-dir", projectDir)
	assert.EqualError(t, err, "argument not set:workflow_name")
}
