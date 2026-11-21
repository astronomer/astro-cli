//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// keepRunningOnClosedStdout makes a write to a closed stdout return EPIPE
// instead of ending the process by SIGPIPE (exit 141), which Go does for fd 1
// and 2 unless the program has asked for the signal. The write error then
// travels up as the command's own error, and cliout.ExitCode makes it 0
// (cliout.ClosedStdout); nothing is canceled, so a deploy piped into
// `head -1` still finishes.
//
// Notify rather than Ignore: an ignored signal is inherited by the children
// a command starts (`astro local run`), which would turn their own broken
// pipes into write errors; a handler is reset to the default across exec, so
// a child still owns its stdout.
func keepRunningOnClosedStdout() {
	// Never read: a signal that finds the channel full is dropped.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)
}
