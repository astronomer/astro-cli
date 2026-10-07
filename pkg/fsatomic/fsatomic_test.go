package fsatomic

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// errBusy stands in for what Windows returns when a handle is open on the
// destination. The tests pass their own predicate, so the value only has to be
// distinguishable.
var errBusy = errors.New("destination is busy")

func busyIs(err error) bool { return errors.Is(err, errBusy) }

// opOf adapts a two-argument rename to the op retryWhileBusy runs.
func opOf(rename func(string, string) error) func() error {
	return func() error { return rename("tmp", "dst") }
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")

	if err := WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("WriteFile over an existing file: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("content = %q, want the second write", got)
	}
	// The published mode is explicit rather than CreateTemp's, and the temp
	// file does not survive.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	assertOnlyFile(t, dir, "value.json")
}

// A write that fails must not leave its temp file behind: these directories are
// the user's vault and config, and a litter of dotfiles there is ours.
func TestWriteFileLeavesNoTempBehindOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory where the file should be makes the rename fail with something
	// that is not "busy", so it returns rather than retrying.
	path := filepath.Join(dir, "value.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("x"), 0o600); err == nil {
		t.Fatal("writing over a directory should fail")
	}
	assertOnlyFile(t, dir, "value.json")
}

func TestReplaceRetriesWhileBusy(t *testing.T) {
	for _, busyTimes := range []int{1, 3, 12} {
		attempts := 0
		rename := func(string, string) error {
			attempts++
			if attempts <= busyTimes {
				return errBusy
			}
			return nil
		}
		if _, err := retryWhileBusy(2*time.Second, busyIs, opOf(rename)); err != nil {
			t.Errorf("busy %d times: %v", busyTimes, err)
		}
		if attempts != busyTimes+1 {
			t.Errorf("busy %d times: %d attempts, want %d", busyTimes, attempts, busyTimes+1)
		}
	}
}

// The budget has to outlast a real contended window, not a guessed one.
//
// This is the regression guard for the failure that prompted the change: the
// old bound was 20 attempts at a flat 5ms, so it gave up after 100ms, and forty
// writers on one key on a loaded Windows runner held the destination longer
// than that. A destination busy for 150ms is well inside what contention
// produces and well inside the budget now.
//
// Asserted on the outcome rather than on a duration, so there is no margin to
// race: either the write completed or it did not.
func TestReplaceOutlastsARealisticContendedWindow(t *testing.T) {
	const busyFor = 150 * time.Millisecond
	start := time.Now()
	rename := func(string, string) error {
		if time.Since(start) < busyFor {
			return errBusy
		}
		return nil
	}
	if _, err := retryWhileBusy(replaceBudget, busyIs, opOf(rename)); err != nil {
		t.Errorf("a destination busy for %s should still be replaced: %v", busyFor, err)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record")
	if err := WriteFile(path, []byte("1234 v2 6563"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "1234 v2 6563" {
		t.Errorf("content = %q", got)
	}

	// A file that is not there is the ordinary answer when nothing has
	// published, so it comes back at once rather than after the budget.
	start := time.Now()
	_, err = ReadFile(filepath.Join(dir, "absent"))
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %s on a missing file; it should return at once", elapsed)
	}
}

// Only "somebody has it open" is worth waiting for. A missing directory or a
// read-only volume does not improve with time, and retrying it for the whole
// budget turns an immediate answer into a two-second hang.
func TestReplaceReturnsANonBusyErrorAtOnce(t *testing.T) {
	want := errors.New("read-only file system")
	attempts := 0
	rename := func(string, string) error {
		attempts++
		return want
	}
	_, err := retryWhileBusy(2*time.Second, busyIs, opOf(rename))
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Errorf("%d attempts, want 1 — a non-busy error must not be retried", attempts)
	}
}

// Replace runs the caller's rename (here an os.Root's) and hands back what
// it returned: the published file on success, the rename's own error on a
// failure that is not contention.
func TestReplaceRunsTheCallersRename(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(dir, "tmp"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(func() error { return root.Rename("tmp", "file") }); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "file")); err != nil || string(got) != "new" {
		t.Errorf("file holds %q (%v)", got, err)
	}
	want := errors.New("read-only file system")
	if err := Replace(func() error { return want }); !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

// A destination held open for good is reported, not spun on forever, and the
// message says how hard we tried so the reader can tell a lock from a race.
func TestReplaceGivesUpWithinItsBudget(t *testing.T) {
	attempts := 0
	rename := func(string, string) error {
		attempts++
		return errBusy
	}
	const budget = 60 * time.Millisecond
	start := time.Now()
	attemptsMade, err := retryWhileBusy(budget, busyIs, opOf(rename))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a permanently busy destination must fail")
	}
	if !errors.Is(err, errBusy) {
		t.Errorf("err = %v, want it to wrap the busy error", err)
	}
	// Generous upper bound: the last sleep can overshoot the deadline, and a
	// loaded runner overshoots more. The point is that it is bounded at all.
	// Tight enough to tell the budget PARAMETER from replaceBudget: a version
	// that ignored the argument and used the 2s constant would take far longer
	// than this and the assertion would have nothing to say.
	if elapsed > budget+500*time.Millisecond {
		t.Errorf("took %s, want it bounded near %s", elapsed, budget)
	}
	if attemptsMade < 2 {
		t.Errorf("%d attempts, want it to have retried before giving up", attemptsMade)
	}
}

