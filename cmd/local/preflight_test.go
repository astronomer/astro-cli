package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// venvCache points the cache root at a temp directory and returns the
// check-venvs directory under it, since sweepCheckVenvs resolves its own path.
func venvCache(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root, err := localrt.CacheRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "check-venvs")
	if err := os.MkdirAll(dir, cacheDirPerms); err != nil {
		t.Fatal(err)
	}
	return dir
}

// venvAged writes a cache entry aged by hand: a directory named like a real
// key, a marker whose mtime is the last use, and a file standing in for the
// 200 MB a real one holds. A negative age is in the past.
func venvAged(t *testing.T, cacheDir, key string, age time.Duration) string {
	t.Helper()
	dir := venvPartial(t, cacheDir, key, age)
	marker := filepath.Join(dir, venvMarker)
	if err := os.WriteFile(marker, nil, markerPerms); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(age)
	if err := os.Chtimes(marker, when, when); err != nil {
		t.Fatal(err)
	}
	// The directory last, because creating a file inside one sets its mtime to
	// now — which quietly undoes venvPartial's aging and, in a case about the
	// directory-age rule, hides the very thing being tested.
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatal(err)
	}
	return dir
}

// venvPartial writes a markerless entry: what a build that died leaves.
func venvPartial(t *testing.T, cacheDir, key string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(cacheDir, key)
	if err := os.MkdirAll(dir, cacheDirPerms); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte("packages"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(age)
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatal(err)
	}
	return dir
}

func venvExists(t *testing.T, dir string) bool {
	t.Helper()
	_, err := os.Stat(dir)
	return err == nil
}

// The cache collects itself, because nothing else collects it.
//
// Each entry is ~200 MB and the key covers the whole requirement set, so an
// Airflow bump or one added dependency leaves the previous one behind for good.
func TestSweepDropsVenvsNothingHasUsed(t *testing.T) {
	cache := venvCache(t)
	stale := venvAged(t, cache, "af3.1-py3.12-aaaaaaaaaaaa", -venvUnusedFor-time.Hour)
	fresh := venvAged(t, cache, "af3.1-py3.12-bbbbbbbbbbbb", -time.Hour)

	var said []string
	sweepCheckVenvs(func(m string) { said = append(said, m) })

	if venvExists(t, stale) {
		t.Error("a venv unused past the window should be gone")
	}
	if !venvExists(t, fresh) {
		t.Error("a venv used within the window must be kept")
	}
	if len(said) != 1 || !strings.Contains(said[0], "removed 1 check environment(s) unused for 14 days or more") {
		t.Errorf("a sweep that removed something should say so once: %q", said)
	}
}

// A sweep that removes nothing says nothing.
func TestSweepIsSilentWhenItRemovesNothing(t *testing.T) {
	cache := venvCache(t)
	venvAged(t, cache, "af3.1-py3.12-cccccccccccc", -time.Hour)

	sweepCheckVenvs(func(m string) { t.Errorf("nothing was removed, so nothing should be said: %q", m) })
}

// A markerless entry is either a build running now or one that died, and its
// age is what separates them.
func TestSweepTellsABuildInProgressFromAnAbandonedOne(t *testing.T) {
	cache := venvCache(t)
	building := venvPartial(t, cache, "af3.1-py3.12-dddddddddddd", -time.Minute)
	abandoned := venvPartial(t, cache, "af3.1-py3.12-eeeeeeeeeeee", -venvUnusedFor-time.Hour)

	var said []string
	sweepCheckVenvs(func(m string) { said = append(said, m) })

	if !venvExists(t, building) {
		t.Error("a build started minutes ago must not be swept out from under itself")
	}
	if venvExists(t, abandoned) {
		t.Error("a markerless entry nothing has touched in the window is wreckage; " +
			"EnsureVenv only clears one whose exact key comes up again")
	}
	// Reported as what it is. A partial build is not an environment somebody
	// was using, and it does not hold 200 MB.
	if len(said) != 1 || !strings.Contains(said[0], "abandoned partial") {
		t.Errorf("wreckage should be reported as wreckage: %q", said)
	}
}

