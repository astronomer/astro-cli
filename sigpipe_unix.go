//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// watchClosedStdout makes a write to a closed stdout return EPIPE instead of
// ending the process by SIGPIPE (exit 141), which Go does for fd 1 and 2
// unless the program has asked for the signal, and records that it happened
// (cliout.NoteSIGPIPE). It cancels nothing: a deploy whose output was piped
// into `head -1` still finishes, and what ends a command is its own error from
// the write that failed, which cliout.ExitCode makes 0.
//
// Notify rather than Ignore: an ignored signal is inherited by the children
// a command starts (`astro local run`), which would turn their own broken
// pipes into write errors; a handler is reset to the default across exec, so
// a child still owns its stdout.
func watchClosedStdout() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGPIPE)
	go func() {
		for range sigs {
			cliout.NoteSIGPIPE()
		}
	}()
}
