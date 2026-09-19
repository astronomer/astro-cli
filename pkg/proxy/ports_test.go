package proxy

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllocatePort(t *testing.T) {
	s := testStore(t)

	// Override isPortAvailable to always return true
	origIsPortAvailable := isPortAvailable
	defer func() { isPortAvailable = origIsPortAvailable }()
	isPortAvailable = func(_ string) bool { return true }

	port, err := s.AllocatePort()
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
	s := testStore(t)

	// Pre-register a route so its port is taken
	_ = s.AddRoute(&Route{
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
		port, err := s.AllocatePort()
		require.NoError(t, err)
		assert.NotEqual(t, "12345", port)
	}
}

func TestIsPortAvailable(t *testing.T) {
	// Override the internal function to control results
	origIsPortAvailable := isPortAvailable
	defer func() { isPortAvailable = origIsPortAvailable }()

	isPortAvailable = func(_ string) bool { return true }
	assert.True(t, IsPortAvailable("9999"))

	isPortAvailable = func(_ string) bool { return false }
	assert.False(t, IsPortAvailable("9999"))
}

func TestAllocatePort_AllBusy(t *testing.T) {
	s := testStore(t)

	// Override isPortAvailable to always return false
	origIsPortAvailable := isPortAvailable
	defer func() { isPortAvailable = origIsPortAvailable }()
	isPortAvailable = func(_ string) bool { return false }

	_, err := s.AllocatePort()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "finding an available port")
}

// routes.json records ports that are reserved but not yet bound, and the
// availability check cannot see those — it only dials. So a routes file that
// cannot be read is not "no ports taken", and allocating anyway would hand a
// second project a port the first is holding.
func TestAllocatePortRefusesWhenTheAllocationsCannotBeRead(t *testing.T) {
	seed := testStore(t)
	require.NoError(t, seed.AddRoute(&Route{Hostname: "taken.localtest.me", Port: "10001"}))

	// A directory standing where the file should be, rather than chmod 0o000.
	// Windows has no Unix mode bits — a 0o000 file is still readable there, and
	// os.Geteuid does not identify an administrator — but reading a directory
	// as a file fails everywhere.
	path := seed.routesFilePath()
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0o750))

	// A second store over the same directory, because the one that wrote the
	// file answers from its cache and never reads it back.
	s := NewStore(seed.dir)

	origIsPortAvailable := isPortAvailable
	t.Cleanup(func() { isPortAvailable = origIsPortAvailable })
	isPortAvailable = func(_ string) bool { return true }

	_, err := s.AllocatePort()
	require.Error(t, err, "a port was handed out without knowing which are already held")
}
