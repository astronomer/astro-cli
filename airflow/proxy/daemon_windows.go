//go:build windows

package proxy

import (
	"fmt"
)

var errUnsupportedWindows = fmt.Errorf("proxy daemon is not supported on Windows")

// StartDaemon is not supported on Windows.
var StartDaemon = func(port string) (string, error) {
	return "", errUnsupportedWindows
}

// BoundPort always returns "" on Windows.
func BoundPort() string {
	return ""
}

// EnsureRunning returns an error on Windows.
func EnsureRunning(port string) (string, error) {
	return "", errUnsupportedWindows
}

// Serve is not supported on Windows.
func Serve(port string) error {
	return errUnsupportedWindows
}

// StopIfEmpty is a no-op on Windows.
func StopIfEmpty() {}
