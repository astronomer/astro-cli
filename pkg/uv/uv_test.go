package uv

// Plain stdlib testing on purpose: pkg/uv is a shared sub-module and keeps
// its dependency list empty (docs/architecture.md), so no testify here.
// Tests seam the uv binary with fake shell scripts, so most skip on Windows.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == windowsGOOS {
		t.Skip("fake uv binaries are shell scripts")
	}
}

// writeFakeUv writes an executable uv stand-in that answers --version with
// the given version and otherwise runs body.
func writeFakeUv(t *testing.T, dir, version, body string) string {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"uv %s (0000000 2026-01-01 test)\"\n  exit 0\nfi\n%s\n", version, body)
	path := filepath.Join(dir, "uv")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func mapGetenv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDiscoverEnvOverrideWins(t *testing.T) {
	skipOnWindows(t)
	override := writeFakeUv(t, t.TempDir(), "9.9.9", "exit 0")
	binDir := t.TempDir()
	writeFakeUv(t, binDir, "9.9.9", "exit 0")
	getenv := mapGetenv(map[string]string{EnvBin: override, "PATH": binDir})

	got, err := discover(getenv, binDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != override {
		t.Errorf("discover() = %q, want the %s override %q", got, EnvBin, override)
	}
}

func TestDiscoverEnvOverrideInvalidErrors(t *testing.T) {
	skipOnWindows(t)
	// A binDir with a perfectly good uv must not rescue a broken override.
	binDir := t.TempDir()
	writeFakeUv(t, binDir, "9.9.9", "exit 0")
	getenv := mapGetenv(map[string]string{EnvBin: filepath.Join(t.TempDir(), "missing")})

	if _, err := discover(getenv, binDir, nil); err == nil {
		t.Fatal("discover() succeeded, want an error for a broken override")
	}
}

func TestDiscoverBinDirBeatsPath(t *testing.T) {
	skipOnWindows(t)
	binDir := t.TempDir()
	fromBinDir := writeFakeUv(t, binDir, "9.9.9", "exit 0")
	pathDir := t.TempDir()
	writeFakeUv(t, pathDir, "9.9.9", "exit 0")

	got, err := discover(mapGetenv(map[string]string{"PATH": pathDir}), binDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != fromBinDir {
		t.Errorf("discover() = %q, want the embedder dir's %q", got, fromBinDir)
	}
}

func TestDiscoverEmptyBinDirFallsThrough(t *testing.T) {
	skipOnWindows(t)
	pathDir := t.TempDir()
	fromPath := writeFakeUv(t, pathDir, "9.9.9", "exit 0")

	got, err := discover(mapGetenv(map[string]string{"PATH": pathDir}), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != fromPath {
		t.Errorf("discover() = %q, want the PATH's %q", got, fromPath)
	}
}

func TestDiscoverExtraDirsAugmentPath(t *testing.T) {
	skipOnWindows(t)
	extraDir := t.TempDir()
	fromExtra := writeFakeUv(t, extraDir, "9.9.9", "exit 0")

	got, err := discover(mapGetenv(map[string]string{"PATH": t.TempDir()}), "", []string{extraDir})
	if err != nil {
		t.Fatal(err)
	}
	if got != fromExtra {
		t.Errorf("discover() = %q, want the extra dir's %q", got, fromExtra)
	}
}

func TestDiscoverNotFound(t *testing.T) {
	_, err := discover(mapGetenv(map[string]string{"PATH": t.TempDir()}), "", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("discover() error = %v, want errors.Is ErrNotFound", err)
	}
}

func TestDiscoverSkipsNonExecutable(t *testing.T) {
	skipOnWindows(t)
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "uv"), []byte("not a program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := discover(mapGetenv(map[string]string{"PATH": pathDir}), "", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("discover() error = %v, want errors.Is ErrNotFound", err)
	}
}

func TestNewRejectsOldUv(t *testing.T) {
	skipOnWindows(t)
	bin := writeFakeUv(t, t.TempDir(), "0.1.0", "exit 0")
	t.Setenv(EnvBin, bin)

	_, err := New(t.Context(), Options{CacheDir: t.TempDir()})
	var verr *VersionError
	if !errors.As(err, &verr) {
		t.Fatalf("New() error = %v, want *VersionError", err)
	}
	if verr.Version != "0.1.0" || verr.Min != MinVersion {
		t.Errorf("VersionError = %+v, want Version 0.1.0 and Min %s", verr, MinVersion)
	}
}

func TestNewAcceptsCurrentUv(t *testing.T) {
	skipOnWindows(t)
	bin := writeFakeUv(t, t.TempDir(), "9.9.9", "exit 0")
	t.Setenv(EnvBin, bin)

	c, err := New(t.Context(), Options{CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if c.Bin() != bin {
		t.Errorf("Bin() = %q, want %q", c.Bin(), bin)
	}
	if c.Version() != "9.9.9" {
		t.Errorf("Version() = %q, want 9.9.9", c.Version())
	}
}

func TestNewRequiresCacheDir(t *testing.T) {
	if _, err := New(t.Context(), Options{}); err == nil {
		t.Fatal("New() succeeded without a CacheDir")
	}
}

func TestMinVersionParses(t *testing.T) {
	if _, ok := parseVersion(MinVersion); !ok {
		t.Fatalf("MinVersion %q does not parse", MinVersion)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.6.0", "0.6.0", 0},
		{"0.5.29", "0.6.0", -1},
		{"0.11.23", "0.6.0", 1},
		{"1.0.0", "0.99.0", 1},
		{"0.6", "0.6.0", 0},
		{"0.9.0rc1", "0.6.0", 1},
		{"garbage", "0.6.0", -1},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
