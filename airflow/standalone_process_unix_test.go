//go:build !windows

package airflow

import "os/exec"

// longSleepCommand returns a cross-platform command that runs for ~60 seconds.
func longSleepCommand() *exec.Cmd {
	return exec.Command("sleep", "60")
}

// failCommand returns a command that exits with code 1 to produce an *exec.ExitError.
func failCommand() *exec.Cmd {
	return exec.Command("sh", "-c", "exit 1") //nolint:gosec
}
