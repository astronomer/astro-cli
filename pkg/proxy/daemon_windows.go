//go:build windows

package proxy

import (
	"errors"
	"os/exec"
)

// errUnsupportedWindows is every lifecycle answer on Windows, where the daemon
// does not run yet. A host there serves its own proxy in process instead.
var errUnsupportedWindows = errors.New("proxy daemon is not supported on Windows")

// startDetached is not supported on Windows.
func startDetached(func() *exec.Cmd) (*exec.Cmd, error) { return nil, errUnsupportedWindows }

// requestStop has nothing to ask on Windows: no daemon can be running.
var requestStop = func(int) bool { return false }

// killProcess has nothing to end on Windows.
var killProcess = func(int) {}

// stopRequests is not supported on Windows.
func stopRequests() (<-chan struct{}, func(), error) { return nil, nil, errUnsupportedWindows }

// processLooksLikeProxy never matches on Windows, where no daemon runs.
var processLooksLikeProxy = func(*Daemon, int) bool { return false }
