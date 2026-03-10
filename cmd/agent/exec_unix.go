//go:build !windows

package agent

import (
	"os"
	"syscall"
)

// execOpencode replaces the current process with the opencode binary.
// On Unix, syscall.Exec gives opencode full control of the terminal
// (TUI rendering, signal handling, etc.) with no parent process overhead.
func execOpencode(binPath string, args []string) error {
	argv := append([]string{binaryName}, args...)
	return syscall.Exec(binPath, argv, os.Environ())
}
