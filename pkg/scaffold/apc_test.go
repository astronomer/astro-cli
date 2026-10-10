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
			assert.Contains(t, err.Error(), "Astro CLI 1.x keeps deploying it to Astro Private Cloud")
			assert.NotContains(t, err.Error(), "astro deploy keeps")
			assert.Equal(t, before, tree(t, dir), "a refused run writes nothing")
		})
	}
}

// A directory inside a 1.x project is refused too, existing or new, naming
// the project: a project scaffolded there would be deployed with it. An
// Astro project in between stops the walk, and without DeploysToAPC the 1.x
// project converts as it always has.
func TestAPCRefusesInsideA1xProject(t *testing.T) {
	root := t.TempDir()
	write1x(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dags", "inner"), 0o755))
	before := tree(t, root)
	abs, err := filepath.Abs(root)
	require.NoError(t, err)

	for _, sub := range []string{"dags", filepath.Join("dags", "inner"), "fresh"} {
		_, err := Run(filepath.Join(root, sub), Options{DeploysToAPC: true})
		var refused *Convert1xUnderAPCError
		require.ErrorAs(t, err, &refused, sub)
		assert.Equal(t, abs, refused.Dir, sub)
	}
	assert.Equal(t, before, tree(t, root), "a refused run writes nothing")

	// An Astro project inside the 1.x one is its own business.
	_, err = Run(filepath.Join(root, "dags"), Options{})
	require.NoError(t, err)
	_, err = Plan(filepath.Join(root, "dags", "inner"), Options{DeploysToAPC: true})
	require.NoError(t, err, "a project in between stops the walk")

	_, err = Run(root, Options{})
	require.NoError(t, err, "under Astro")
	assert.FileExists(t, filepath.Join(root, "pyproject.toml"))
}

// The CLI's own settings file in a home directory's .astro/ is not a 1.x
// project's, so a stray Dockerfile beside it does not make home one; a home
// directory that holds a 1.x project (HOME=/usr/local/airflow in a 1.x image)
// still is one.
func TestTheCLISettingsFileIsNotA1xProject(t *testing.T) {
	const settings = "context: astronomer_io\ncontexts:\n  astronomer_io:\n    domain: astronomer.io\n" +
		"telemetry:\n  enabled: \"false\"\n"
	home := t.TempDir()
	writeTree(t, home, map[string]string{fileDockerfile: "FROM x\n", ".astro/config.yaml": settings})
	assert.False(t, Is1xProject(home))
	_, err := Plan(filepath.Join(home, "work"), Options{DeploysToAPC: true})
	require.NoError(t, err)

	airflowHome := t.TempDir()
	write1x(t, airflowHome)
	assert.True(t, Is1xProject(airflowHome))
	_, err = Plan(filepath.Join(airflowHome, "dags"), Options{DeploysToAPC: true})
	require.ErrorIs(t, err, ErrConvert1xUnderAPC)
}

func TestIs1xProject(t *testing.T) {
	const df = "FROM x\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"a Dockerfile beside .astro/", map[string]string{fileDockerfile: df, ".astro/config.yaml": "project:\n  name: x\n"}, true},
		{"a 1.x config with no project key", map[string]string{fileDockerfile: df, ".astro/config.yaml": "webserver:\n  port: 8081\n"}, true},
		{"a .astro/ with no config.yaml", map[string]string{fileDockerfile: df, ".astro/test_dag_integrity_default.py": "\n"}, true},
		{"a config.yaml that does not parse", map[string]string{fileDockerfile: df, ".astro/config.yaml": "[: nope\n"}, true},
		{"a project key beside contexts is a project's", map[string]string{
			fileDockerfile: df, ".astro/config.yaml": "project:\n  name: x\ncontexts: {}\n",
		}, true},
		{"the CLI's settings file", map[string]string{fileDockerfile: df, ".astro/config.yaml": "contexts: {}\n"}, false},
		{"settings with only telemetry", map[string]string{fileDockerfile: df, ".astro/config.yaml": "telemetry:\n  anonymous_id: x\n"}, false},
		{"with a pyproject.toml that only configures tools", map[string]string{
			fileDockerfile: df, ".astro/config.yaml": "project:\n  name: x\n", "pyproject.toml": "[tool.ruff]\nline-length = 100\n",
		}, true},
		{"with a manifest", map[string]string{
			fileDockerfile: df, ".astro/config.yaml": "project:\n  name: x\n",
			"pyproject.toml": "[project]\nname = 'x'\nversion = '1.0'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n",
		}, false},
		{"a Dockerfile alone", map[string]string{fileDockerfile: df}, false},
		{".astro/ alone", map[string]string{".astro/config.yaml": "project:\n  name: x\n"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, tc.files)
			assert.Equal(t, tc.want, Is1xProject(dir))
		})
	}
	assert.False(t, Is1xProject(filepath.Join(t.TempDir(), "missing")))
}
