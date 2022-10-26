package sql

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func execFlowCmd(args ...string) error {
	cmd := NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestFlowCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	tempDir := t.TempDir()

	err := execFlowCmd([]string{"version"}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"about"}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"init", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"validate", "--project_dir", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"generate", "example_basic_transform", "--project_dir", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"run", "example_basic_transform", "--project_dir", tempDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"run", "example_templating", "--env", "dev", "--project_dir", tempDir}...)
	assert.NoError(t, err)
}
