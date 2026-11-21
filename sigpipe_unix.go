//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// watchClosedStdout makes a write to a closed stdout return EPIPE instead of
// ending the process by SIGPIPE (exit 141), which Go does for fd 1 and 2
// unless the program has asked for the signal, and cancels the run's context
// with cliout.ErrClosedStdout when that happens. The waits, polls, pages and
// uploads that watch the context then stop as they do for an interrupt, rather
// than running on for a reader that has left; cliout.ExitCode makes the exit
// 0 unless a write was in flight, so `astro config list | head -1` ends
// quietly.
//
// Notify rather than Ignore: an ignored signal is inherited by the children
// a command starts (`astro local run`), which would turn their own broken
// pipes into write errors; a handler is reset to the default across exec, so
// a child still owns its stdout.
func watchClosedStdout(cancel context.CancelCauseFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGPIPE)
	go func() {
		for range sigs {
			// Go delivers SIGPIPE for a broken socket too, which is a failed
			// request and none of stdout's business.
			if stdoutGone.Load().(func() bool)() {
				cancel(cliout.ErrClosedStdout)
			}
		}
	}()
}

// stdoutGone is what decides a SIGPIPE is stdout's; a test replaces it.
var stdoutGone atomic.Value // of func() bool

func init() { stdoutGone.Store(stdoutReaderGone) }

// stdoutReaderGone reports that fd 1 is a pipe with no reader: poll sets
// POLLERR on it on Linux and POLLHUP on macOS.
func stdoutReaderGone() bool {
	fds := []unix.PollFd{{Fd: int32(os.Stdout.Fd()), Events: unix.POLLOUT}}
	n, err := unix.Poll(fds, 0)
	return err == nil && n > 0 && fds[0].Revents&(unix.POLLERR|unix.POLLHUP) != 0
}
