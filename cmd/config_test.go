package cmd

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestConfigRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("config")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro config")
}

func TestConfigGetCommandSuccess(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("config", "get", "project.name", "-g")
	assert.NoError(t, err)
}

func TestConfigGetCommandFailure(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("config", "get", "-g", "test")
	assert.Error(t, err)
	assert.Contains(t, output, "Config does not exist, check your config key")
}

func TestConfigSetCommandFailure(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("config", "set", "test", "testing", "-g")
	assert.Error(t, err)
}

func TestConfigSetCommandFailurePrjConfig(t *testing.T) {
	testUtil.InitTestConfig()
	_, err := executeCommand("config", "set", "-g", "project.name", "testing")
	assert.Error(t, err)
}
