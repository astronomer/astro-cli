//go:build !windows

package airflowrt

import (
	"os"
	"syscall"
)

// isProcessAlive checks whether a process with the given PID is running.
// On Unix, FindProcess always succeeds, so we probe with signal 0.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// terminateProcess sends SIGTERM to the process group.
func terminateProcess(pid int) {
	// A group that is already gone is the outcome asked for, and the caller
	// polls for exit rather than trusting this return.
	syscall.Kill(-pid, syscall.SIGTERM) //nolint:errcheck // deliberate, for the reason above
}

// killProcess sends SIGKILL to the process group.
func killProcess(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck // as above: this is the last resort, and there is nothing after it to tell
}
