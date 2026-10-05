// Package fsatomic reads and writes files that two processes share: a write
// publishes through a temp file and a rename, so a reader sees the old contents
// or the new ones and never half of either.
//
// It is a sub-module rather than root-module internal/ because the core's state
// writers are spread across modules — pkg/localrt's record store, pkg/proxy's
// routes and record files, pkg/secrets' vault, internal/userstate — and every
// one of them is written by both the CLI and Astro Desktop.
//
// That sharing is also why the package owns a read: on Windows a rename cannot
// replace a file another handle has open, and a file being renamed onto cannot
// be opened. Both sides of that are transient, both need the same
// platform-specific knowledge of which errors mean "busy", and keeping that
// knowledge in one place is the point — the inline copies this replaced had
// three different retry policies between them, one of which was no retry at
// all.
package fsatomic

import (
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"
)

// WriteFile writes data to path atomically with the given permissions.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		// CreateTemp creates 0o600; make the published mode explicit.
		werr = os.Chmod(tmpPath, perm)
	}
	if werr == nil {
		werr = replace(tmpPath, path)
	}
	if werr != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup; the write error below is what we return
		return fmt.Errorf("writing %s: %w", path, werr)
	}
	return nil
}

// ReadFile reads path, waiting out a Windows publisher that has it open.
//
// The mirror of WriteFile's problem, and the other half of why this package
// exists. A file being renamed onto cannot be opened at that instant on
// Windows, so a reader arriving during a publish fails for a reason that is
// about neither the file nor the caller — while on Unix it simply cannot
// happen, because rename(2) swaps the directory entry atomically.
//
// A file that is NOT THERE is returned immediately, never waited on: that is
// the ordinary answer when nothing has published yet, and retrying it would
// turn the common case into a delay.
//
// A genuine permission failure is NOT distinguished from contention on Windows,
// and is therefore waited on — see isBusy in busy_windows.go for why, and what
// it costs.
func ReadFile(path string) ([]byte, error) {
	var data []byte
	_, err := retryWhileBusy(replaceBudget, isBusy, func() error {
		var rerr error
		data, rerr = os.ReadFile(path)
		return rerr
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// The bounds on waiting for a destination another process is holding open.
//
// The wait backs off exponentially with jitter rather than sleeping a fixed
// step, which matters more than the size of the budget. Every contender that
// collides retries on the same fixed schedule, so a constant step keeps them in
// lockstep and they collide again — and again — until the attempts run out. That
// is how 20 tries at a flat 5ms managed to exhaust 100ms and fail a write:
// pkg/secrets' concurrency test puts forty writers on one key, and on a loaded
// Windows runner they simply took turns losing together. Jitter breaks the
// convoy up; doubling means the few unlucky writers wait in proportion to the
// contention rather than in proportion to the constant somebody guessed.
//
// The budget is a deadline rather than an attempt count, because what a caller
// cares about is how long it can be made to wait. Two seconds is far longer
// than any real contended window (the window is one rename) and still short
// enough that a genuinely locked file is reported rather than spun on.
//
// The deadline alone is not enough, though, because it is checked after an
// attempt and an attempt is not instant. On a loaded Windows machine a single
// MoveFileEx onto a contended file can block for seconds before reporting it
// busy, and a first attempt that outlives the budget would otherwise return
// with no retry at all ("after 1 attempts"). replaceMinAttempts guarantees a
// few retries after it, which is what such a wait needs: by the time a blocked
// call returns, whatever held the file has usually let go.
const (
	replaceBudget      = 2 * time.Second
	replaceMinWait     = time.Millisecond
	replaceMaxWait     = 50 * time.Millisecond
	replaceMinAttempts = 3
)

// replace renames tmp onto path, retrying while Windows reports the destination
// as busy.
//
// On Unix rename(2) is atomic and never fails because someone else has the
// destination open, so this is a single call there. Windows has no such
// guarantee: MoveFileEx fails with ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION
// when another handle is open on the target, which includes the perfectly
// ordinary case of a second writer replacing the same file at the same moment,
// or a reader that has it open to read.
//
// That is not a theoretical race for these files. This package exists for state
// two processes share — the astro config, the routes file, the vault's per-key
// values — where the CLI and the desktop write concurrently by design. Without
// the retry, one of them simply fails, and pkg/secrets' own concurrency test
// proved it the first time it ran on Windows.
func replace(tmp, path string) error {
	start := time.Now()
	attempts, err := retryWhileBusy(replaceBudget, isBusy, func() error {
		return os.Rename(tmp, path)
	})
	if err != nil && isBusy(err) {
		// Elapsed rather than the budget: the budget is a constant a reader can
		// look up, and the loop always overshoots it by up to one wait. What
		// tells a lock from a race is how long it actually took.
		return fmt.Errorf("destination held open by another process after %d attempts over %s: %w",
			attempts, time.Since(start).Round(time.Millisecond), err)
	}
	return err
}

// retryWhileBusy runs op until it succeeds, fails for a reason other than the
// destination being held open, or runs out of budget, which it cannot do
// before replaceMinAttempts attempts. It reports how many attempts it made.
//
// The one place the policy lives. Both the read and the write need it, and the
// two inline copies this replaced had already drifted — one of them doubled its
// wait past the cap it declared, and the other returned a bare error where the
// first added context.
//
// op and busy are parameters rather than fixed calls so the loop is reachable
// off Windows. isBusy is a constant false on Unix, so a version that closed
// over it directly could never run outside CI's one Windows job — which is how
// the flat backoff this replaced survived being written, and how the same
// mistake nearly shipped again in ReadFile.
func retryWhileBusy(budget time.Duration, busy func(error) bool, op func() error) (attempts int, err error) {
	deadline := time.Now().Add(budget)
	wait := replaceMinWait
	for attempts = 1; ; attempts++ {
		err = op()
		// Anything that is not "somebody has it open" is the caller's answer,
		// and asking again will not change it.
		if err == nil || !busy(err) {
			return attempts, err
		}
		if attempts >= replaceMinAttempts && !time.Now().Before(deadline) {
			return attempts, err
		}
		time.Sleep(jitter(wait))
		wait = min(wait*2, replaceMaxWait)
	}
}

// jitter returns a wait somewhere in the back half of d, so contenders that
// collided do not line up to collide again.
func jitter(d time.Duration) time.Duration {
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1)) //nolint:gosec // spreading retries apart, not generating a secret
}
