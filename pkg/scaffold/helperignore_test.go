package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// The matcher reads a .gitignore as git does, for the paths it is asked about.
// Checked against git itself where git is on PATH.
func TestGitignoreMatcherAgreesWithGit(t *testing.T) {
	paths := []string{airflowrt.InstallHelperPath, ".astro", "pyproject.toml"}
	for _, gitignore := range []string{
		".astro/\n",
		".astro\n",
		".astro/*\n",
		"*.py\n",
		"/.astro/install-project.py\n",
		"install-project.py\n",
		".astro/*\n!.astro/install-project.py\n",
		"**/.astro/**\n",
		".venv/\n.env\n",
	} {
		pm, err := gitignoreMatcher([]byte(gitignore))
		require.NoError(t, err)
		got := map[string]bool{}
		for _, p := range paths {
			got[p] = matchesGitignore(pm, p)
		}
		git, err := exec.LookPath("git")
		if err != nil {
			continue
		}
		dir := t.TempDir()
		require.NoError(t, exec.Command(git, "-C", dir, "init", "-q").Run())
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0o600))
		// On disk, so git judges .astro as the directory it is, which is how
		// planHelperCommittable asks about it.
		writeTree(t, dir, map[string]string{airflowrt.InstallHelperPath: "", "pyproject.toml": ""})
		for _, p := range paths {
			// check-ignore exits 0 when the path is ignored, 1 when it is not.
			ignored := exec.Command(git, "-C", dir, "check-ignore", "-q", "--no-index", p).Run() == nil
			assert.Equal(t, ignored, got[p], "%q under %q", p, gitignore)
		}
	}
}

func rewriteWithGitignore(t *testing.T, gitignore string) (string, *Result) {
	t.Helper()
	dir, res, _ := initWith(t, map[string]string{
		fileDockerfile:     "FROM quay.io/astronomer/astro-runtime:12.6.0\nENV A=b\n",
		"requirements.txt": "pandas==2.1.0\n",
		fileGitignore:      gitignore,
	})
	return dir, res
}

// A .gitignore rule that ignores the helper gains an exception, so the moved
// Dockerfile's helper can be committed.
func TestConversionKeepsTheHelperCommittable(t *testing.T) {
	dir, _ := rewriteWithGitignore(t, "*.py\n")
	data, err := os.ReadFile(filepath.Join(dir, fileGitignore))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "*.py\n"), "the file's own lines are kept: %q", data)
	assert.Contains(t, string(data), helperIgnoreHeader+"!"+airflowrt.InstallHelperPath+"\n")
	pm, err := gitignoreMatcher(data)
	require.NoError(t, err)
	assert.False(t, matchesGitignore(pm, airflowrt.InstallHelperPath))
	assert.FileExists(t, filepath.Join(dir, airflowrt.InstallHelperPath))
}

// .astro ignored as a whole cannot be excepted from, so the Dockerfile stays
// on its ONBUILD tag, with the lists, and the note names the line to change.
func TestConversionKeepsTheONBUILDTagWhenGitIgnoresTheHelpersDirectory(t *testing.T) {
	dir, res := rewriteWithGitignore(t, ".astro/\n")
	assert.NoFileExists(t, filepath.Join(dir, airflowrt.InstallHelperPath))
	assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
	got, err := os.ReadFile(filepath.Join(dir, fileDockerfile))
	require.NoError(t, err)
	assert.Equal(t, "FROM quay.io/astronomer/astro-runtime:12.6.0\nENV A=b\n", string(got))
	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, ".gitignore ignores .astro/ as a whole")
	assert.Contains(t, notes, "requirements.txt: kept, because your Dockerfile's base image reads it")
}

// A final stage built on an earlier stage that is on a -base tag runs no
// ONBUILD steps: the two lists are read by nothing, so they are retired like
// any carried file, with no note claiming the base reads them.
func TestConversionRetiresListsNoStageReads(t *testing.T) {
	dir, res, _ := initWith(t, map[string]string{
		fileDockerfile:     "FROM quay.io/astronomer/astro-runtime:12.6.0-base AS base\nFROM base\nRUN true\n",
		"requirements.txt": "pandas==2.1.0\n",
	})
	assert.NoFileExists(t, filepath.Join(dir, "requirements.txt"))
	for _, n := range res.Notes {
		assert.NotContains(t, n, "kept, because your Dockerfile's base image reads it")
	}
}
