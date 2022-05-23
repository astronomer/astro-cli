package cmd

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestVersionRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// Locally we are not support version, that's why we see err
	expectedOut := ""
	output, err := executeCommand("version")
	assert.Equal(t, expectedOut, output, err)
}
