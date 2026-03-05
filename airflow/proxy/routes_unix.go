//go:build !windows

package proxy

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// acquireLock acquires an exclusive file lock (flock) with timeout.
// Returns the lock file which must be passed to releaseLock.
func acquireLock() (*os.File, error) {
	if err := os.MkdirAll(proxyDirPath(), dirPermRWX); err != nil {
		return nil, fmt.Errorf("error creating proxy directory: %w", err)
	}

	f, err := os.OpenFile(lockFilePath(), os.O_CREATE|os.O_RDWR, filePermRW)
	if err != nil {
		return nil, fmt.Errorf("error opening lock file: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timed out waiting for routes lock")
		}
		time.Sleep(50 * time.Millisecond) //nolint:mnd
	}
}

// releaseLock releases the flock and closes the lock file.
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	f.Close()
}

// isPIDAlive checks if a process with the given PID is still running.
var isPIDAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
