package proxy

import (
	"fmt"
	"os"
	"testing"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllocatePort(t *testing.T) {
	setupTestDir(t)

	// Override isPortAvailable via pkg/proxy's internal testing support
	// Since we can't access pkg/proxy's unexported isPortAvailable from here,
	// we test via the exported wrapper which delegates to pkg/proxy.
	// The actual port availability logic is tested in pkg/proxy/ports_test.go.

	port, err := AllocatePort()
	require.NoError(t, err)
	assert.NotEmpty(t, port)

	// Port should be in range
	var portNum int
	_, err = fmt.Sscanf(port, "%d", &portNum)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, portNum, 10000)
	assert.LessOrEqual(t, portNum, 19999)
}

func TestAllocatePort_AvoidAllocated(t *testing.T) {
	setupTestDir(t)

	// Pre-register a route so its port is taken
	_ = AddRoute(&Route{
		Hostname:   "existing.localhost",
		Port:       "12345",
		ProjectDir: "/tmp/existing",
		PID:        os.Getpid(),
	})

	// Allocate many ports and ensure none are 12345
	for range 20 {
		port, err := AllocatePort()
		require.NoError(t, err)
		assert.NotEqual(t, "12345", port)
	}
}

func TestIsPortAvailable(t *testing.T) {
	// Just verify the wrapper delegates correctly
	result := IsPortAvailable("1") // port 1 should not be available (privileged)
	// We can't predict the exact result, but it should not panic
	_ = result
}

func TestIsPortAvailable_Delegates(t *testing.T) {
	// Verify that the wrapper calls through to pkg/proxy
	assert.Equal(t, pkgproxy.IsPortAvailable("99999"), IsPortAvailable("99999"))
}
