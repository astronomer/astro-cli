//go:build windows

package airflowrt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The helpers below are vars so tests can stub them.

// isProcessAlive checks whether a process with the given PID is running on Windows.
var isProcessAlive = func(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH") //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return !strings.Contains(string(output), "No tasks")
}

// isProcessGroupAlive falls back to checking the single PID on Windows, where
// Unix-style process groups don't apply and terminateProcess only targets the
// master process.
var isProcessGroupAlive = func(pid int) bool {
	return isProcessAlive(pid)
}

// terminateProcess kills the process on Windows (no SIGTERM equivalent).
var terminateProcess = func(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck
}

// killProcess force-kills the process on Windows.
var killProcess = func(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck
}
