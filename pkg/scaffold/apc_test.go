package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// write1x lays out a 1.x project in dir: a Dockerfile beside .astro/.
func write1x(t *testing.T, dir string) {
	t.Helper()
	writeTree(t, dir, map[string]string{
		fileDockerfile:       pinOnlyDockerfile,
		".astro/config.yaml": "project:\n  name: orders\n",
		"requirements.txt":   "pandas==2.1.0\n",
	})
}

// writeTree is writeAll for paths that need their directories made.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
}

// tree lists every path under dir with its contents, for asserting that a
// refused run wrote nothing.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		out[path] = string(data)
		return err
	}))
	return out
}

// Under Options.DeploysToAPC, Plan and Run refuse a directory that is itself
// a 1.x project, with the shared message and nothing written; Desktop and
// astro init get the same refusal.
func TestPlanAndRunRefuseA1xProjectUnderAPC(t *testing.T) {
	for name, call := range map[string]func(string, Options) error{
		"Plan": func(dir string, o Options) error { _, err := Plan(dir, o); return err },
		"Run":  func(dir string, o Options) error { _, err := Run(dir, o); return err },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write1x(t, dir)
			before := tree(t, dir)

			err := call(dir, Options{DeploysToAPC: true})
			require.ErrorIs(t, err, ErrConvert1xUnderAPC)
			var refused *Convert1xUnderAPCError
			require.ErrorAs(t, err, &refused)
			abs, _ := filepath.Abs(dir)
			assert.Equal(t, abs, refused.Dir)
			assert.Equal(t, Project1xUnderAPCMessage(abs), err.Error())
			assert.NotContains(t, err.Error(), "`")
			assert.Contains(t, err.Error(), "run astro init in "+abs)
			assert.Equal(t, before, tree(t, dir), "a refused run writes nothing")
		})
	}
}

// Only the directory itself counts: one below a 1.x project, existing or new,
// is converted under APC, leaving the 1.x project's files as they were; and
// without DeploysToAPC the 1.x project converts as it always has.
func TestAPCRefusalIsTheDirectoryAlone(t *testing.T) {
	root := t.TempDir()
	write1x(t, root)
	require.NoError(t, os.Mkdir(filepath.Join(root, "dags"), 0o755))
	before := tree(t, root)

	for _, sub := range []string{"dags", "fresh"} {
		_, err := Run(filepath.Join(root, sub), Options{DeploysToAPC: true})
		require.NoError(t, err, sub)
		assert.FileExists(t, filepath.Join(root, sub, "pyproject.toml"))
	}
	for path, body := range before {
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, body, string(got), path)
	}
	assert.NoFileExists(t, filepath.Join(root, "pyproject.toml"))

	_, err := Run(root, Options{})
	require.NoError(t, err, "under Astro")
	assert.FileExists(t, filepath.Join(root, "pyproject.toml"))
}

func TestIs1xProject(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"a Dockerfile beside .astro/", map[string]string{fileDockerfile: "FROM x\n", ".astro/config.yaml": "x: 1\n"}, true},
		{"with a pyproject.toml that only configures tools", map[string]string{
			fileDockerfile: "FROM x\n", ".astro/config.yaml": "x: 1\n", "pyproject.toml": "[tool.ruff]\nline-length = 100\n",
		}, true},
		{"with a manifest", map[string]string{
			fileDockerfile: "FROM x\n", ".astro/config.yaml": "x: 1\n",
			"pyproject.toml": "[project]\nname = 'x'\nversion = '1.0'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n",
		}, false},
		{"a Dockerfile alone", map[string]string{fileDockerfile: "FROM x\n"}, false},
		{".astro/ alone", map[string]string{".astro/config.yaml": "x: 1\n"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, tc.files)
			assert.Equal(t, tc.want, Is1xProject(dir))
		})
	}
	assert.False(t, Is1xProject(filepath.Join(t.TempDir(), "missing")))
}
