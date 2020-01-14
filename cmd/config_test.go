package cmd

import (
	"testing"

	testUtil "github.com/sjmiller609/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestConfigRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("config")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro config")
}
