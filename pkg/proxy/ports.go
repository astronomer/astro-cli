package proxy

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

const (
	portRangeMin = 10000
	portRangeMax = 19999
	maxRetries   = 50
	dialTimeout  = 500 * time.Millisecond
)

// AllocatePort picks a random available port from the pool (10000-19999).
// It checks that the port is not already in use by another process and not
// already allocated in routes.json.
func (s *Store) AllocatePort() (string, error) {
	// Refused rather than guessed at. A missing or empty routes file reads as no
	// routes, so an error here is a real one — and the availability check below
	// only sees ports something is LISTENING on, while routes.json also records
	// ports held across a provisioning run that has not bound them yet (see
	// Reserve in pkg/localrt). Allocating without that list hands a reserved
	// port to a second project, which is the collision the reservation exists
	// to prevent.
	routes, err := s.ReadRoutes()
	if err != nil {
		return "", fmt.Errorf("reading the ports already allocated: %w", err)
	}
	allocated := map[string]bool{}
	for _, r := range routes {
		allocated[r.Port] = true
		for _, p := range r.Services {
			allocated[p] = true
		}
	}

	for range maxRetries {
		port := fmt.Sprintf("%d", portRangeMin+rand.Intn(portRangeMax-portRangeMin+1)) //nolint:gosec // G404: spreading attempts across the range, not generating a secret

		// Skip if already allocated
		if allocated[port] {
			continue
		}

		// Check if port is available on the system
		if IsPortAvailable(port) {
			return port, nil
		}
	}

	return "", fmt.Errorf("finding an available port: %d attempts exhausted", maxRetries)
}

// IsPortAvailable checks if a port is free by attempting to connect.
func IsPortAvailable(port string) bool {
	return isPortAvailable(port)
}

// isPortAvailable checks if a port is free by attempting to connect.
var isPortAvailable = func(port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), dialTimeout)
	if err != nil {
		return true // Connection refused / timeout → port is free
	}
	conn.Close() //nolint:errcheck // the dial answered the question; the close is tidiness
	return false
}
