//go:build windows

package airflowrt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// isProcessAlive checks whether a process with the given PID is running on Windows.
func isProcessAlive(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH") //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return !strings.Contains(string(output), "No tasks")
}

// terminateProcess kills the process on Windows (no SIGTERM equivalent).
func terminateProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck
}

// killProcess force-kills the process on Windows.
func killProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck
}
