//go:build !windows

package proxy

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// AcquireLock acquires an exclusive file lock (flock) with timeout.
// Returns the lock file which must be passed to ReleaseLock.
func AcquireLock() (*os.File, error) {
	if err := os.MkdirAll(RoutesDir(), DirPermRWX); err != nil {
		return nil, fmt.Errorf("error creating proxy directory: %w", err)
	}

	f, err := os.OpenFile(LockFilePath(), os.O_CREATE|os.O_RDWR, FilePermRW)
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

// ReleaseLock releases the flock and closes the lock file.
func ReleaseLock(f *os.File) {
	if f == nil {
		return
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	f.Close()
}
