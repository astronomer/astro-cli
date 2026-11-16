package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	finish := Isolate()
	os.Exit(finish(m.Run()))
}

// The isolation itself, as a test binary sees it: nothing here is the
// developer's home, and the keyring is in memory.
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
	if runtime.GOOS == "windows" {
		for _, k := range []string{"USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
			if v := os.Getenv(k); !within(v, os.TempDir()) {
				t.Errorf("%s = %q, outside the temporary directory", k, v)
			}
		}
	}
	if v := os.Getenv("UV_PYTHON_INSTALL_DIR"); !within(v, os.TempDir()) || within(v, home) {
		t.Errorf("UV_PYTHON_INSTALL_DIR = %q, want a shared directory in the temp dir, not under HOME", v)
	}

	// The keyring is the in-memory one: a value set here reads back, and the
	// OS keychain was never asked.
	if err := keyring.Set("astro-testenv", "probe", "value"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
	if got, err := keyring.Get("astro-testenv", "probe"); err != nil || got != "value" {
		t.Errorf("keyring.Get = %q, %v", got, err)
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
