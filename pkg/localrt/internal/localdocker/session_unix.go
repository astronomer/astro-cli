//go:build !windows

package localdocker

import "syscall"

// detachSysProcAttr puts the watcher in its own process group so it outlives
// the starting CLI and survives a Ctrl-C aimed at the starter's group.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
