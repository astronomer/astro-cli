package localrt

// localrt owns where a project's runtime state lives; every consumer (the
// CLI, Astro Desktop) derives the location from the functions here so state
// records never split. The CLI's internal/project and internal/userstate
// delegate to these.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// CanonicalPath returns a path in the one spelling every tool agrees on:
// absolute, with symlinks resolved. Two routes to the same directory (a
// relative path, a symlink, a worktree link) collapse to the same string,
// so anything keyed on it — the project id, the project's .env — never
// forks. It is the single canonicalizer internal/project (through ProjectID)
// and the local env-values feature share, so a value written against one
// spelling of a directory is found against another.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks in %s: %w", abs, err)
	}
	return resolved, nil
}

// ProjectID returns the identity key for a project directory: the sha256
// hex of its symlink-resolved absolute path. Resolving symlinks first means
// every route to the same directory yields the same ID, so per-project
// state never forks. IDs key state; hostnames are display labels only.
func ProjectID(projectPath string) (string, error) {
	resolved, err := CanonicalPath(projectPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:]), nil
}

// CacheRoot returns the astro cache directory: $XDG_CACHE_HOME/astro when
// set, otherwise ~/.cache/astro. Per the XDG spec a relative XDG_CACHE_HOME
// is invalid and ignored.
func CacheRoot() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "astro"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "astro"), nil
}

// StateDir returns the runtime state directory for a project,
// <cache>/projects/<ProjectID>. It does not create the directory.
func StateDir(projectPath string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	id, err := ProjectID(projectPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "projects", id), nil
}
