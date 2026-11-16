package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) { Main(m) }

// The isolation itself, as a test binary sees it: nothing here is the
// developer's home.
func TestTheRunIsIsolated(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if !within(home, os.TempDir()) {
		t.Fatalf("HOME = %q, outside the temporary directory", home)
	}
	if got := os.Getenv("ASTRO_HOME"); got != home {
		t.Errorf("ASTRO_HOME = %q, want %q", got, home)
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		if v := os.Getenv(k); !within(v, os.TempDir()) {
			t.Errorf("%s = %q, outside the temporary directory", k, v)
		}
	}
	if runtime.GOOS == goosWindows {
		for _, k := range []string{"USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
			if v := os.Getenv(k); !within(v, os.TempDir()) {
				t.Errorf("%s = %q, outside the temporary directory", k, v)
			}
		}
	}
	if v := os.Getenv("UV_PYTHON_INSTALL_DIR"); !within(v, os.TempDir()) || within(v, home) {
		t.Errorf("UV_PYTHON_INSTALL_DIR = %q, want a shared directory in the temp dir, not under HOME", v)
	}
}

// A helper process the test starts finds the marker and keeps the home.
func TestAHelperInheritsTheParentsHome(t *testing.T) {
	if os.Getenv(inheritedEnv) == "" {
		t.Error("the isolated process did not mark its environment for a helper to inherit")
	}
}

func TestWithin(t *testing.T) {
	tmp := t.TempDir()
	for path, want := range map[string]bool{
		tmp:                                true,
		filepath.Join(tmp, "a", "missing"): true,
		filepath.Dir(tmp):                  false,
		"":                                 false,
	} {
		if got := within(path, tmp); got != want {
			t.Errorf("within(%q, tmp) = %v, want %v", path, got, want)
		}
	}
}

// The guard on the developer's real config, pointed at a decoy.
func TestTheRealConfigWatch(t *testing.T) {
	decoy := t.TempDir()
	cfgDir := filepath.Join(decoy, ".astro")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("a: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cfg, old, old); err != nil {
		t.Fatal(err)
	}

	w := newStateWatch(decoy)
	if got := w.changed(); len(got) != 0 {
		t.Fatalf("nothing happened, but changed = %v", got)
	}

	if err := os.WriteFile(cfg, []byte("a: c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := w.changed(); len(got) != 1 {
		t.Errorf("a rewritten config: changed = %v, want one entry", got)
	}

	w = newStateWatch(decoy)
	if err := os.WriteFile(cfg+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := w.changed(); len(got) != 1 {
		t.Errorf("a created lock file: changed = %v, want one entry", got)
	}
}

// The keyring hooks a caller passes are what Isolate calls: the mock once the
// home is made, the refusal count when the run ends. The Options value is what
// internal/archlint checks a package linking pkg/secrets passes.
func TestOptionsHooksAreCalled(t *testing.T) {
	var mocked int
	o := Options{MockKeyring: func() { mocked++ }, KeyringRefusals: func() int { return 0 }}
	finish := driveIsolateWith(t, guards{watch: newStateWatch(t.TempDir()), refusals: o.KeyringRefusals}, o)
	if mocked != 1 {
		t.Errorf("MockKeyring called %d times, want 1", mocked)
	}
	if got := finish(0); got != 0 {
		t.Errorf("finish(0) = %d", got)
	}
}

// CI is read by the rule pkg/input uses: spaces ignored, only "", "0" and
// "false" are off, in any case.
func TestCIIsSetFollowsTheInputPackagesRule(t *testing.T) {
	for v, want := range map[string]bool{
		"": false, "0": false, "false": false, "FALSE": false, " false ": false, " 0": false,
		"true": true, "1": true, "yes": true, "no": true, "TRUE": true,
	} {
		t.Setenv("CI", v)
		if got := CIIsSet(); got != want {
			t.Errorf("CI=%q: CIIsSet() = %v, want %v", v, got, want)
		}
	}
}
