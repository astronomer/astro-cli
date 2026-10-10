package scaffold

import (
	"errors"
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

// A 1.x project is what Astro CLI 1.x itself takes for one: a Dockerfile and
// a .astro/config.yaml, unless that file is the CLI's own settings, as in a
// home directory. So a stray Dockerfile beside settings, or beside a .astro/
// with no config.yaml, does not make home a 1.x project; a config.yaml that
// is empty or does not parse counts, as it does for 1.x, failing closed; and
// a home directory that holds a 1.x project (HOME=/usr/local/airflow in a 1.x
// image) is one, and is refused from below. Nothing compares paths.
func TestTheCLIHomeIsA1xProjectOnlyWhenItSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		is1x         bool
	}{
		{"settings", "context: astronomer_io\ncontexts:\n  astronomer_io:\n    domain: astronomer.io\n", false},
		{"settings with only telemetry", "telemetry:\n  anonymous_id: x\n", false},
		{"no config.yaml", "-", false},
		{"an empty config.yaml", "", true},
		{"an unparseable config.yaml", "context: [unclosed\n", true},
		{"a 1.x project's", "project:\n  name: x\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			files := map[string]string{fileDockerfile: "FROM x\n", ".astro/.keep": ""}
			if tc.config != "-" {
				files[".astro/config.yaml"] = tc.config
			}
			writeTree(t, home, files)
			t.Setenv("HOME", home)
			assert.Equal(t, tc.is1x, Is1xProject(home))
			_, err := Plan(filepath.Join(home, "dags"), Options{DeploysToAPC: true})
			assert.Equal(t, tc.is1x, errors.Is(err, ErrConvert1xUnderAPC), "%v", err)
		})
	}
}

// The walk stops at a directory that has a manifest, loading or not: Plan
// then reports that manifest rather than the 1.x project above it.
func TestTheWalkStopsAtAnyManifest(t *testing.T) {
	root := t.TempDir()
	write1x(t, root)
	writeTree(t, root, map[string]string{"sub/pyproject.toml": "[project]\nname = 'x'\n\n[tool.astro]\nnot-a-key = 1\n"})
	assert.Empty(t, Find1xProject(filepath.Join(root, "sub", "dags")))
	_, err := Plan(filepath.Join(root, "sub"), Options{DeploysToAPC: true})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrConvert1xUnderAPC)
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
		{"a .astro/ with no config.yaml", map[string]string{fileDockerfile: df, ".astro/test_dag_integrity_default.py": "\n"}, false},
		{"a config.yaml that does not parse", map[string]string{fileDockerfile: df, ".astro/config.yaml": "[: nope\n"}, true},
		{"the CLI's settings file", map[string]string{fileDockerfile: df, ".astro/config.yaml": "contexts: {}\n"}, false},
		{"a project key beside contexts is a project's", map[string]string{
			fileDockerfile: df, ".astro/config.yaml": "project:\n  name: x\ncontexts: {}\n",
		}, true},
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