// An attempt that blocks past the whole budget still gets retried. The deadline
// is checked after each attempt, so without a floor on attempts a first call
// slower than the budget reports busy with no retry, which is how a Windows
// rename that blocked for seconds on a contended file failed a write the next
// attempt would have completed.
func TestReplaceRetriesAfterAnAttemptOutlivesTheBudget(t *testing.T) {
	const budget = 20 * time.Millisecond
	attempts := 0
	made, err := retryWhileBusy(budget, busyIs, func() error {
		attempts++
		if attempts == 1 {
			time.Sleep(2 * budget)
			return errBusy
		}
		return nil
	})
	if err != nil {
		t.Fatalf("a slow first attempt must still be retried: %v", err)
	}
	if made != 2 {
		t.Errorf("%d attempts, want 2", made)
	}
}

// The backoff has to GROW, which is the half of this change the budget does not
// cover — and the half no test caught until one was written for it. Reverting
// the doubling to a flat sleep left every other test green, because a 2s budget
// passes the contended-window case on its own.
//
// Attempt count is what separates the two, and by a wide margin rather than a
// close one: over a 200ms busy window, doubling from 1ms reaches the 50ms cap in
// seven steps and makes roughly a dozen attempts, while a flat 1ms makes about
// two hundred. Anything under fifty can only be a growing wait.
func TestRetryBacksOffExponentiallyNotFlat(t *testing.T) {
	const busyFor = 200 * time.Millisecond
	start := time.Now()
	attempts, err := retryWhileBusy(replaceBudget, busyIs, func() error {
		if time.Since(start) < busyFor {
			return errBusy
		}
		return nil
	})
	if err != nil {
		t.Fatalf("should have succeeded once the window passed: %v", err)
	}
	if attempts > 50 {
		t.Errorf("%d attempts over %s — that is a flat wait, not a growing one", attempts, busyFor)
	}
	// And it did have to wait, so the count means something.
	if attempts < 2 {
		t.Errorf("%d attempts, expected the window to force a retry", attempts)
	}
}

// The declared cap has to be the real one. `if wait < max { wait *= 2 }` lets
// the last doubling overshoot — 32ms is under a 50ms cap, so the next wait is
// 64ms and stays there — which makes the constant a lie about the schedule.
func TestWaitCapIsNotOvershot(t *testing.T) {
	wait := replaceMinWait
	for range 20 {
		wait = min(wait*2, replaceMaxWait)
		if wait > replaceMaxWait {
			t.Fatalf("wait reached %s, above the declared cap of %s", wait, replaceMaxWait)
		}
	}
	if wait != replaceMaxWait {
		t.Errorf("wait settled at %s, want it to reach the cap of %s", wait, replaceMaxWait)
	}
}

func TestJitterBacksOffWithinTheBackHalf(t *testing.T) {
	for _, d := range []time.Duration{time.Millisecond, 8 * time.Millisecond, replaceMaxWait} {
		for range 200 {
			got := jitter(d)
			if got < d/2 || got > d {
				t.Fatalf("jitter(%s) = %s, want it within [%s, %s]", d, got, d/2, d)
			}
		}
	}
}

// The property the vault actually needs, on whatever OS this runs: many writers
// replacing one path all succeed, and the survivor is one of the values written
// rather than a mixture. On Windows this is the case that was failing.
func TestConcurrentWriteFileToOneTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.json")
	if err := WriteFile(path, []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}

	const writers = 40
	var wg sync.WaitGroup
	errs := make([]error, writers)
	values := make([]string, writers)
	for i := range writers {
		values[i] = "value-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = WriteFile(path, []byte(values[i]), 0o600)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d: %v", i, err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Atomic means the file holds one whole value, never a splice of two.
	found := false
	for _, v := range values {
		if string(got) == v {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("content = %q, which is none of the values written", got)
	}
	assertOnlyFile(t, dir, "shared.json")
}

// assertOnlyFile checks the directory holds exactly the named entry, so a
// leaked temp file is a failure rather than something nobody looks at.
func assertOnlyFile(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		t.Errorf("directory holds %v, want only %q", got, name)
	}
}
