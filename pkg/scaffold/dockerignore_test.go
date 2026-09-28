package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wantDockerignoreRules is the list spelled out independently of
// dockerignoreRules, so the assertions below cannot agree with a list that lost
// an entry.
var wantDockerignoreRules = []string{
	".venv/",
	".env",
	".astro/standalone/",
	".astro/worktrees/",
	".astro/*.local.yaml",
	".astro/*.local.yml",
	".astro/otto/*.local.json",
	".astro/otto/mcp.json",
	"plugins/fix_local_executor_pickle.py",
}

// v1Dockerignore is the .dockerignore v1's `astro dev init` wrote, which a
// converted project still carries.
const v1Dockerignore = "astro\n.git\n.env\nairflow_settings.yaml\nlogs/\n.venv\nairflow.db\nairflow.cfg\n"

func writeProjectFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func applyDockerignore(t *testing.T, dir string) *Change {
	t.Helper()
	c, err := planDockerignore(dir, "Dockerfile")
	require.NoError(t, err)
	if c != nil {
		require.NoError(t, c.apply(dir))
	}
	return c
}

func TestDockerignoreRules(t *testing.T) {
	assert.Equal(t, wantDockerignoreRules, dockerignoreRules)
}

func TestPlanDockerignore(t *testing.T) {
	t.Run("no file gets one with every rule", func(t *testing.T) {
		dir := t.TempDir()
		c := applyDockerignore(t, dir)
		require.NotNil(t, c)
		assert.Equal(t, CreateFile, c.Kind)
		assert.Equal(t, []string{".dockerignore"}, c.Labels)
		assert.Equal(t, dockerignoreHeader+strings.Join(wantDockerignoreRules, "\n")+"\n",
			readDockerignoreFile(t, dir))
	})

	t.Run("a v1 file keeps its lines and gains only what it misses", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, ".dockerignore", v1Dockerignore)
		c := applyDockerignore(t, dir)
		require.NotNil(t, c)
		assert.Equal(t, UpdateFile, c.Kind)
		assert.Equal(t, []string{".dockerignore (added the per-machine rules)"}, c.Labels)
		assert.Equal(t, v1Dockerignore+"\n"+dockerignoreHeader+strings.Join(wantDockerignoreRules[2:], "\n")+"\n",
			readDockerignoreFile(t, dir),
			".venv and .env are already excluded, however they are spelled")
	})

	t.Run("a file with no trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, ".dockerignore", "# mine\ndist")
		applyDockerignore(t, dir)
		assert.True(t, strings.HasPrefix(readDockerignoreFile(t, dir), "# mine\ndist\n\n"+dockerignoreHeader))
	})

	t.Run("broader patterns count", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, ".dockerignore", "**/.venv\n.env*\n.astro\nplugins/fix_*.py\n")
		assert.Nil(t, applyDockerignore(t, dir))
	})

	t.Run("a re-included path is added back", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, ".dockerignore", strings.Join(wantDockerignoreRules, "\n")+"\n!.astro/otto/mcp.json\n")
		applyDockerignore(t, dir)
		assert.True(t, strings.HasSuffix(readDockerignoreFile(t, dir), dockerignoreHeader+".astro/otto/mcp.json\n"))
	})

	t.Run("a second run changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, ".dockerignore", v1Dockerignore)
		applyDockerignore(t, dir)
		assert.Nil(t, applyDockerignore(t, dir))
	})

	t.Run("the Dockerfile's own ignore file is the one the build reads", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, "Dockerfile.dockerignore", "dist\n")
		c := applyDockerignore(t, dir)
		require.NotNil(t, c)
		assert.Equal(t, "Dockerfile.dockerignore", c.Path)
		assert.NoFileExists(t, filepath.Join(dir, ".dockerignore"))
	})
}

func readDockerignoreFile(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".dockerignore"))
	require.NoError(t, err)
	return string(data)
}

// localStateProject is a project whose standalone runs and tools have left
// their per-machine files behind, beside the shared ones.
func localStateProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range []string{
		"Dockerfile",
		".venv/bin/python",
		".astro/standalone/airflow.db",
		".astro/config.local.yaml",
		".astro/config.yaml",
		".astro/otto/permissions.json",
		"dags/dag.py",
	} {
		writeProjectFile(t, dir, rel, "")
	}
	return dir
}

