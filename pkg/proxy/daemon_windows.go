//go:build windows

package proxy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachedFlags start the daemon with no console of its own and outside the
// starting console's process group.
//
// DETACHED_PROCESS rather than CREATE_NO_WINDOW: the daemon writes only to its
// log file, so it needs no console at all, and the two are mutually exclusive
// (CREATE_NO_WINDOW is ignored alongside DETACHED_PROCESS). Without a console
// it is not one of the processes Windows ends when the starting terminal is
// closed. CREATE_NEW_PROCESS_GROUP keeps a Ctrl+C or Ctrl+Break meant for the
// starting process from reaching it.
const detachedFlags = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP

// startDetached starts the command build returns with no console and, where
// allowed, outside the starting process's job object.
//
// The job is the one thing that can still take the daemon down with whatever
// started it: a process in a job with kill-on-close (some terminals and IDEs
// run their children that way) ends with the job, and the daemon is meant to
// outlive its starter. CREATE_BREAKAWAY_FROM_JOB leaves the job, but a job that
// does not permit breakaway refuses the whole CreateProcess with access
// denied. So it is tried first, and the start is retried without it, in which
// case the daemon lives as long as that job does.
func startDetached(build func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := build()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedFlags | windows.CREATE_BREAKAWAY_FROM_JOB}
	err := cmd.Start()
	if err == nil || !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return cmd, err
	}
	cmd = build()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedFlags}
	return cmd, cmd.Start()
}

// stopEventName is the named event a daemon with this pid waits on to stop.
//
// A named event rather than a signal, which Windows does not have between
// unrelated processes, and rather than a shutdown request on the proxy's port,
// which would put a stop button on a port any local process can reach and need
// a secret in the record to guard it. Local\ is the starting user's session,
// where both tools that share the daemon run.
func stopEventName(pid int) string { return fmt.Sprintf(`Local\astro-proxy-stop-%d`, pid) }

// stopRequests returns a channel that is closed when the daemon is asked to
// stop: its stop event set by requestStop, or Ctrl+C (and the console close,
// logoff and shutdown events Go delivers as SIGTERM) when it runs in a
// terminal. The func it returns stops listening.
//
// The event is created before Serve writes the port file, since a record can
// name this process from then on and a stop has to have somewhere to land.
func stopRequests() (<-chan struct{}, func(), error) {
	name, err := windows.UTF16PtrFromString(stopEventName(os.Getpid()))
	if err != nil {
		return nil, nil, err
	}
	// Manual reset, so a set that lands before the wait below starts is not
	// lost. CreateEvent also answers ERROR_ALREADY_EXISTS with a usable handle;
	// only a zero handle is a failure.
	ev, err := windows.CreateEvent(nil, 1, 0, name)
	if ev == 0 {
		return nil, nil, fmt.Errorf("creating the stop event: %w", err)
	}

	stop := make(chan struct{})
	var once sync.Once
	fire := func() { once.Do(func() { close(stop) }) }

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fire()
	}()

	// The waiter owns the handle and closes it once the wait returns. Closing
	// it from release while the wait is pending is undefined behavior, so
	// release wakes the waiter by setting the event instead, and the mutex
	// keeps that set from landing on a handle the waiter has already closed.
	var mu sync.Mutex
	closed := false
	go func() {
		windows.WaitForSingleObject(ev, windows.INFINITE) //nolint:errcheck // any return ends the wait
		fire()
		mu.Lock()
		closed = true
		windows.CloseHandle(ev) //nolint:errcheck // nothing to do if it fails
		mu.Unlock()
	}()

	release := func() {
		signal.Stop(sigCh)
		mu.Lock()
		if !closed {
			windows.SetEvent(ev) //nolint:errcheck // only wakes the waiter
		}
		mu.Unlock()
	}
	return stop, release, nil
}

// requestStop has nothing to ask on Windows yet.
var requestStop = func(int) bool { return false }

// killProcess has nothing to end on Windows yet.
var killProcess = func(int) {}

// processLooksLikeProxy never matches on Windows. The unix fallback reads the
// command line with ps, which Windows has no equivalent of short of reading
// another process's memory. So a live PID whose recorded port does not answer
// with the proxy's signature is treated as not the daemon, which is the same
// verdict unix reaches when the command line does not match either.
var processLooksLikeProxy = func(*Daemon, int) bool { return false }
