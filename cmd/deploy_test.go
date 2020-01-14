package cmd

import (
	"testing"

	testUtil "github.com/sjmiller609/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestDeployRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("deploy")
	assert.EqualError(t, err, "not in a project directory")
	assert.Contains(t, output, "astro deploy")
}
