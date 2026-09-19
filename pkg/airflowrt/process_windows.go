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
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH") //nolint:gosec // G204: a fixed system command, and pid goes through %d rather than into the command line as text
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
	// A process that cannot be killed is one the caller polls for anyway, and
	// a process already gone is the outcome asked for.
	proc.Kill() //nolint:errcheck // deliberate, for the reason above
}

// killProcess force-kills the process on Windows.
func killProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Kill() //nolint:errcheck // as above: this is the last resort, and nothing follows it to tell
}
