//go:build windows

package proxy

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// AcquireLock acquires an exclusive lock on the routes lock file (LockFileEx)
// with timeout. Returns the lock file which must be passed to ReleaseLock.
//
// The same contract as flock on unix: one holder at a time across processes,
// not reentrant across handles, polled against lockTimeout rather than
// blocking, so a holder that never lets go fails a waiter instead of hanging
// it. The lock is on the file's first byte. Windows byte-range locks are
// mandatory, which would matter for a file anyone read or wrote; this one is
// only ever locked.
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
		err := windows.LockFileEx(windows.Handle(f.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, new(windows.Overlapped))
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

// lockPollInterval is how often a waiter retries the lock. Short enough that a
// released lock is picked up promptly, long enough not to spin.
const lockPollInterval = 50 * time.Millisecond

// ReleaseLock releases the lock and closes the lock file.
func ReleaseLock(f *os.File) {
	if f == nil {
		return
	}
	// The close below drops the lock whatever this returns, so a failure here
	// changes nothing a caller could act on.
	windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped)) //nolint:errcheck // deliberate, for the reason above
	f.Close()
}
