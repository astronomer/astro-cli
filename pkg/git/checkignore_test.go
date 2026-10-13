package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func needsGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
}

func writeAll(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
}

func TestCheckIgnored(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	writeAll(t, dir, map[string]string{
		".gitignore":       "secret.txt\nbuild/\ntracked.log\n",
		"secret.txt":       "s",
		"build/out.bin":    "b",
		"dags/a.py":        "a",
		"name with space":  "n",
		"tracked.log":      "t",
		"plugins/keep.txt": "k",
	})
	gitIn(t, dir, "add", "-f", "tracked.log")

	got, err := CheckIgnored(dir, []string{"secret.txt", "build/out.bin", "dags/a.py", "name with space", "tracked.log", "plugins/keep.txt"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"secret.txt", "build/out.bin"}, got, "tracked files are not reported")

	got, err = CheckIgnored(dir, []string{"dags/a.py"})
	require.NoError(t, err)
	assert.Nil(t, got, "nothing ignored is an answer")

	got, err = CheckIgnored(dir, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// A path inside a submodule is one git refuses to answer for from the
// superproject; that is an error, not "not ignored".
func TestCheckIgnoredFailsOnAPathInASubmodule(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	sub := filepath.Join(dir, "vendor", "lib")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	gitIn(t, sub, "init", "-q")
	writeAll(t, sub, map[string]string{"x.py": ""})
	gitIn(t, sub, "add", "x.py")
	gitIn(t, sub, "commit", "-q", "-m", "x")
	gitIn(t, dir, "add", "vendor/lib")

	_, err := CheckIgnored(dir, []string{"vendor/lib/x.py"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "submodule")
}

func TestCheckIgnoredWithoutGit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "")
	_, err := CheckIgnored(dir, []string{"x"})
	require.Error(t, err)
}

func TestInWorkTree(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, InWorkTree(dir))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
	sub := filepath.Join(dir, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	assert.True(t, InWorkTree(sub), "a .git above counts")

	file := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(file, ".git"), []byte("gitdir: ../x\n"), 0o600))
	assert.True(t, InWorkTree(file), "a .git file, as a submodule or worktree has, counts")
}
