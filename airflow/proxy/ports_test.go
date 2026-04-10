package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// Port allocation logic is thoroughly tested in pkg/proxy/ports_test.go.
// These tests verify the CLI wrapper delegates correctly and that
// ensureInit sets up the routes directory.

func TestAllocatePort_Delegates(t *testing.T) {
	setupTestDir(t)

	port, err := AllocatePort()
	require.NoError(t, err)
	assert.NotEmpty(t, port)
}

func TestIsPortAvailable_Delegates(t *testing.T) {
	assert.Equal(t, pkgproxy.IsPortAvailable("99999"), IsPortAvailable("99999"))
}
