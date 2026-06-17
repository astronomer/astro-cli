//go:build windows

package airflow

import "os/exec"

// longSleepCommand returns a cross-platform command that runs for ~60 seconds.
func longSleepCommand() *exec.Cmd {
	return exec.Command("cmd", "/C", "timeout", "/T", "60", "/NOBREAK")
}
