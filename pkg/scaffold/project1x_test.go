package scaffold

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// A 1.x project converts the same way on every platform: DeploysToAPC, which
// once refused it, is read by nothing, so Plan's changeset is the same with
// it set or not, for the project's root and for a directory inside it, and
// Run converts the root.
func TestDeploysToAPCChangesNothing(t *testing.T) {
	root := t.TempDir()
	write1x(t, root)
	for _, dir := range []string{root, filepath.Join(root, "dags"), filepath.Join(root, "fresh")} {
		astro, err := Plan(dir, Options{})
		require.NoError(t, err, dir)
		apc, err := Plan(dir, Options{DeploysToAPC: true})
		require.NoError(t, err, dir)
		assert.Equal(t, astro, apc, dir)
	}

	res, err := Run(root, Options{DeploysToAPC: true})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "pyproject.toml"))
	assert.True(t, slices.ContainsFunc(res.Deleted, func(d string) bool { return strings.HasPrefix(d, "Dockerfile (") }),
		"the pin-only Dockerfile is retired as under Astro: %v", res.Deleted)
}

// Find1xProject names the 1.x project a directory is in, from the root, from
// existing and new directories below it, and stops at an Astro project in
// between, whose own business the 1.x project above it is not.
func TestFind1xProjectNamesTheRoot(t *testing.T) {
	root := t.TempDir()
	write1x(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dags", "inner"), 0o755))
	abs, err := filepath.Abs(root)
	require.NoError(t, err)

	for _, sub := range []string{".", "dags", filepath.Join("dags", "inner"), "fresh"} {
		assert.Equal(t, abs, Find1xProject(filepath.Join(root, sub)), sub)
	}

	_, err = Run(filepath.Join(root, "dags"), Options{})
	require.NoError(t, err)
	assert.Empty(t, Find1xProject(filepath.Join(root, "dags", "inner")), "a project in between stops the walk")
	assert.Empty(t, Find1xProject(t.TempDir()))
}

// A 1.x project is what Astro CLI 1.x itself takes for one: a Dockerfile and
// a .astro/config.yaml, unless that file is the CLI's own settings, as in a
// home directory. So a stray Dockerfile beside settings, or beside a .astro/
// with no config.yaml, does not make home a 1.x project; a config.yaml that
// is empty or does not parse counts, as it does for 1.x, failing closed; and
// a home directory that holds a 1.x project (HOME=/usr/local/airflow in a 1.x
// image) is one, and is found from below. Nothing compares paths.
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
			abs, err := filepath.Abs(home)
			require.NoError(t, err)
			assert.Equal(t, tc.is1x, Find1xProject(filepath.Join(home, "dags")) == abs)
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
	assert.Empty(t, Find1xProject(filepath.Join(root, "sub")))
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
