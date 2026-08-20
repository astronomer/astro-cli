package rt

// Plain stdlib testing on purpose: pkg/localrt is a shared sub-module and
// keeps its dependency list near-empty (docs/v2-architecture.md), so no
// testify here.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const windowsOS = "windows"

func TestCacheRootXDGOverride(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root, err := CacheRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cache, "astro"); root != want {
		t.Errorf("CacheRoot() = %q, want %q", root, want)
	}
}

func TestCacheRootIgnoresRelativeXDG(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("XDG fallback path is unix-shaped")
	}
	t.Setenv("XDG_CACHE_HOME", "relative/cache")
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := CacheRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".cache", "astro"); root != want {
		t.Errorf("CacheRoot() = %q, want %q", root, want)
	}
}

func TestCacheRootDefaultsToHome(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("XDG fallback path is unix-shaped")
	}
	t.Setenv("XDG_CACHE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := CacheRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".cache", "astro"); root != want {
		t.Errorf("CacheRoot() = %q, want %q", root, want)
	}
}

func TestProjectIDSymlinkedPathsHashIdentically(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("symlinks need extra privileges on windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "real-project")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link-project")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	realID, err := ProjectID(target)
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := ProjectID(link)
	if err != nil {
		t.Fatal(err)
	}
	if realID != linkID {
		t.Errorf("ProjectID differs across symlink: %q vs %q", realID, linkID)
	}
	if len(realID) != 64 {
		t.Errorf("ProjectID length = %d, want 64", len(realID))
	}
}

func TestProjectIDDiffersPerDirectory(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}

	aID, err := ProjectID(a)
	if err != nil {
		t.Fatal(err)
	}
	bID, err := ProjectID(b)
	if err != nil {
		t.Fatal(err)
	}
	if aID == bID {
		t.Errorf("ProjectID identical for %q and %q", a, b)
	}
}

func TestProjectIDMissingDirectory(t *testing.T) {
	if _, err := ProjectID(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("ProjectID on a missing directory: want error, got nil")
	}
}

func TestStateDirComposition(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	proj := t.TempDir()

	dir, err := StateDir(proj)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ProjectID(proj)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cache, "astro", "projects", id); dir != want {
		t.Errorf("StateDir() = %q, want %q", dir, want)
	}
}
