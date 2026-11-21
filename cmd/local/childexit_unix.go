//go:build !windows

package local

import (
	"os/exec"
	"syscall"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// childExitCode is the status a wrapped command's exit carries out of the CLI.
// A child ended by a signal has no exit code (ExitCode reports -1, which a
// shell would show as 255), so it becomes 128+n, as a shell reports it.
func childExitCode(exitErr *exec.ExitError) int {
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return cliout.SignalExitCode(int(ws.Signal()))
	}
	return exitErr.ExitCode()
}
