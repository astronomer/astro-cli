//go:build windows

package airflowrt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// isProcessAlive checks whether a process with the given PID is running on Windows.
// Declared as a var so tests can stub it.
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
// master process. Declared as a var so tests can stub it.
var isProcessGroupAlive = func(pid int) bool {
	return isProcessAlive(pid)
}

// terminateProcess kills the process on Windows (no SIGTERM equivalent).
// Declared as a var so tests can stub it.
var terminateProcess = func(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck
}

// killProcess force-kills the process on Windows.
// Declared as a var so tests can stub it.
var killProcess = func(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck
}
