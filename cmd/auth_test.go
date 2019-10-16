package cmd

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestAuthRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("auth")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro auth")
}