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

// DeriveHostname converts a project name and directory into a valid DNS hostname.
//
// projectName comes from .astro/config.yaml (project.name) and is preferred
// over the directory name when set. This matches the help text for
// `astro dev proxy`, which advertises `<project>.localhost` URLs.
//
// If projectName is empty (or sanitizes to empty), the directory name is used
// as a fallback.
//
// When the project lives inside a git worktree, the worktree name is prepended
// for disambiguation: <worktree>.<projectLabel>.localhost.
func DeriveHostname(projectName, projectDir string) (string, error) {
	projectLabel := SanitizeLabel(projectName)

	// Try worktree detection first; projectLabel fills in the project segment
	// when set, otherwise the main repo's directory name is used.
	if hostname, err := deriveWorktreeHostname(projectDir, projectLabel); err == nil && hostname != "" {
		return hostname, nil
	}

	// Non-worktree: prefer project name, fall back to directory name.
	if projectLabel != "" {
		return projectLabel + LocalhostSuffix, nil
	}
	dirLabel := SanitizeLabel(filepath.Base(projectDir))
	if dirLabel == "" {
		return "", fmt.Errorf("could not derive a valid hostname from project directory %q", projectDir)
	}
	return dirLabel + LocalhostSuffix, nil
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
// repo's .git/worktrees/<name> directory). Returns
// <worktree>.<projectLabelOrRepo>.localhost or ("", nil) if not a worktree.
//
// If projectLabel is non-empty it is used as the project segment; otherwise
// the main repo's directory name is used.
func deriveWorktreeHostname(projectDir, projectLabel string) (string, error) {
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
	repoLabel := projectLabel
	if repoLabel == "" {
		repoLabel = SanitizeLabel(filepath.Base(repoRoot))
	}

	if worktreeLabel == "" || repoLabel == "" {
		return "", nil
	}

	return worktreeLabel + "." + repoLabel + LocalhostSuffix, nil
}
