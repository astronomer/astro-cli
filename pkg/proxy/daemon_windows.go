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
	err = cmd.Start()
	return cmd, err
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
func stopRequests() (stop <-chan struct{}, release func(), err error) {
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

	done := make(chan struct{})
	var once sync.Once
	fire := func() { once.Do(func() { close(done) }) }

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fire()
	}()

	// The handle stays open until release, which Serve defers, rather than
	// closing when the wait returns: the event is how another tool tells this
	// daemon is ours and whether a stop reached it, and that has to stay true
	// through the graceful shutdown a stop starts. A second stop in that
	// window sets it again, which on a manual-reset event changes nothing.
	//
	// Closing a handle while a wait on it is pending is undefined behavior, so
	// release sets the event to end the wait, waits for the waiter to finish,
	// and only then closes it.
	waited := make(chan struct{})
	go func() {
		defer close(waited)
		windows.WaitForSingleObject(ev, windows.INFINITE) //nolint:errcheck // any return ends the wait
		fire()
	}()

	release = func() {
		signal.Stop(sigCh)
		windows.SetEvent(ev) //nolint:errcheck // only wakes the waiter
		<-waited
		windows.CloseHandle(ev) //nolint:errcheck // nothing to do if it fails
	}
	return done, release, nil
}

// stopping holds a handle to each daemon a stop has reached, from requestStop
// until forgetStop.
//
// While a handle to a process is open, Windows does not hand its pid to
// another process. So the wait that follows a stop, and the force if the wait
// runs out, reach the daemon the event was found for and not whatever took its
// pid after it exited: the forced kill goes through this handle, never through
// the pid again.
var (
	stoppingMu sync.Mutex
	stopping   = map[int]windows.Handle{}
)

// requestStop asks the daemon at pid to stop by setting its stop event, and
// reports whether it could. When it could, it keeps a handle to the process
// until forgetStop (see stopping).
//
// False means there is no such event: pid is not a daemon that ever listened
// for one, most likely a process that recycled a stale record's PID, or one in
// another session. The caller then neither waits nor forces it, because forcing
// would end a process that is not ours.
var requestStop = func(pid int) bool {
	// The handle first, so the pid is pinned before the event says whose it is.
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return false
	}
	name, err := windows.UTF16PtrFromString(stopEventName(pid))
	if err != nil {
		windows.CloseHandle(h) //nolint:errcheck // nothing to do if it fails
		return false
	}
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		windows.CloseHandle(h) //nolint:errcheck // nothing to do if it fails
		return false
	}
	defer windows.CloseHandle(ev) //nolint:errcheck // nothing to do if it fails
	if err := windows.SetEvent(ev); err != nil {
		windows.CloseHandle(h) //nolint:errcheck // nothing to do if it fails
		return false
	}

	stoppingMu.Lock()
	defer stoppingMu.Unlock()
	if old, ok := stopping[pid]; ok {
		windows.CloseHandle(old) //nolint:errcheck // replaced by the handle just opened
	}
	stopping[pid] = h
	return true
}

// killProcess ends a daemon a stop reached and that did not exit within
// stopTimeout, through the handle requestStop kept. A pid no stop reached is
// left alone: forcing it by number could end a process that took the pid.
var killProcess = func(pid int) {
	stoppingMu.Lock()
	h, ok := stopping[pid]
	stoppingMu.Unlock()
	if ok {
		windows.TerminateProcess(h, 1) //nolint:errcheck // nothing left to try
	}
}

// forgetStop closes the handle requestStop kept for pid, once the stop is over.
var forgetStop = func(pid int) {
	stoppingMu.Lock()
	defer stoppingMu.Unlock()
	if h, ok := stopping[pid]; ok {
		windows.CloseHandle(h) //nolint:errcheck // nothing to do if it fails
		delete(stopping, pid)
	}
}

// processLooksLikeProxy reports whether pid has a stop event: the identity check
// for when the recorded port does not answer the signature probe in time.
//
// Unix reads the command line with ps instead; Windows has no equivalent short
// of reading another process's memory. The event is the stronger answer anyway:
// only a daemon creates it, under its own pid, and it lasts until Serve
// returns. Without a fallback at all, one probe that failed — a landing page
// that could not read routes.json, a read retried past the probe's timeout —
// would read a live daemon as stale, drop its record, and start a second one
// beside it that nothing could ever stop.
var processLooksLikeProxy = func(_ *Daemon, pid int) bool {
	name, err := windows.UTF16PtrFromString(stopEventName(pid))
	if err != nil {
		return false
	}
	ev, err := windows.OpenEvent(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	windows.CloseHandle(ev) //nolint:errcheck // only asked whether it exists
	return true
}
