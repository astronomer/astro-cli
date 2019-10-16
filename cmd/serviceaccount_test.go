package cmd

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceAccountRootCommand(t *testing.T) {
	testUtil.InitTestConfig()
	output, err := executeCommand("service-account")
	assert.NoError(t, err)
	assert.Contains(t, output, "service-account")
}
