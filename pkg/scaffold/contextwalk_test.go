package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
}

func walked(t *testing.T, dir, sub, ignore string) []string {
	t.Helper()
	var got []string
	require.NoError(t, WalkContext(dir, sub, ignore, func(rel string, d fs.DirEntry) error {
		if !d.IsDir() {
			got = append(got, filepath.ToSlash(rel))
		}
		return nil
	}))
	return got
}

func TestWalkContextFollowsTheIgnoreRules(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"dags/a.py":          "",
		"plugins/p.py":       "",
		".venv/lib/x.py":     "",
		"include/keep.sql":   "",
		"include/drop.sql":   "",
		"secrets/token":      "",
		"secrets/public.pem": "",
	})
	got := walked(t, dir, "", ".venv\ninclude/drop.sql\nsecrets\n!secrets/public.pem\n")
	assert.Equal(t, []string{"dags/a.py", "include/keep.sql", "plugins/p.py", "secrets/public.pem"}, got)
}

func TestWalkContextOfASubdirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"dags/a.py": "", "dags/sub/b.py": "", "plugins/p.py": ""})
	assert.Equal(t, []string{"dags/a.py", "dags/sub/b.py"}, walked(t, dir, "dags", "dags/sub\n!dags/sub/b.py\n"))
	assert.Empty(t, walked(t, dir, "missing", ""))
}

// An excluded directory is not entered, so one that cannot be read fails
// nothing, unless a "!" rule could match beneath it.
func TestWalkContextDoesNotEnterAnExcludedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{".venv/lib/x.py": "", "dags/a.py": ""})
	locked := filepath.Join(dir, ".venv")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	assert.Equal(t, []string{"dags/a.py"}, walked(t, dir, "", ".venv\n!dags/a.py\n"),
		"a \"!\" rule elsewhere does not open .venv")
}

func TestCouldMatchBeneath(t *testing.T) {
	for _, tc := range []struct {
		pattern, dir string
		want         bool
	}{
		{"secrets/public.pem", "secrets", true},
		{"secrets/public.pem", "other", false},
		{"secrets", "secrets", false},
		{"**/keep", "anything/deep", true},
		{"s*/keep", "secrets", true},
		{"a/b/c", "a/b", true},
		{"a/b/c", "a/x", false},
	} {
		assert.Equal(t, tc.want, couldMatchBeneath(splitSlash(tc.pattern), splitSlash(tc.dir)), "%s under %s", tc.pattern, tc.dir)
	}
}

func splitSlash(s string) []string { return strings.Split(s, "/") }

func TestDagFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"dags/a.py": "", "dags/sub/b.py": "", "dags/readme.md": ""})
	for ignore, want := range map[string]int{
		"":                 2,
		"dags/\n":          0,
		"dags/**\n":        0,
		"**/*.py\n":        0,
		"*\n":              0,
		"dags/sub\n":       1,
		"*\n!dags/a.py\n":  1,
		"dags/readme.md\n": 2,
	} {
		onDisk, shipped, err := DagFiles(dir, ignore)
		require.NoError(t, err)
		assert.Equal(t, 2, onDisk, ignore)
		assert.Equal(t, want, shipped, ignore)
	}

	onDisk, shipped, err := DagFiles(t.TempDir(), "")
	require.NoError(t, err)
	assert.Zero(t, onDisk+shipped, "no dags/ has none")
}

func TestIgnoreFor(t *testing.T) {
	dir := t.TempDir()
	got, err := IgnoreFor(dir, "")
	require.NoError(t, err)
	assert.Empty(t, got)

	writeFiles(t, dir, map[string]string{".dockerignore": "a\n"})
	got, err = IgnoreFor(dir, "Dockerfile")
	require.NoError(t, err)
	assert.Equal(t, "a\n", got)

	writeFiles(t, dir, map[string]string{"Dockerfile.dockerignore": "b\n"})
	got, err = IgnoreFor(dir, "Dockerfile")
	require.NoError(t, err)
	assert.Equal(t, "b\n", got, "a Dockerfile's own ignore file wins, as BuildKit reads it")
}
