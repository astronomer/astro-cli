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
func (s *Store) AcquireLock() (*os.File, error) {
	if err := os.MkdirAll(s.dir, DirPermRWX); err != nil {
		return nil, fmt.Errorf("creating proxy directory: %w", err)
	}

	f, err := os.OpenFile(s.lockFilePath(), os.O_CREATE|os.O_RDWR, FilePermRW)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
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
		time.Sleep(lockPollInterval)
	}
}

// lockPollInterval is how often a waiter retries the flock. Short enough that a
// released lock is picked up promptly, long enough not to spin.
const lockPollInterval = 50 * time.Millisecond

// ReleaseLock releases the flock and closes the lock file.
func ReleaseLock(f *os.File) {
	if f == nil {
		return
	}
	// The close below drops the lock whatever this returns, so a failure here
	// changes nothing a caller could act on.
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck // deliberate, for the reason above
	f.Close()
}
