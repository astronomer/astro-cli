//go:build !windows

package airflowrt

import (
	"os"
	"syscall"
)

// The helpers below are vars so tests can stub them.

// isProcessAlive checks whether a process with the given PID is running.
// On Unix, FindProcess always succeeds, so we probe with signal 0.
var isProcessAlive = func(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// isProcessGroupAlive reports whether any process in pid's group is still
// reachable. kill(-pgid, 0) returns nil iff at least one member is alive, so
// this stays true while scheduler/triggerer children outlive the master.
var isProcessGroupAlive = func(pid int) bool {
	return syscall.Kill(-pid, syscall.Signal(0)) == nil
}

// terminateProcess sends SIGTERM to the process group.
var terminateProcess = func(pid int) {
	syscall.Kill(-pid, syscall.SIGTERM) //nolint:errcheck
}

// killProcess sends SIGKILL to the process group.
var killProcess = func(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck
}
