package cmd

import (
	"testing"

	testUtil "github.com/sjmiller609/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestVersionRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	// Locally we are not support version, that's why we see err
	expectedOut := "Error: Astronomer CLI version is not valid\n"
	output, err := executeCommand("version")
	assert.Equal(t, expectedOut, output, err)
}
