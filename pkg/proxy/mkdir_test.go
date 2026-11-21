package proxy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The store's parent, ~/.astro, is made owner-only when the store is its first
// writer, and an existing one keeps its mode.
func TestMkdirStoreDirMakesAMissingHomeConfigOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX mode bits")
	}
	home := t.TempDir()
	if err := NewStore(filepath.Join(home, ".astro", "proxy")).mkdirStoreDir(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(home, ".astro")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf(".astro: %v %v", fi, err)
	}

	existing := filepath.Join(t.TempDir(), ".astro")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(filepath.Join(existing, "proxy")).mkdirStoreDir(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(existing); fi.Mode().Perm() != 0o755 {
		t.Fatalf("existing dir changed: %v", fi.Mode())
	}
}

// A caller that names the home config directory is believed over the basename.
func TestMkdirStoreDirUsesTheNamedHomeConfigDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX mode bits")
	}
	home := filepath.Join(t.TempDir(), "relocated")
	if err := NewStore(filepath.Join(home, "proxy"), WithHomeConfigDir(home)).mkdirStoreDir(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(home); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("home: %v %v", fi, err)
	}
}
