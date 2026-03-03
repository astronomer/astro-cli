package proxy

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllocatePort(t *testing.T) {
	setupTestDir(t)

	// Override isPortAvailable to always return true
	origIsPortAvailable := isPortAvailable
	defer func() { isPortAvailable = origIsPortAvailable }()
	isPortAvailable = func(_ string) bool { return true }

	port, err := AllocatePort()
	require.NoError(t, err)
	assert.NotEmpty(t, port)

	// Port should be in range
	var portNum int
	_, err = fmt.Sscanf(port, "%d", &portNum)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, portNum, portRangeMin)
	assert.LessOrEqual(t, portNum, portRangeMax)
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

	// Override isPortAvailable to always return true
	origIsPortAvailable := isPortAvailable
	defer func() { isPortAvailable = origIsPortAvailable }()
	isPortAvailable = func(_ string) bool { return true }

	// Allocate many ports and ensure none are 12345
	for range 100 {
		port, err := AllocatePort()
		require.NoError(t, err)
		assert.NotEqual(t, "12345", port)
	}
}

func TestAllocatePort_AllBusy(t *testing.T) {
	setupTestDir(t)

	// Override isPortAvailable to always return false
	origIsPortAvailable := isPortAvailable
	defer func() { isPortAvailable = origIsPortAvailable }()
	isPortAvailable = func(_ string) bool { return false }

	_, err := AllocatePort()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find an available port")
}