// The sweep only removes what this code creates.
//
// It runs as a side effect of a check, so anything else that ever sits beside
// the venvs — a lock, an index, a staging directory added later — must survive
// it, however old.
func TestSweepLeavesWhatItDidNotCreate(t *testing.T) {
	cache := venvCache(t)
	old := time.Now().Add(-venvUnusedFor * 10)

	stranger := filepath.Join(cache, "index")
	if err := os.MkdirAll(stranger, cacheDirPerms); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(cache, "notes.json")
	if err := os.WriteFile(loose, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{stranger, loose} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	sweepCheckVenvs(func(string) {})

	if !venvExists(t, stranger) {
		t.Error("a directory that is not a venv key is not this sweep's to delete")
	}
	if !venvExists(t, loose) {
		t.Error("a loose file is not this sweep's to delete")
	}
}

// Leftovers from a delete that did not finish are retried, not leaked.
//
// removeVenv renames before deleting, so a rename that succeeds and a delete
// that does not leaves a *.removing directory. It no longer answers to a cache
// key, so nothing would ever look at it again unless the sweep does.
func TestSweepFinishesADeleteThatDidNotComplete(t *testing.T) {
	cache := venvCache(t)
	leftover := filepath.Join(cache, "af3.1-py3.12-ffffffffffff"+venvRemoving)
	if err := os.MkdirAll(leftover, cacheDirPerms); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leftover, "payload"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var said []string
	sweepCheckVenvs(func(m string) { said = append(said, m) })

	if venvExists(t, leftover) {
		t.Error("a half-deleted leftover should be collected, since nothing else will")
	}
	// Not counted: the entry it came from was reported when it was renamed.
	if len(said) != 0 {
		t.Errorf("finishing an earlier delete is not a new removal: %q", said)
	}
}

// Freeing an entry must free the KEY, whatever happens to the bytes.
//
// A bare RemoveAll makes maximal progress, so on a file that will not unlink it
// can take the marker and leave the rest — which leaves an entry EnsureVenv can
// neither reuse nor clear, failing every later check for that spec. The rename
// is what makes that impossible.
func TestRemoveVenvFreesTheKeyEvenWhenTheBytesWillNotGo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the directory permission this relies on")
	}
	cache := venvCache(t)
	dir := venvAged(t, cache, "af3.1-py3.12-111111111111", -venvUnusedFor-time.Hour)

	// A subdirectory whose contents cannot be unlinked, so the delete half
	// fails while the rename half succeeds.
	stuck := filepath.Join(dir, "lib")
	if err := os.MkdirAll(stuck, cacheDirPerms); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stuck, "held"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stuck, 0o500); err != nil {
		t.Fatal(err)
	}
	// Restore both spellings: a successful rename moves the stuck directory,
	// and t.TempDir's own cleanup cannot delete it while it is unwritable.
	t.Cleanup(func() {
		_ = os.Chmod(stuck, 0o700)
		_ = os.Chmod(filepath.Join(dir+venvRemoving, "lib"), 0o700)
	})

	if !removeVenv(dir) {
		t.Fatal("the key should be reported free once the rename succeeded")
	}
	if venvExists(t, dir) {
		t.Error("the cache key must be free for a clean rebuild, not left half-deleted")
	}
}

// Only a genuinely absent marker gets the age-of-directory rule.
//
// A complete venv's directory mtime is its BUILD time — use only touches the
// marker inside it — so applying that rule to a venv whose marker merely could
// not be read would delete one that was used this morning.
func TestVenvStaleKeepsAnEntryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the directory permission this relies on")
	}
	cache := venvCache(t)
	// Built long ago, used just now: the shape the directory-age rule gets
	// wrong. venvAged leaves the directory at the build time, so only the
	// marker says it is still wanted — and the marker is what cannot be read.
	dir := venvAged(t, cache, "af3.1-py3.12-222222222222", -venvUnusedFor*10)
	now := time.Now()
	if err := os.Chtimes(filepath.Join(dir, venvMarker), now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	stale, built := venvStale(dir, time.Now().Add(-venvUnusedFor))
	if stale {
		t.Error("an entry whose marker cannot be read must be left alone, not judged by the directory's build time")
	}
	if built {
		t.Error("nothing was established about the entry, so it should not be reported as a finished environment")
	}
}

