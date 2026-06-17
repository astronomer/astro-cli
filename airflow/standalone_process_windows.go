//go:build windows

package airflow

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to start in a new process group on Windows
// using CREATE_NEW_PROCESS_GROUP so child processes can be terminated together.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// terminateProcessGroup kills the process tree rooted at pid using taskkill /T.
func terminateProcessGroup(pid int) {
	exec.Command("taskkill", "/T", "/PID", fmt.Sprintf("%d", pid)).Run() //nolint:errcheck,gosec
}

// killProcessGroup forcefully kills the process tree rooted at pid.
func killProcessGroup(pid int) {
	exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run() //nolint:errcheck,gosec
}

// interruptSignals returns the OS signals to listen for on graceful shutdown.
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// venvBinDir returns the subdirectory name for executables inside a Python venv.
func venvBinDir() string {
	return "Scripts"
}
