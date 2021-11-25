package cmd

import (
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand()
	assert.NoError(t, err)
	assert.Contains(t, output, "astro [command]")
}

func TestSetupLogsDefault(t *testing.T) {
	testUtil.InitTestConfig()
	err := SetUpLogs(os.Stdout, "warning")
	assert.NoError(t, err)
}

func TestSetupLogsDebug(t *testing.T) {
	testUtil.InitTestConfig()
	err := SetUpLogs(os.Stdout, "debug")
	assert.NoError(t, err)
}
