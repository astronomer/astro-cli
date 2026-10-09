//go:build windows

package proxy

import (
	"testing"
	"time"
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
