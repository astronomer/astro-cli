//go:build !windows

package agent

import (
	"os"
	"syscall"
)

// execAgent replaces the current process with the agent binary.
// On Unix, syscall.Exec gives the agent full control of the terminal
// (TUI rendering, signal handling, etc.) with no parent process overhead.
func execAgent(binPath string, args []string) error {
	argv := append([]string{binaryName}, args...)
	return syscall.Exec(binPath, argv, os.Environ())
}
