//go:build windows

package airflow

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// setProcGroup configures the command to start in a new process group on Windows
// using CREATE_NEW_PROCESS_GROUP so child processes can be terminated together.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// terminateProcessGroup sends CTRL_BREAK_EVENT to the process group led by pid.
// This is the Windows equivalent of Unix SIGTERM for console processes.
// CREATE_NEW_PROCESS_GROUP (set in setProcGroup) makes the child the leader of
// its own console group, so the event reaches the entire Airflow process tree.
func terminateProcessGroup(pid int) {
	windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)) //nolint:errcheck
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

// shellCommand returns an *exec.Cmd that executes a shell command string.
func shellCommand(command string) *exec.Cmd {
	return exec.Command("cmd", "/C", command) //nolint:gosec
}

// interactiveShellArgs returns the command and arguments for an interactive shell.
func interactiveShellArgs() []string {
	return []string{"cmd"}
}
