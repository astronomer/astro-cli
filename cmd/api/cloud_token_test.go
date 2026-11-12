package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
)

func TestFillPlaceholdersWithNoWorkspaceUnderATokenSaysToWriteTheID(t *testing.T) {
	t.Setenv("ASTRO_API_TOKEN", "token")
	_, err := fillPlaceholders("/workspaces/{workspaceId}", &config.Context{Organization: "org-123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "an Organization token does not")
	assert.Contains(t, err.Error(), "write the id in the path")
	assert.NotContains(t, err.Error(), "workspace switch")

	t.Setenv("ASTRO_API_TOKEN", "")
	_, err = fillPlaceholders("/workspaces/{workspaceId}", &config.Context{Organization: "org-123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "astro workspace switch")
}
