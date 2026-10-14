//go:build windows

package proxy

import "syscall"

// waitTimeout is what WaitForSingleObject returns for a handle that is not
// signaled — for a process handle, one that has not exited.
const waitTimeout = uintptr(0x00000102)

// IsPIDAlive reports whether a process with the given PID is still running.
//
// Answered rather than stubbed, because it decides more than one thing here: a
// Windows host reads the daemon's record with it and the desktop's own proxy
// record too (through LiveRecord), so a fixed false makes a serving proxy read
// as absent. The same answer feeds route pruning, where "dead" evicts a route
// that is in use.
//
// Asked via the process handle's signal state rather than its exit code.
// GetExitCodeProcess is the more obvious route and has a trap: it reports
// STILL_ACTIVE (259) for a running process, and 259 is also a legal exit code,
// so a process that exited with it would read as alive forever.
//
// A handle that cannot be opened is treated as dead. That covers the ordinary
// case — the PID is gone — and also a live process this one may not query,
// which is indistinguishable from here. SYNCHRONIZE is the narrowest right that
// answers the question, which keeps the second case rare.
var IsPIDAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h) //nolint:errcheck // nothing useful to do if the close fails
	event, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		return false
	}
	return uintptr(event) == waitTimeout
}
