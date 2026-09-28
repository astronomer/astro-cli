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
// apart; only a component with no exact match is resolved by other means,
// which is precisely the insensitive-filesystem situation.
//
// Those other means end in a question rather than a guess. Comparing names
// closes one equivalence at a time — case was folded in and Unicode
// normalization was not, so `café` typed in a terminal and the same name from
// Finder stayed two project ids for one directory — and there is no list of
// equivalences to finish, because which ones a volume honors is the volume's
// business. So the last resort asks the filesystem which entry this actually
// is, by identity, and takes that entry's name.
//
// Best effort: a directory this process cannot read leaves its component as
// given. Costs one ReadDir per component — about 1ms for a path eight deep,
// against callers that run once or twice per command. The identity scan runs
// only for a component no name matched, and costs one Lstat of that component
// plus, at most, the entry kinds already carried by the listing. On a
// byte-exact filesystem a component that exists always matches by name, so
// only the Lstat is reached, and only for a component that is not there.
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
	if spelled, ok := pickSpelling(names, want); ok {
		return spelled
	}
	return sameEntrySpelling(dir, entries, want)
}

// sameEntrySpelling finds the entry that IS dir/want and returns its name.
//
// The general form of the question pickSpelling answers by comparing text:
// two names a filesystem considers one are one, whether they differ by case,
// by Unicode normalization, or by something this code has never heard of.
//
// "Is" has to be read narrowly, and the narrowing is the whole of this
// function's correctness:
//
// "Is" has to be read narrowly, and the narrowing is the whole of this
// function's correctness.
//
// Lstat on both sides, never Stat. That single choice is what makes a symlink
// unable to win: an alias is a different object that points at the target, so
// comparing objects rather than what they resolve to excludes it. Comparing
// resolved files instead — os.Stat on each entry, which is how this was first
// written — made an `aaa-alias` sorting before the real `café` compare equal
// to it, and its name became the canonical spelling: the wrong id, and a path
// with an unresolved symlink in it after CanonicalPath has promised there are
// none. Measured, and it is what the normalization test's symlink decoy holds.
//
// Two claimants mean no answer. Hard links make two names for one file —
// directories cannot have them, but the leaf of a path need not be a
// directory — and a filesystem that reports identity badly, as SMB and several
// FUSE mounts do by handing out a zero inode for everything, makes every entry
// look like the target. Either way the alphabetically first sibling would
// become the project's identity. Ambiguity is a reason to stop, not to guess.
//
// The kind check and the lstat are independent guards against the same thing,
// and each alone is sufficient: DirEntry.IsDir reports the entry's own type,
// so a symlink to a directory is already excluded by kind before identity is
// consulted. Measured, because it decides what the tests can prove — removing
// either one on its own changes no test, and removing both reproduces the
// alias defect, which the normalization test's symlink decoy then catches.
// The kind check also narrows the degenerate-filesystem case above, where it
// keeps a directory from being answered for by a file.
//
// Returns want when nothing answers — the component is absent, cannot be
// stat'd, or is claimed more than once — because trueCase is best effort and a
// spelling it cannot verify is left as the caller wrote it.
func sameEntrySpelling(dir string, entries []os.DirEntry, want string) string {
	target, err := os.Lstat(filepath.Join(dir, want)) //nolint:gosec // G703: an Lstat to learn a spelling; CanonicalPath's callers pass paths to resolve, some read from git's pointer files
	if err != nil {
		return want
	}
	found := ""
	for _, e := range entries {
		if e.IsDir() != target.IsDir() {
			continue
		}
		// Info is the entry's own lstat, and comes from the listing, so most
		// platforms answer it without another syscall.
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !os.SameFile(target, info) {
			continue
		}
		if found != "" {
			return want
		}
		found = e.Name()
	}
	if found == "" {
		return want
	}
	return found
}

// pickSpelling chooses among the names a directory holds, and reports
// whether any of them matched.
//
// Separate from the ReadDir so the rule can be tested anywhere. The rule
// that matters — an exact match wins over a case-insensitive one — only
// shows itself when a directory holds two names differing just by case,
// which a case-insensitive filesystem cannot represent. Left inside the I/O
// it was unfalsifiable on macOS: removing the exact pass changed no test.
//
// The bool is what lets a miss fall through to the identity scan rather than
// being reported as "spelled exactly as asked", which is the same answer for
// two different situations.
func pickSpelling(names []string, want string) (string, bool) {
	// Exact first, and in its own pass: on a case-sensitive filesystem both
	// spellings can exist, and the one asked for is the one meant.
	for _, n := range names {
		if n == want {
			return want, true
		}
	}
	for _, n := range names {
		if strings.EqualFold(n, want) {
			return n, true
		}
	}
	return want, false
}

