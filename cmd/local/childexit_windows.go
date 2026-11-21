//go:build windows

package local

import "os/exec"

// childExitCode is the status a wrapped command's exit carries out of the CLI.
// Windows has no signal deaths: the exit code is the whole story.
func childExitCode(exitErr *exec.ExitError) int { return exitErr.ExitCode() }
