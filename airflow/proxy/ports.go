package proxy

import (
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// AllocatePort picks a random available port from the pool (10000-19999).
func AllocatePort() (string, error) {
	ensureInit()
	return pkgproxy.AllocatePort()
}

// IsPortAvailable checks if a port is free by attempting to connect.
func IsPortAvailable(port string) bool {
	return pkgproxy.IsPortAvailable(port)
}