func TestLocalFilesWarning(t *testing.T) {
	t.Run("no ignore file names every per-machine file there", func(t *testing.T) {
		dir := localStateProject(t)
		assert.Equal(t, []string{".venv", ".astro/standalone", ".astro/config.local.yaml"}, unignoredLocalFiles(dir, ".dockerignore"))
		assert.Equal(t, "the image built from Dockerfile would copy in these per-machine files: "+
			".venv, .astro/standalone, .astro/config.local.yaml. To keep them out, add them to .dockerignore",
			LocalFilesWarning(dir, "Dockerfile"))
	})

	t.Run("a v1 ignore file leaves the .astro ones", func(t *testing.T) {
		dir := localStateProject(t)
		writeProjectFile(t, dir, ".dockerignore", v1Dockerignore)
		assert.Equal(t, []string{".astro/standalone", ".astro/config.local.yaml"}, unignoredLocalFiles(dir, ".dockerignore"))
	})

	t.Run("one file", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectFile(t, dir, ".astro/standalone/airflow.db", "")
		writeProjectFile(t, dir, "docker/Dockerfile", "")
		assert.Equal(t, "the image built from docker/Dockerfile would copy in these per-machine files: "+
			".astro/standalone. To keep them out, add it to .dockerignore",
			LocalFilesWarning(dir, "docker/Dockerfile"))
	})

	t.Run("silent once init has written the rules", func(t *testing.T) {
		dir := localStateProject(t)
		applyDockerignore(t, dir)
		assert.Empty(t, LocalFilesWarning(dir, "Dockerfile"))
	})

	t.Run("a rule for a directory's contents counts", func(t *testing.T) {
		dir := localStateProject(t)
		writeProjectFile(t, dir, ".dockerignore", ".venv/**\n.astro/standalone/*\n.astro/*.local.yaml\n")
		assert.Empty(t, LocalFilesWarning(dir, "Dockerfile"))
		applyDockerignore(t, dir)
		got := readDockerignoreFile(t, dir)
		assert.NotContains(t, got, "\n.venv/\n", "init appended a directory whose contents were already left out")
		assert.NotContains(t, got, "\n.astro/standalone/\n")
	})

	t.Run("a project path with glob characters", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "proj[1]")
		writeProjectFile(t, dir, ".astro/standalone/airflow.db", "")
		assert.Contains(t, LocalFilesWarning(dir, "Dockerfile"), ": .astro/standalone.")
	})

	t.Run("silent without a declared Dockerfile", func(t *testing.T) {
		assert.Empty(t, LocalFilesWarning(localStateProject(t), ""))
	})

	t.Run("reads the Dockerfile's own ignore file", func(t *testing.T) {
		dir := localStateProject(t)
		applyDockerignore(t, dir)
		writeProjectFile(t, dir, "Dockerfile.dockerignore", "dist\n")
		assert.Contains(t, LocalFilesWarning(dir, "Dockerfile"), "add them to Dockerfile.dockerignore")
	})

	t.Run("an ignore file docker cannot parse is the build's to report", func(t *testing.T) {
		dir := localStateProject(t)
		writeProjectFile(t, dir, ".dockerignore", "[\n")
		assert.Empty(t, LocalFilesWarning(dir, "Dockerfile"))
	})
}

// init writes the rules when it keeps a Dockerfile as the build, and leaves
// .dockerignore alone when the image is generated.
func TestPlanWritesDockerignoreForAKeptDockerfile(t *testing.T) {
	const runtimeRef = "astrocrpublic.azurecr.io/runtime:3.1-12"
	for _, tc := range []struct {
		name, dockerfile string
		want             bool
	}{
		{"kept", "FROM " + runtimeRef + "\nRUN echo hi\n", true},
		{"retired", "FROM " + runtimeRef + "\n", false},
		{"none", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.dockerfile != "" {
				writeProjectFile(t, dir, "Dockerfile", tc.dockerfile)
			}
			writeProjectFile(t, dir, ".dockerignore", v1Dockerignore)

			res, err := Run(dir, Options{})
			require.NoError(t, err)

			if !tc.want {
				assert.Equal(t, v1Dockerignore, readDockerignoreFile(t, dir))
				assert.NotContains(t, res.Updated, ".dockerignore (added the per-machine rules)")
				return
			}
			assert.Contains(t, res.Updated, ".dockerignore (added the per-machine rules)")
			assert.Contains(t, readDockerignoreFile(t, dir), "\n.astro/standalone/\n")
		})
	}
}
