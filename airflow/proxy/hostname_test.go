package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveHostname(t *testing.T) {
	// Ensure worktree detection doesn't interfere with basic tests
	origReadDotGit := readDotGit
	readDotGit = func(_ string) ([]byte, bool, error) {
		return nil, false, os.ErrNotExist
	}
	defer func() { readDotGit = origReadDotGit }()

	tests := []struct {
		name       string
		projectDir string
		expected   string
	}{
		{
			name:       "simple name",
			projectDir: "/home/user/my-project",
			expected:   "my-project.localhost",
		},
		{
			name:       "uppercase",
			projectDir: "/home/user/My-Project",
			expected:   "my-project.localhost",
		},
		{
			name:       "spaces and special chars",
			projectDir: "/home/user/my project @v2",
			expected:   "my-project-v2.localhost",
		},
		{
			name:       "underscores",
			projectDir: "/home/user/my_project_v2",
			expected:   "my-project-v2.localhost",
		},
		{
			name:       "leading special chars",
			projectDir: "/home/user/---my-project",
			expected:   "my-project.localhost",
		},
		{
			name:       "trailing special chars",
			projectDir: "/home/user/my-project---",
			expected:   "my-project.localhost",
		},
		{
			name:       "dots in name",
			projectDir: "/home/user/my.project.v2",
			expected:   "my-project-v2.localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostname, err := DeriveHostname(tt.projectDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, hostname)
		})
	}
}

func TestDeriveHostname_Empty(t *testing.T) {
	origReadDotGit := readDotGit
	readDotGit = func(_ string) ([]byte, bool, error) {
		return nil, false, os.ErrNotExist
	}
	defer func() { readDotGit = origReadDotGit }()

	// A directory that results in an empty label after cleanup
	_, err := DeriveHostname("/home/user/---")
	assert.Error(t, err)
}

func TestDeriveHostname_LongName(t *testing.T) {
	origReadDotGit := readDotGit
	readDotGit = func(_ string) ([]byte, bool, error) {
		return nil, false, os.ErrNotExist
	}
	defer func() { readDotGit = origReadDotGit }()

	longName := strings.Repeat("a", 100)
	hostname, err := DeriveHostname("/home/user/" + longName)
	require.NoError(t, err)
	// Should be truncated to 63 chars + ".localhost"
	label := strings.TrimSuffix(hostname, ".localhost")
	assert.LessOrEqual(t, len(label), maxLabelLen)
	assert.True(t, strings.HasSuffix(hostname, ".localhost"))
}

func TestDeriveHostname_Worktree(t *testing.T) {
	origReadDotGit := readDotGit
	defer func() { readDotGit = origReadDotGit }()

	// Simulate a git worktree where .git is a file
	readDotGit = func(_ string) ([]byte, bool, error) {
		return []byte("gitdir: /home/user/my-repo/.git/worktrees/feature-branch\n"), false, nil
	}

	hostname, err := DeriveHostname("/home/user/worktrees/feature-branch")
	require.NoError(t, err)
	assert.Equal(t, "feature-branch.my-repo.localhost", hostname)
}

func TestDeriveHostname_WorktreeRelativePath(t *testing.T) {
	origReadDotGit := readDotGit
	defer func() { readDotGit = origReadDotGit }()

	// Some worktrees use relative gitdir paths.
	// For a worktree at /home/user/repos/my-repo/.claude/worktrees/my-worktree,
	// the relative path to main-repo/.git/worktrees/my-worktree is ../../../.git/worktrees/my-worktree
	readDotGit = func(_ string) ([]byte, bool, error) {
		return []byte("gitdir: ../../../.git/worktrees/my-worktree\n"), false, nil
	}

	hostname, err := DeriveHostname("/home/user/repos/my-repo/.claude/worktrees/my-worktree")
	require.NoError(t, err)
	assert.Equal(t, "my-worktree.my-repo.localhost", hostname)
}

func TestDeriveHostname_NormalRepo(t *testing.T) {
	origReadDotGit := readDotGit
	defer func() { readDotGit = origReadDotGit }()

	// Normal repo: .git is a directory
	readDotGit = func(_ string) ([]byte, bool, error) {
		return nil, true, nil // isDir=true
	}

	hostname, err := DeriveHostname("/home/user/my-project")
	require.NoError(t, err)
	assert.Equal(t, "my-project.localhost", hostname)
}

func TestDeriveHostname_RealWorktree(t *testing.T) {
	// Create a real git worktree structure on disk
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main-repo")
	worktreeDir := filepath.Join(dir, "my-worktree")
	worktreesGitDir := filepath.Join(mainRepo, ".git", "worktrees", "my-worktree")

	// Set up main repo .git directory structure
	require.NoError(t, os.MkdirAll(filepath.Join(mainRepo, ".git"), dirPermRWX))
	require.NoError(t, os.MkdirAll(worktreesGitDir, dirPermRWX))

	// Set up worktree directory with .git file
	require.NoError(t, os.MkdirAll(worktreeDir, dirPermRWX))
	gitFileContent := "gitdir: " + worktreesGitDir + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte(gitFileContent), filePermRW))

	hostname, err := DeriveHostname(worktreeDir)
	require.NoError(t, err)
	assert.Equal(t, "my-worktree.main-repo.localhost", hostname)
}
