//go:build windows

package proxy

import (
	"fmt"
)

var errUnsupportedWindows = fmt.Errorf("proxy daemon is not supported on Windows")

// StartDaemon is not supported on Windows.
var StartDaemon = func(port string) error {
	return errUnsupportedWindows
}

// IsRunning always returns false on Windows.
func IsRunning() (int, bool) {
	return 0, false
}

// EnsureRunning returns an error on Windows.
func EnsureRunning(port string) (string, error) {
	return "", errUnsupportedWindows
}

// StopDaemon is a no-op on Windows.
func StopDaemon() error {
	return nil
}

// StopIfEmpty is a no-op on Windows.
func StopIfEmpty() {}
