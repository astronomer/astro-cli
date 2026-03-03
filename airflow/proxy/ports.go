package proxy

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

const (
	portRangeMin  = 10000
	portRangeMax  = 19999
	maxRetries    = 50
	dialTimeout   = 500 * time.Millisecond
)

// AllocatePort picks a random available port from the pool (10000-19999).
// It checks that the port is not already in use by another process and not
// already allocated in routes.json.
func AllocatePort() (string, error) {
	// Read existing routes to avoid collisions
	allocated := map[string]bool{}
	routes, _ := readRoutes() // ignore error — best effort
	for _, r := range routes {
		allocated[r.Port] = true
		for _, p := range r.Services {
			allocated[p] = true
		}
	}

	for range maxRetries {
		port := fmt.Sprintf("%d", portRangeMin+rand.Intn(portRangeMax-portRangeMin+1)) //nolint:gosec

		// Skip if already allocated
		if allocated[port] {
			continue
		}

		// Check if port is available on the system
		if isPortAvailable(port) {
			return port, nil
		}
	}

	return "", fmt.Errorf("failed to find an available port after %d attempts", maxRetries)
}

// isPortAvailable checks if a port is free by attempting to connect.
var isPortAvailable = func(port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), dialTimeout)
	if err != nil {
		return true // Connection refused / timeout → port is free
	}
	conn.Close()
	return false
}
