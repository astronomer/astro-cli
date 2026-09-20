package localrt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// isolatedCache is the cache root this package's tests run against, created by
// TestMain. Exposed so the case below can check the lever actually moved.
var isolatedCache string

// TestMain isolates this package's state, and then checks that it worked.
//
// Two separate jobs, because the lever and the writes are different claims.
//
// The lever: XDG_CACHE_HOME and ASTRO_HOME are pointed at directories of this
// package's own. isolatedLevers already does that per test, which is what
// keeps tests from seeing each other's records; this is the backstop for the
// write that happens BEFORE any helper runs — a record saved in the first line
// of a test. Per-test isolation cannot prevent that one, because it has
// already happened. Which it did: sixty-eight records had collected under a
// developer's ~/.cache/astro/projects for temp directories that no longer
// existed, two more on every run of this package.
//
// The writes: the real cache is listed before the lever moves and again after
// the run, and anything new fails the run. That is the property actually worth
// having, and a lever check is only a proxy for it — a proxy blind to a test
// that repoints the variable itself, and to a code path that resolves state
// without going through rt.CacheRoot at all. It can be fooled the other way
// too: something else on the machine starting a project mid-run reads as a
// leak, which cannot happen on CI and on a laptop means the ids it prints
// belong to whatever was started.
func TestMain(m *testing.M) { os.Exit(runIsolated(m)) }

// runIsolated is TestMain's body, split out so its cleanup can be deferred.
// os.Exit runs no deferred function, so the directories would otherwise
// survive every early return and every panic — the suite leaving state behind,
// one level down from the bug this file is about.
func runIsolated(m *testing.M) int {
	// Resolved before anything moves the lever, so it names the cache a test
	// would reach if the isolation failed.
	ambient, err := rt.CacheRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolving the ambient cache:", err)
		return 1
	}
	before := recordIDs(ambient)

	cache, err := isolatedDir("localrt-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolating the cache:", err)
		return 1
	}
	// Repeated below, where a failure can still reach the exit code; this one
	// is for the early returns and the panics that never get there.
	defer func() { _ = os.RemoveAll(cache) }()

	astroHome, err := isolatedDir("localrt-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolating the astro home:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(astroHome) }()

	// os.Setenv, not t.Setenv: there is no T yet, and both have to be in place
	// before the first test function runs.
	for _, lever := range []struct{ name, dir string }{
		{"XDG_CACHE_HOME", cache},
		{"ASTRO_HOME", astroHome},
	} {
		if err := os.Setenv(lever.name, lever.dir); err != nil {
			fmt.Fprintf(os.Stderr, "isolating %s: %s\n", lever.name, err)
			return 1
		}
	}
	isolatedCache = cache

	code := m.Run()

	if leaked := newSince(before, recordIDs(ambient)); len(leaked) > 0 {
		fmt.Fprintf(os.Stderr,
			"this package wrote %d record(s) into %s, which is not its own cache: %v\n",
			len(leaked), filepath.Join(ambient, "projects"), leaked)
		code = 1
	}
	// Again here, and not only deferred, because a cleanup this package cannot
	// do is the same complaint it exists to make — and stderr from a passing
	// package is not read.
	if err := os.RemoveAll(cache); err != nil {
		fmt.Fprintln(os.Stderr, "removing the isolated cache:", err)
		code = 1
	}
	return code
}

// isolatedDir makes a temp directory and returns it as an absolute path.
//
// Absolute on purpose: rt.CacheRoot ignores a relative XDG_CACHE_HOME and
// falls back to the home directory without saying so, and os.MkdirTemp builds
// on TMPDIR, which a container image is free to set to a relative path. The
// lever would then be a silent no-op and every write would land in the
// developer's real cache — this bug, reintroduced by its own fix.
func isolatedDir(prefix string) (string, error) {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", err
	}
	return filepath.Abs(dir)
}

// recordIDs lists the project ids recorded under a cache root. A root that
// does not exist has none, which is not an error: a machine that has never
// started a project has no such directory.
func recordIDs(cacheRoot string) map[string]bool {
	ids := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "projects"))
	if err != nil {
		return ids
	}
	for _, e := range entries {
		ids[e.Name()] = true
	}
	return ids
}

// newSince returns the ids in after that were not in before.
func newSince(before, after map[string]bool) []string {
	var added []string
	for id := range after {
		if !before[id] {
			added = append(added, id)
		}
	}
	return added
}

// isolatedLevers points this test's state at directories of its own.
//
// One copy of the lever-setting, which is the whole point: there were four,
// and one set neither variable, which is where the leaked records came from.
// Anything that needs a Runtime built differently — a routes directory it
// keeps a handle on, say — calls this and then builds one, rather than writing
// its own pair of Setenvs and drifting from this one.
//
// Two levers, not three. HOME is the third, and it moves the vault, which
// ASTRO_HOME deliberately does not (pkg/secrets/home.go says so). Nothing in
// this package touches the vault, so nothing here moves it — measured rather
// than assumed: probing all three showed writes to the cache alone.
func isolatedLevers(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
}

// isolatedRuntime builds a Runtime whose state goes nowhere but this test's
// own directories. Pass a ProxyDaemon when the test cares what the proxy was
// asked; most do not.
//
// Unconstrained, and deliberately: this used to live beside the tests that
// need a real process group, so every caller inherited //go:build !windows and
// an unconstrained file calling it would not compile for Windows.
func isolatedRuntime(t *testing.T, daemon ...ProxyDaemon) *Runtime {
	t.Helper()
	isolatedLevers(t)
	var d ProxyDaemon
	if len(daemon) > 0 {
		d = daemon[0]
	}
	return New(Config{RoutesDir: t.TempDir(), ProxyDaemon: d})
}

// And the lever points where TestMain put it.
//
// Cheaper and more specific than the write check in TestMain, which speaks
// only at the end of a run and only about records that reached disk. This one
// names the variable, so a TestMain deleted or pointed elsewhere fails here
// rather than as a list of stray ids.
func TestTheSuiteWritesRecordsOnlyUnderItsOwnCache(t *testing.T) {
	require.NotEmpty(t, isolatedCache,
		"TestMain must point XDG_CACHE_HOME at a directory of this package's own")

	// Read, never set. Setting XDG_CACHE_HOME here and then asking where
	// records go is a tautology — it passed against a TestMain that had been
	// mutated to move no lever at all, which is the one thing this exists to
	// catch. Every other test uses t.Setenv, which restores on the way out, so
	// what is in the environment now is what TestMain put there.
	//
	// Compared exactly, not by prefix: MkdirTemp's suffixes vary in length, so
	// a prefix match accepts a sibling whose name merely starts the same way.
	root, err := rt.CacheRoot()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(isolatedCache, "astro"), root,
		"records would be written outside this package's cache")
}
