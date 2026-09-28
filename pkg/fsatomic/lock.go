package fsatomic

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// LockTimeout is how long Lock waits for a lock another process holds.
//
// Long enough to outlast any honest holder: the writers this serves hold the
// lock for one read, one small edit and one atomic publish. Short enough that a
// holder that has hung, or a filesystem that never grants the lock, is reported
// rather than waited on for the life of the command.
const LockTimeout = 5 * time.Second

// lockPollInterval is how often a waiter asks again. Neither flock nor
// LockFileEx has a timed wait, so waiting is a non-blocking attempt repeated:
// short enough that a released lock is picked up promptly, long enough not to
// spin.
const lockPollInterval = 10 * time.Millisecond

// lockPerm is owner-only: the lock file holds nothing, but it sits beside files that do.
const lockPerm = 0o600

// ErrLockTimeout reports a lock another holder kept for the whole of the wait.
var ErrLockTimeout = errors.New("timed out waiting for a file lock")

// Lock takes an exclusive lock on path, creating the file (0600) if it is not
// there, and waits up to LockTimeout for another holder to let go. The returned
// func releases it; call it exactly once.
//
// For a read-modify-write of a file two processes share. WriteFile alone makes
// each publish atomic, so a reader never sees half a file, but two writers that
// each read, edit and publish can still lose an edit: the second publish is
// built from a read that predates the first. The lock serializes the whole
// sequence. Readers do not need it.
//
// Lock a separate file, never the file being published. WriteFile replaces its
// target with a rename, and a lock on the replaced file guards nothing once the
// new one is in place.
//
// The lock belongs to the open file, not the process (flock on Unix, LockFileEx
// on Windows), so two Lock calls in one process contend exactly as two
// processes do. The lock file is left in place on release: deleting it would
// let a waiter holding the old file and a newcomer that created a new one both
// hold "the" lock.
func Lock(path string) (unlock func(), err error) {
	return lockWithin(path, LockTimeout)
}

// lockWithin is Lock with the wait as a parameter, so a test can time it out
// without spending the full LockTimeout.
func lockWithin(path string, timeout time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, lockPerm)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", path, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		held, err := tryLock(f)
		if err != nil {
			_ = f.Close() // the lock error is the one worth reporting
			return nil, fmt.Errorf("locking %s: %w", path, err)
		}
		if held {
			return func() {
				unlockFile(f)
				_ = f.Close() // closing drops the lock whatever unlock said; nothing to act on
			}, nil
		}
		if !time.Now().Before(deadline) {
			_ = f.Close() // the timeout is the one worth reporting
			return nil, fmt.Errorf("%s is held by another process; gave up after %s: %w", path, timeout, ErrLockTimeout)
		}
		time.Sleep(lockPollInterval)
	}
}
