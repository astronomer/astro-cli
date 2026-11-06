//go:build e2e && windows

package e2e

import "os/exec"

// dumpStacksOnTimeout leaves cmd as it is on Windows, which has no SIGQUIT to
// ask a Go program for its stacks with: a command that outlives its bound is
// killed, and its failure says only that it hung. See the Unix version.
func dumpStacksOnTimeout(*exec.Cmd) {}
