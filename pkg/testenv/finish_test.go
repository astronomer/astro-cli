package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// driveIsolate runs isolate with g inside this already-isolated test binary and
// puts the environment back afterward, so a test can call the finish function
// it returns.
func driveIsolate(t *testing.T, g guards) func(code int) int {
	t.Helper()
	return driveIsolateWith(t, g, Options{})
}

func driveIsolateWith(t *testing.T, g guards, o Options) func(code int) int {
	t.Helper()
	saved := os.Environ()
	hooks := afterIsolate
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			k, v, _ := strings.Cut(kv, "=")
			os.Setenv(k, v) //nolint:usetesting // restoring the whole environment
		}
		afterIsolate = hooks
	})
	return isolate(g, o)
}

// changedDecoy is a watch on a decoy home whose config changed after the watch
// was taken, standing in for the developer's real one.
func changedDecoy(t *testing.T) *stateWatch {
	t.Helper()
	decoy := t.TempDir()
	if err := os.MkdirAll(filepath.Join(decoy, ".astro"), 0o700); err != nil {
		t.Fatal(err)
	}
	w := newStateWatch(decoy)
	if err := os.WriteFile(filepath.Join(decoy, ".astro", "config.yaml"), []byte("a: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return w
}

func none() int { return 0 }

// A change to the real config fails a passing package only when CI is set or
// ASTRO_TEST_STRICT_REALCONFIG=1; otherwise it warns, because a real `astro
// login` in another terminal changes it too.
func TestFinishFailsOnARealConfigChangeOnlyWhenStrict(t *testing.T) {
	if got := driveIsolate(t, guards{watch: changedDecoy(t), strict: true, refusals: none})(0); got != 1 {
		t.Errorf("strict, config changed: finish(0) = %d, want 1", got)
	}
	if got := driveIsolate(t, guards{watch: changedDecoy(t), strict: false, refusals: none})(0); got != 0 {
		t.Errorf("not strict, config changed: finish(0) = %d, want 0 (a warning)", got)
	}
	if got := driveIsolate(t, guards{watch: changedDecoy(t), strict: false, refusals: none})(3); got != 3 {
		t.Errorf("not strict: finish(3) = %d, want the tests' own 3", got)
	}
	if got := driveIsolate(t, guards{watch: newStateWatch(t.TempDir()), strict: true, refusals: none})(0); got != 0 {
		t.Errorf("strict, nothing changed: finish(0) = %d, want 0", got)
	}
}

// An OS keychain refusal fails the package whatever the mode, strict or not.
func TestFinishFailsOnAnOSKeychainRefusal(t *testing.T) {
	refused := func() int { return 2 }
	for _, strict := range []bool{false, true} {
		g := guards{watch: newStateWatch(t.TempDir()), strict: strict, refusals: refused}
		if got := driveIsolate(t, g)(0); got != 1 {
			t.Errorf("strict=%v, 2 refusals: finish(0) = %d, want 1", strict, got)
		}
	}
}

// A package that worked a path out from the real home when it loaded repoints
// it in a hook, which runs after HOME has moved.
func TestAfterIsolateHooksSeeTheTemporaryHome(t *testing.T) {
	var sawHome string
	afterIsolate = append(afterIsolate, func() { sawHome = os.Getenv("HOME") })
	driveIsolate(t, guards{watch: newStateWatch(t.TempDir()), refusals: none})
	if sawHome == "" || sawHome != os.Getenv("HOME") {
		t.Errorf("the hook saw HOME=%q, the run's is %q", sawHome, os.Getenv("HOME"))
	}
}

// The shared uv directory is refused when it is not a private directory of
// this user's own: a symlink, or open to others.
func TestPrepareUVDirRefusesWhatItDidNotMake(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes and links here are Unix's")
	}
	t.Run("a symlink", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		if err := os.Symlink(t.TempDir(), filepath.Dir(uvPythonDir())); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareUVDir(); err == nil {
			t.Error("a symlinked parent was accepted")
		}
	})
	t.Run("open to others", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		if err := os.MkdirAll(uvPythonDir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Dir(uvPythonDir()), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareUVDir(); err == nil {
			t.Error("a directory open to others was accepted")
		}
	})
	t.Run("one made here is accepted, twice", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		for range 2 {
			if _, err := prepareUVDir(); err != nil {
				t.Errorf("prepareUVDir: %v", err)
			}
		}
	})
}
