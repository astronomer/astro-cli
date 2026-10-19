//go:build !windows

package localdocker

import (
	"os/exec"
	"syscall"
)

// killTreeOnCancel starts cmd in a process group of its own and makes its
// cancel kill that whole group, so the children it spawned go with it.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
