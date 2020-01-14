package cmd

import (
	"testing"

	testUtil "github.com/sjmiller609/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand()
	assert.NoError(t, err)
	assert.Contains(t, output, "astro [command]")
}