// ProjectID returns the identity key for a project directory: the sha256
// hex of its CanonicalPath — absolute, symlinks resolved, and spelled the way
// the filesystem spells it. Every route to the same directory yields the same
// ID, so per-project state never forks. IDs key state; hostnames are display
// labels only.
//
// The ID is a hash of a spelling, so every way of reaching one directory has
// to arrive at one spelling. Symlinks are resolved, case is respelled from the
// directory listing, and anything else a volume treats as the same name —
// Unicode normalization, NFC against NFD, is the one that turns up on APFS —
// is settled by asking the filesystem which entry it is. That last step is
// what closes the family rather than one member at a time; see trueCase.
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

// ProjectHome returns the project a checkout belongs to, as a canonical path:
// the key a link to "this project" is recorded under, so that the link reaches
// every worktree of it.
//
//   - A linked git worktree resolves to its main worktree's root, plus the
//     checkout's offset from its own git toplevel, so a monorepo subdirectory
//     in a worktree maps to the same subdirectory of the main checkout.
//   - Anything else resolves to its own canonical path: a main worktree, a
//     subdirectory of one, a directory outside any repository, and a git
//     submodule, which is a repository in its own right rather than a
//     checkout of its parent.
//
// Worktrees can live anywhere, so this reads git's own pointers rather than
// matching path prefixes. A linked worktree is a .git FILE whose gitdir holds a
// commondir file naming the shared repository directory; the main worktree's
// root is that directory's parent. A submodule also has a .git file, but its
// gitdir (.git/modules/<name>) has no commondir, which is what tells the two
// apart. A worktree whose shared directory is not named .git (a bare
// repository's worktree, or a worktree of a submodule) has no main checkout
// this can name, and resolves to itself. So does any pointer that does not
// resolve: reaching fewer projects is the safe failure for a link.
//
// Only reads files; it does not run git. The error is CanonicalPath's, for a
// directory that cannot be resolved at all.
func ProjectHome(dir string) (string, error) {
	path, err := CanonicalPath(dir)
	if err != nil {
		return "", err
	}
	top, ok := gitToplevel(path)
	if !ok {
		return path, nil
	}
	main, ok := mainWorktreeRoot(top)
	if !ok {
		return path, nil
	}
	rel, err := filepath.Rel(top, path)
	if err != nil {
		return path, nil //nolint:nilerr // top is an ancestor of path by construction; a failure means there is no home to name
	}
	return filepath.Join(main, rel), nil
}

// gitToplevel is the nearest directory at or above path holding a .git entry,
// the way git itself finds a checkout's toplevel.
func gitToplevel(path string) (string, bool) {
	for d := path; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// gitdirPrefix starts the one line of a .git file.
const gitdirPrefix = "gitdir:"

// backLinked reports whether a worktree's admin directory names this .git file
// as its own, the back-link `git worktree add` writes to <gitdir>/gitdir. The
// forward pointer is just a file in the checkout, so without this a hand-made
// .git could borrow another project's home, and with it that project's linked
// globals. Both sides are canonicalized before comparing.
func backLinked(gitdir, dotGit string) bool {
	raw, err := os.ReadFile(filepath.Join(gitdir, "gitdir")) //nolint:gosec // G703: git's own pointer, only parsed
	if err != nil {
		return false
	}
	back := strings.TrimSpace(string(raw))
	if back == "" {
		return false
	}
	if !filepath.IsAbs(back) {
		back = filepath.Join(gitdir, back)
	}
	want, err := CanonicalPath(dotGit)
	if err != nil {
		return false
	}
	got, err := CanonicalPath(back)
	return err == nil && got == want
}

// mainWorktreeRoot is the canonical root of the main worktree when top is a
// linked worktree, and false for anything else.
func mainWorktreeRoot(top string) (string, bool) {
	dotGit := filepath.Join(top, ".git")
	info, err := os.Stat(dotGit)
	if err != nil || info.IsDir() {
		return "", false // a .git directory is a main worktree
	}
	raw, err := os.ReadFile(dotGit)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(line, gitdirPrefix) {
		return "", false
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, gitdirPrefix))
	if gitdir == "" {
		return "", false
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(top, gitdir)
	}
	if !backLinked(gitdir, dotGit) {
		return "", false
	}
	// commondir is what a linked worktree's gitdir has and a submodule's does not.
	common, err := os.ReadFile(filepath.Join(gitdir, "commondir")) //nolint:gosec // G703: a pointer file in the checkout's own repository, only parsed
	if err != nil {
		return "", false
	}
	commonDir := strings.TrimSpace(string(common))
	if commonDir == "" {
		return "", false
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitdir, commonDir)
	}
	commonDir = filepath.Clean(commonDir)
	if filepath.Base(commonDir) != ".git" {
		return "", false // a bare repository, or a submodule's worktree: no main checkout to name
	}
	if info, err := os.Stat(commonDir); err != nil || !info.IsDir() {
		return "", false
	}
	main, err := CanonicalPath(filepath.Dir(commonDir))
	if err != nil {
		return "", false
	}
	return main, true
}
