package cmd

import (
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("config")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro config")
}

func TestConfigGetCommandSuccess(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "get", "project.name", "-g")
	assert.NoError(t, err)
}

func TestConfigGetCommandFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "get", "-g", "test")
	assert.Error(t, err)
	assert.EqualError(t, err, errInvalidConfigPath.Error())

	err = os.RemoveAll("./.astro")
	assert.Error(t, err)
	_, err = executeCommand("config", "get", "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "You are attempting to get [setting-name] a project config outside of a project directory")
}

func TestConfigSetCommandFailure(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set", "test", "testing", "-g")
	assert.Error(t, err)

	_, err = executeCommand("config", "set", "test", "-g")
	assert.ErrorIs(t, err, errInvalidSetArgs)
}

func TestConfigSetCommandSuccess(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set", "-g", "project.name", "testing")
	assert.NoError(t, err)
}
