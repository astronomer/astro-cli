//go:build windows

package airflow

import "os/exec"

// longSleepCommand returns a cross-platform command that runs for ~60 seconds.
func longSleepCommand() *exec.Cmd {
	return exec.Command("cmd", "/C", "timeout", "/T", "60", "/NOBREAK")
}

// failCommand returns a command that exits with code 1 to produce an *exec.ExitError.
func failCommand() *exec.Cmd {
	return exec.Command("cmd", "/C", "exit", "1") //nolint:gosec
}
