package fsatomic

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A second Lock on a held file waits, and gets it once the first lets go. Two
// calls in one process contend because the lock belongs to the open file, which
// is what makes this testable without a subprocess (pkg/secrets has the
// cross-process test).
func TestLockWaitsForTheHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}

	const held = 150 * time.Millisecond
	got := make(chan time.Duration, 1)
	start := time.Now()
	go func() {
		u, err := Lock(path)
		if err != nil {
			t.Errorf("second Lock: %v", err)
			got <- 0
			return
		}
		got <- time.Since(start)
		u()
	}()
	time.Sleep(held)
	unlock()

	if waited := <-got; waited < held {
		t.Errorf("second Lock returned after %s, while the first still held it for %s", waited, held)
	}
}

func TestLockTimesOutWithAClearError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.lock")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	const budget = 80 * time.Millisecond
	start := time.Now()
	u, err := lockWithin(path, budget)
	if err == nil {
		u()
		t.Fatal("a held lock was taken a second time")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Errorf("err = %v, want ErrLockTimeout", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %q, want it to name the lock file", err)
	}
	if elapsed := time.Since(start); elapsed < budget || elapsed > budget+time.Second {
		t.Errorf("gave up after %s, want about %s", elapsed, budget)
	}
}

// The property a caller buys the lock for: read-modify-write sequences under it
// do not lose each other's edits. Without the lock this loses most of them,
// because every goroutine reads the same count between publishes.
func TestLockSerializesReadModifyWrite(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "count.lock")
	path := filepath.Join(dir, "count")
	if err := WriteFile(path, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}

	const writers, each = 8, 10
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				unlock, err := Lock(lockPath)
				if err != nil {
					t.Error(err)
					return
				}
				raw, err := ReadFile(path)
				if err == nil {
					n, _ := strconv.Atoi(string(raw)) // the final count checks it
					time.Sleep(time.Millisecond)      // widen the window a lost update needs
					err = WriteFile(path, []byte(strconv.Itoa(n+1)), 0o600)
				}
				unlock()
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), strconv.Itoa(writers*each); got != want {
		t.Errorf("count = %s, want %s: an edit was lost", got, want)
	}
}

// Release really releases: the file can be locked again at once, and it stays
// on disk (deleting it would let two holders each own a different file).
func TestUnlockReleasesAndKeepsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	for range 3 {
		unlock, err := lockWithin(path, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("relock after release: %v", err)
		}
		unlock()
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("lock file gone after release: %v", err)
	}
}

func TestLockReportsAnUnopenablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "x.lock")
	if _, err := Lock(path); err == nil || errors.Is(err, ErrLockTimeout) {
		t.Errorf("err = %v, want an open error rather than a timeout", err)
	}
}
