package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func execFlowCmd(args ...string) error {
	cmd := NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestFlowCmd(t *testing.T) {
	err := execFlowCmd([]string{}...)
	assert.NoError(t, err)
}

func TestFlowVersionCmd(t *testing.T) {
	err := execFlowCmd([]string{"version"}...)
	assert.NoError(t, err)
}

func TestFlowAboutCmd(t *testing.T) {
	err := execFlowCmd([]string{"about"}...)
	assert.NoError(t, err)
}

func TestFlowInitCmd(t *testing.T) {
	tempDir := t.TempDir()
	tempDirAirflowHome := t.TempDir()
	tempDirAirflowDagsFolder := t.TempDir()
	err := execFlowCmd([]string{"init", tempDir, "--airflow_home", tempDirAirflowHome, "--airflow_dags_folder", tempDirAirflowDagsFolder}...)
	assert.NoError(t, err)
}

func TestFlowValidateCmd(t *testing.T) {
	tempDir := t.TempDir()
	err := execFlowCmd([]string{"init", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"validate", tempDir, "--connection", "sqlite_conn"}...)
	assert.NoError(t, err)
}

func TestFlowGenerateCmd(t *testing.T) {
	tempDir := t.TempDir()
	err := execFlowCmd([]string{"init", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"generate", "example_basic_transform", "--project_dir", tempDir}...)
	assert.NoError(t, err)
}

func TestFlowGenerateCmdWorkflowNameNotSet(t *testing.T) {
	tempDir := t.TempDir()
	err := execFlowCmd([]string{"init", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"generate"}...)
	assert.EqualError(t, err, "argument not set:workflow_name")
}

func TestFlowRunCmd(t *testing.T) {
	tempDir := t.TempDir()
	err := execFlowCmd([]string{"init", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"run", "example_templating", "--env", "dev", "--project_dir", tempDir}...)
	assert.NoError(t, err)
}

func TestFlowRunCmdWorkflowNameNotSet(t *testing.T) {
	tempDir := t.TempDir()
	err := execFlowCmd([]string{"init", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"run"}...)
	assert.EqualError(t, err, "argument not set:workflow_name")
}
