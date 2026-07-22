//go:build darwin

package supervise

import (
	"errors"
	"syscall"
)

// waitForProcessExit blocks until the given pid exits. Uses kqueue so the
// wake-up is event-driven (no polling). Falls back to pollProcessExit if
// kqueue setup fails or the pid isn't watchable.
func waitForProcessExit(pid int) {
	kq, err := syscall.Kqueue()
	if err != nil {
		pollProcessExit(pid)
		return
	}
	defer func() { _ = syscall.Close(kq) }()

	ev := syscall.Kevent_t{
		Ident:  uint64(pid), //nolint:gosec // pid is always a positive int from the caller; kevent requires uint64
		Filter: syscall.EVFILT_PROC,
		Flags:  syscall.EV_ADD | syscall.EV_ENABLE | syscall.EV_ONESHOT,
		Fflags: syscall.NOTE_EXIT,
	}
	if _, err := syscall.Kevent(kq, []syscall.Kevent_t{ev}, nil, nil); err != nil {
		pollProcessExit(pid)
		return
	}

	// Race: the pid may have exited between the caller's check and EV_ADD.
	// kill(pid, 0) returns ESRCH if the process no longer exists.
	if err := syscall.Kill(pid, 0); err != nil {
		return
	}

	// A signal landing on this thread wakes Kevent early with EINTR, and no
	// event is delivered. Treating that as "parent exited" would, in
	// session-tied mode, kill Airflow out from under a live session, so keep
	// waiting until the exit event actually arrives — the same EINTR loop the
	// linux pidfd watcher runs. The ONESHOT filter stays armed until it
	// fires, so re-waiting after an interruption needs no re-add.
	events := make([]syscall.Kevent_t, 1)
	for {
		n, err := syscall.Kevent(kq, nil, events, nil)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			// An unexpected error: fall back to polling rather than falsely
			// report the parent gone.
			pollProcessExit(pid)
			return
		}
		if n > 0 {
			// NOTE_EXIT delivered: the parent is gone.
			return
		}
		// A wake with no event: confirm the process is actually gone before
		// returning, else keep waiting.
		if syscall.Kill(pid, 0) != nil {
			return
		}
	}
}
