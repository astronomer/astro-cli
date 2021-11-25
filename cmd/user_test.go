package cmd

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestUserRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("user")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro user")
}
