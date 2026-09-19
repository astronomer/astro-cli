package rt

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
	"strings"
)

// CanonicalPath returns a path in the one spelling every tool agrees on:
// absolute, with symlinks resolved, and each component spelled the way the
// filesystem spells it. Two routes to the same directory (a relative path, a
// symlink, a worktree link, a different capitalization) collapse to the same
// string, so anything keyed on it — the project id, the project's .env —
// never forks. It is the single canonicalizer internal/project (through
// ProjectID) and the local env-values feature share, so a value written
// against one spelling of a directory is found against another.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks in %s: %w", abs, err)
	}
	return trueCase(resolved), nil
}

// trueCase respells path the way the filesystem does.
//
// macOS and Windows are case-insensitive by default, and neither
// filepath.Abs nor filepath.EvalSymlinks touches case — so `cd
// ~/work/analytics` and `cd ~/Work/analytics` reach one directory and
// produced two canonical paths, two project ids, two state records and two
// Airflows for it, with `astro local list` showing it twice and `stop`
// ending one of them. Measured: on darwin the two spellings hashed to
// different ids while stat said they were the same directory.
//
// Component by component, because there is no portable call that asks a
// filesystem how it spells a path. An exact match always wins, so a
// case-SENSITIVE filesystem holding both Analytics and analytics keeps them
// apart; only a component with no exact match is resolved case-insensitively,
// which is precisely the case-insensitive-filesystem situation.
//
// Best effort: a directory this process cannot read leaves its component as
// given. Costs one ReadDir per component — about 1ms for a path eight deep,
// against callers that run once or twice per command.
func trueCase(path string) string {
	vol := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path, vol)
	if !strings.HasPrefix(rest, string(filepath.Separator)) {
		// A relative remainder means this is not a shape the walk
		// understands; Abs above should have prevented it.
		return path
	}

	out := vol + string(filepath.Separator)
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		out = filepath.Join(out, spellingOnDisk(out, part))
	}
	return out
}

// spellingOnDisk returns how dir spells want, or want itself when it cannot
// tell.
func spellingOnDisk(dir, want string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return want
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return pickSpelling(names, want)
}

// pickSpelling chooses among the names a directory holds.
//
// Separate from the ReadDir so the rule can be tested anywhere. The rule
// that matters — an exact match wins over a case-insensitive one — only
// shows itself when a directory holds two names differing just by case,
// which a case-insensitive filesystem cannot represent. Left inside the I/O
// it was unfalsifiable on macOS: removing the exact pass changed no test.
func pickSpelling(names []string, want string) string {
	// Exact first, and in its own pass: on a case-sensitive filesystem both
	// spellings can exist, and the one asked for is the one meant.
	for _, n := range names {
		if n == want {
			return want
		}
	}
	for _, n := range names {
		if strings.EqualFold(n, want) {
			return n
		}
	}
	return want
}

// ProjectID returns the identity key for a project directory: the sha256
// hex of its CanonicalPath — absolute, symlinks resolved, and spelled the way
// the filesystem spells it. Every route to the same directory yields the same
// ID, so per-project state never forks. IDs key state; hostnames are display
// labels only.
//
// Because the ID is a hash of a spelling, each new way of reaching one
// directory has to be folded in here: symlinks were, capitalization now is,
// and Unicode normalization (NFC against NFD on APFS) is not. Keying identity
// on what the filesystem calls the object — dev+inode — would close the
// family rather than one member at a time, at the cost of an on-disk map from
// identity to state directory.
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
