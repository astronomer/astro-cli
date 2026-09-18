package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	LocalhostSuffix = ".localhost"
	maxLabelLen     = 63
	gitdirPrefix    = "gitdir: "
)

// nonAlphanumRe matches characters that are not lowercase alphanumeric or hyphens.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9-]+`)

// SanitizeLabel lowercases a name, replaces non-alphanumeric characters with
// hyphens, trims leading/trailing hyphens, and truncates to the DNS label max.
func SanitizeLabel(name string) string {
	name = strings.ToLower(name)
	name = nonAlphanumRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > maxLabelLen {
		name = name[:maxLabelLen]
		name = strings.TrimRight(name, "-")
	}
	return name
}

// HostnameIDLen is how much of a project's path hash goes into a hostname:
// as the discriminator that tells two projects of the same name apart, and
// as the whole label when a directory's name has nothing DNS can use.
//
// Six hex characters is 16.7M values against the handful of projects one
// machine runs at once, and short enough that the name is still something to
// type. One constant for both uses, so "how much id goes in a hostname" is a
// decision in one place.
const HostnameIDLen = 6

// DisambiguateHostname folds a discriminator into a hostname's leftmost
// label: analytics.localhost with "a1b2c3" becomes analytics-a1b2c3.localhost,
// and a worktree's wt.repo.localhost becomes wt-a1b2c3.repo.localhost.
//
// The leftmost label is the project's own and the rest says where it lives,
// so that is the one to qualify. The result stays inside the DNS label limit
// by truncating the original label rather than the discriminator, which is
// the part carrying the uniqueness.
//
// An empty discriminator, or a hostname with no label to qualify, is returned
// unchanged: the caller is no worse off than before it asked.
func DisambiguateHostname(hostname, discriminator string) string {
	discriminator = SanitizeLabel(discriminator)
	if discriminator == "" {
		return hostname
	}
	label, rest, found := strings.Cut(hostname, ".")
	if !found {
		return hostname
	}
	// A label is joined to the discriminator with a hyphen, so it must not
	// already end (or begin) with one: --a1b2c3 is not a label DNS will take.
	label = strings.Trim(label, "-")
	if label == "" {
		return hostname
	}
	room := maxLabelLen - len(discriminator) - 1
	if room <= 0 {
		return hostname
	}
	if len(label) > room {
		label = strings.TrimRight(label[:room], "-")
	}
	if label == "" {
		return hostname
	}
	return label + "-" + discriminator + "." + rest
}

// DeriveHostname converts a project directory path into a valid DNS hostname.
//
// If the project is inside a linked git worktree, the hostname includes both
// the worktree name and the repo name — <worktree>.<repo>.localhost — and
// isWorktree is true. Otherwise, it uses just the directory name,
// <dir>.localhost. Callers wanting the worktree fact use the returned flag
// instead of re-reading .git or inspecting the hostname's shape.
//
// The result is a function of the path and nothing else, which is what lets
// callers re-derive it rather than carry it around. It is therefore not
// unique: ~/work/analytics and ~/personal/analytics both derive
// analytics.localhost. Whoever registers a route decides what to do about
// that — see DisambiguateHostname, and localshared.PlanHostname, which picks
// the name a starting project actually answers to.
func DeriveHostname(projectDir string) (hostname string, isWorktree bool, err error) {
	// Try worktree detection first
	if hostname, wErr := deriveWorktreeHostname(projectDir); wErr == nil && hostname != "" {
		return hostname, true, nil
	}

	// Fallback: use directory name
	label := SanitizeLabel(filepath.Base(projectDir))
	if label == "" {
		return "", false, fmt.Errorf("deriving a hostname from project directory %q: no usable label", projectDir)
	}
	return label + LocalhostSuffix, false, nil
}

// ReadDotGit reads the .git file/directory at the given path. It is a variable
// for testing.
var ReadDotGit = func(projectDir string) ([]byte, bool, error) {
	dotGit := filepath.Join(projectDir, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return nil, false, err
	}
	if info.IsDir() {
		return nil, true, nil // .git is a directory → normal repo
	}
	data, err := os.ReadFile(dotGit) //nolint:gosec
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

// deriveWorktreeHostname detects if projectDir is a git worktree by checking
// whether .git is a file (worktrees have a .git file pointing to the main
// repo's .git/worktrees/<name> directory). Returns <worktree>.<repo>.localhost
// or ("", nil) if not a worktree.
func deriveWorktreeHostname(projectDir string) (string, error) {
	data, isDir, err := ReadDotGit(projectDir)
	if err != nil {
		return "", err
	}
	// .git is a directory → normal repo, not a worktree
	if isDir {
		return "", nil
	}

	// .git is a file — this is a worktree
	// Contents: "gitdir: /path/to/main-repo/.git/worktrees/<name>\n"
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, gitdirPrefix) {
		return "", nil
	}

	gitdir := strings.TrimPrefix(line, gitdirPrefix)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(projectDir, gitdir)
	}
	gitdir = filepath.Clean(gitdir)

	// gitdir = main-repo/.git/worktrees/<name>
	// Navigate up: worktrees/<name> → .git → main-repo
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(gitdir)))

	worktreeLabel := SanitizeLabel(filepath.Base(projectDir))
	repoLabel := SanitizeLabel(filepath.Base(repoRoot))

	if worktreeLabel == "" || repoLabel == "" {
		return "", nil
	}

	return worktreeLabel + "." + repoLabel + LocalhostSuffix, nil
}