// A venv the run just used survives the sweep that follows it, however long it
// had been sitting before.
//
// This is what replaces carrying a set of live keys around: EnsureVenv stamps
// on reuse, so by the time a run is finished with the cache, everything it
// touched is fresh. It is also the multi-target case — `--target mwaa,composer`
// resolves two different keys, and the first must not be collected on the way
// to the second and rebuilt in the same command.
func TestVenvsUsedInThisRunSurviveTheSweep(t *testing.T) {
	cache := venvCache(t)
	p := &uvProvisioner{cacheDir: cache}
	first := checks.VenvSpec{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6"}}
	second := checks.VenvSpec{Airflow: "3.1.8", Python: "3.12", Reqs: []string{"apache-airflow==3.1.8"}}

	// Both cached, both long unused — two targets' environments after a
	// fortnight away from the project.
	firstDir := venvAged(t, cache, p.key(first), -venvUnusedFor-time.Hour)
	secondDir := venvAged(t, cache, p.key(second), -venvUnusedFor-time.Hour)

	for _, spec := range []checks.VenvSpec{first, second} {
		if _, err := p.EnsureVenv(context.Background(), spec, func(string) {}); err != nil {
			t.Fatalf("a marked venv should be reused without a uv client: %v", err)
		}
	}
	sweepCheckVenvs(func(m string) { t.Errorf("nothing used this run should be removed: %q", m) })

	if !venvExists(t, firstDir) {
		t.Error("the first target's environment was collected on the way to the second")
	}
	if !venvExists(t, secondDir) {
		t.Error("the second target's environment should have survived too")
	}
}

// `astro local check` actually sweeps.
//
// Worth its own case at the command level, because every test above calls
// sweepCheckVenvs directly: with only those, deleting the one call site in
// runCheck leaves the whole package green and the feature silently gone.
func TestCheckSweepsTheVenvCacheWhenTheRunIsDone(t *testing.T) {
	cache := venvCache(t)
	stale := venvAged(t, cache, "af3.1-py3.12-999999999999", -venvUnusedFor-time.Hour)

	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		Dags:  []checks.ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []checks.ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}}
	if err := execute(t, d, "local", "check"); err != nil {
		t.Fatalf("a clean check must still exit zero: %v", err)
	}

	if venvExists(t, stale) {
		t.Error("a finished check should have collected the environment nothing has used")
	}
	if !strings.Contains(out.String(), "check environment(s) unused") {
		t.Errorf("the removal should be reported to the person running it:\n%s", out.String())
	}
}

// And so does a target run, which reaches the cache by a different path.
//
// Two call sites, so two cases: with only the one above, removing the sweep
// from the target path leaves the package green — and the target path is the
// one that resolves several environments in a run, so it is the one the design
// is about.
func TestCheckTargetSweepsTheVenvCacheWhenTheRunIsDone(t *testing.T) {
	cache := venvCache(t)
	stale := venvAged(t, cache, "af3.1-py3.12-888888888888", -venvUnusedFor-time.Hour)

	d, out := targetDeps(t)
	if err := execute(t, d, "local", "check", "--target", "mwaa"); err != nil {
		t.Fatalf("a clean target check must still exit zero: %v", err)
	}

	if venvExists(t, stale) {
		t.Error("a finished target check should have collected the unused environment")
	}
	if !strings.Contains(out.String(), "check environment(s) unused") {
		t.Errorf("the removal should be reported:\n%s", out.String())
	}
}

// Reuse records the use, which is the only thing that keeps a venv somebody
// relies on daily from aging out.
func TestEnsureVenvStampsTheUseOnAHit(t *testing.T) {
	cache := venvCache(t)
	p := &uvProvisioner{cacheDir: cache}
	spec := checks.VenvSpec{Airflow: "3.1", Python: "3.12", Reqs: []string{"apache-airflow==3.1.*"}}

	dir := venvAged(t, cache, p.key(spec), -venvUnusedFor-time.Hour)
	marker := filepath.Join(dir, venvMarker)
	before, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}

	python, err := p.EnsureVenv(context.Background(), spec, func(string) {})
	if err != nil {
		t.Fatalf("a marked venv should be reused without a uv client: %v", err)
	}
	if python != checks.VenvInterpreter(dir) {
		t.Errorf("EnsureVenv returned %q, want the cached interpreter", python)
	}

	after, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().After(before.ModTime()) {
		t.Error("a cache hit must stamp the marker, or daily use still ages out")
	}
}

