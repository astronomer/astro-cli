//go:build !windows

package airflow

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to start in its own process group so
// the entire tree (scheduler, triggerer, api-server, etc.) can be killed together.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcessGroup sends SIGTERM to the process group led by pid.
func terminateProcessGroup(pid int) {
	syscall.Kill(-pid, syscall.SIGTERM) //nolint:errcheck
}

// killProcessGroup sends SIGKILL to the process group led by pid.
func killProcessGroup(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck
}

// interruptSignals returns the OS signals to listen for on graceful shutdown.
func interruptSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// venvBinDir returns the subdirectory name for executables inside a Python venv.
func venvBinDir() string {
	return "bin"
}

// shellCommand returns an *exec.Cmd that executes a shell command string.
func shellCommand(command string) *exec.Cmd {
	return exec.Command("bash", "-c", command) //nolint:gosec
}

// interactiveShellArgs returns the command and arguments for an interactive shell.
func interactiveShellArgs() []string {
	return []string{"bash"}
}
