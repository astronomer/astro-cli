//go:build windows

package proxy

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A second holder waits for the first, across handles, the way flock makes it
// on unix. The old Windows lock was a plain open, which let both in at once.
func TestAcquireLockExcludesASecondHolder(t *testing.T) {
	s := NewStore(t.TempDir())

	first, err := s.AcquireLock()
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan error, 1)
	go func() {
		second, err := s.AcquireLock()
		if err == nil {
			ReleaseLock(second)
		}
		acquired <- err
	}()

	select {
	case err := <-acquired:
		t.Fatalf("a second holder got the lock while the first held it (err %v)", err)
	case <-time.After(4 * lockPollInterval):
	}

	ReleaseLock(first)
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("the second holder failed after the first let go: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second holder never got the lock after the first let go")
	}
}

// Only another holder is worth waiting out. Anything else LockFileEx reports
// fails the lock at once, rather than spinning for lockTimeout and then calling
// it a timeout. (The test above is what pins that a held lock really comes back
// as ERROR_LOCK_VIOLATION: if it came back as anything else, the second holder
// would now fail instead of waiting.)
func TestOnlyAHeldLockIsWaitedOut(t *testing.T) {
	if !isLockContention(windows.ERROR_LOCK_VIOLATION) {
		t.Error("a held lock was not treated as contention")
	}
	for _, err := range []error{windows.ERROR_INVALID_HANDLE, windows.ERROR_NOT_SUPPORTED, windows.ERROR_ACCESS_DENIED} {
		if isLockContention(err) {
			t.Errorf("%v was treated as contention and would be polled for lockTimeout", err)
		}
	}
}
