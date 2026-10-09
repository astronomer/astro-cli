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
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestCheckIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	for name, body := range map[string]string{
		".gitignore":       "secret.txt\nbuild/\ntracked.log\n",
		"secret.txt":       "s",
		"build/out.bin":    "b",
		"dags/a.py":        "a",
		"name with space":  "n",
		"tracked.log":      "t",
		"plugins/keep.txt": "k",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	gitIn(t, dir, "add", "-f", "tracked.log")

	got := CheckIgnored(dir, []string{"secret.txt", "build/out.bin", "dags/a.py", "name with space", "tracked.log", "plugins/keep.txt"})
	assert.ElementsMatch(t, []string{"secret.txt", "build/out.bin"}, got, "tracked files are not reported")

	assert.Nil(t, CheckIgnored(dir, []string{"dags/a.py"}), "nothing ignored")
	assert.Nil(t, CheckIgnored(dir, nil))
}

func TestCheckIgnoredOutsideARepository(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("x\n"), 0o600))
	assert.Nil(t, CheckIgnored(dir, []string{"x"}))
}
