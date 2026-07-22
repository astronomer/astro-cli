//go:build linux

package supervise

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

// waitForProcessExit blocks until the given pid exits. Uses pidfd_open +
// poll so the wake-up is event-driven. Falls back to pollProcessExit if the
// kernel is too old for pidfd (Linux < 5.3). WSL2 and every currently
// supported distro has pidfd.
func waitForProcessExit(pid int) {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		// ESRCH: the process already exited between the caller's check and
		// here.
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		// ENOSYS on pre-5.3 kernels; anything else is unexpected. Either
		// way, polling is a safe fallback.
		pollProcessExit(pid)
		return
	}
	defer func() { _ = unix.Close(fd) }()

	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // pidfd_open returns a non-negative fd that always fits in int32

	for {
		_, err := unix.Poll(fds, -1)
		if err == nil {
			return
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		pollProcessExit(pid)
		return
	}
}
