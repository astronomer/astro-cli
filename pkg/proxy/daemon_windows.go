//go:build windows

package proxy

import "errors"

// errUnsupportedWindows is every lifecycle answer on Windows, where the daemon
// does not run yet. A host there serves its own proxy in process instead.
var errUnsupportedWindows = errors.New("proxy daemon is not supported on Windows")

// EnsureRunning is not supported on Windows.
func (d *Daemon) EnsureRunning(string) (string, error) { return "", errUnsupportedWindows }

// Start is not supported on Windows.
func (d *Daemon) Start(string) (string, error) { return "", errUnsupportedWindows }

// Serve is not supported on Windows.
func (d *Daemon) Serve(string) error { return errUnsupportedWindows }

// Stop is a no-op on Windows: no daemon can be running.
func (d *Daemon) Stop() error { return nil }

// StopIfEmpty is a no-op on Windows.
func (d *Daemon) StopIfEmpty() {}
