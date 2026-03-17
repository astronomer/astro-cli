//go:build !windows

package proxy

import (
	"os"
	"syscall"
)

// IsPIDAlive checks if a process with the given PID is still running.
var IsPIDAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
