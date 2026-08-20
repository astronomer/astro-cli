//go:build windows

package localdocker

import "syscall"

// detachedProcess starts the watcher without a console, so it outlives the CLI
// it descends from. Value from the Win32 process-creation flags.
const detachedProcess = 0x00000008

// detachSysProcAttr detaches the watcher into its own process group so it
// survives the starting CLI's exit and a Ctrl-C on that console.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}