func TestProvisionerKeyIsStableAndSpecific(t *testing.T) {
	p := &uvProvisioner{cacheDir: "/cache"}
	base := checks.VenvSpec{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "pandas"}}

	k1 := p.key(base)
	k2 := p.key(base)
	if k1 != k2 {
		t.Errorf("same spec must hash to the same key: %q vs %q", k1, k2)
	}
	if !strings.HasPrefix(k1, "af3.0.6-py3.12-") {
		t.Errorf("key should carry a readable version/python prefix: %q", k1)
	}

	// A changed dependency, Airflow, or Python must land in a different key.
	changed := []checks.VenvSpec{
		{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "numpy"}},
		{Airflow: "3.1.8", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "pandas"}},
		{Airflow: "3.0.6", Python: "3.11", Reqs: []string{"apache-airflow==3.0.6", "pandas"}},
		{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "pandas"}, Constraints: []string{"sqlalchemy<2.1"}},
	}
	for _, spec := range changed {
		if p.key(spec) == k1 {
			t.Errorf("a changed spec must change the key: %+v", spec)
		}
	}

	// A constraint is not a requirement: the same line in the other list is a
	// different environment.
	asReq := checks.VenvSpec{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6", "sqlalchemy<2.1"}}
	asConstraint := checks.VenvSpec{Airflow: "3.0.6", Python: "3.12", Reqs: []string{"apache-airflow==3.0.6"}, Constraints: []string{"sqlalchemy<2.1"}}
	if p.key(asReq) == p.key(asConstraint) {
		t.Error("a constraint and a requirement with the same text must not share a key")
	}
}

func TestResolveConstraintsOfflineDegrades(t *testing.T) {
	// A fetch failure returns ErrConstraintsUnavailable without touching uv, so
	// the offline path never needs a binary.
	p := &uvProvisioner{
		fetch: func(context.Context, string) ([]byte, error) {
			return nil, errors.New("dial tcp: no route to host")
		},
	}
	err := p.ResolveConstraints(context.Background(), []string{"pandas"}, "https://example/constraints.txt", "3.12")
	if !errors.Is(err, checks.ErrConstraintsUnavailable) {
		t.Errorf("an unreachable constraints file should degrade to ErrConstraintsUnavailable, got %v", err)
	}
}

// The readable prefix of a cache directory has to be a legal directory name.
//
// It was pasted in verbatim, which held while every caller passed a concrete
// version. A provisioned check passes the manifest's requires-python, so
// ">=3.10,<3.13" would reach a path — and "<" and ">" are reserved on Windows,
// where the create fails with ERROR_INVALID_NAME rather than with anything
// that reads like a version problem.
func TestCacheKeyIsALegalDirectoryName(t *testing.T) {
	p := &uvProvisioner{}
	for _, spec := range []checks.VenvSpec{
		{Airflow: "3.1", Python: ">=3.10,<3.13", Reqs: []string{"apache-airflow==3.1.*"}},
		{Airflow: "2.10", Python: ">=3.10,<3.12", Reqs: []string{"apache-airflow==2.10.*"}},
		{Airflow: "3.1", Python: "3.12", Reqs: []string{"apache-airflow==3.1.*"}},
		{Airflow: "3.1", Python: "", Reqs: nil},
	} {
		key := p.key(spec)
		if strings.ContainsAny(key, `<>:"/\|?*`) {
			t.Errorf("key %q contains a character a Windows path cannot hold", key)
		}
	}

	// And it still separates: two different requests must not collide.
	a := p.key(checks.VenvSpec{Airflow: "2.10", Python: ">=3.10,<3.12", Reqs: []string{"x"}})
	b := p.key(checks.VenvSpec{Airflow: "2.10", Python: ">=3.10,<3.13", Reqs: []string{"x"}})
	if a == b {
		t.Errorf("two different interpreter requests share a cache directory: %q", a)
	}
}
