package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/uv"
)

const windowsOS = "windows"

func TestRunFreshScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "flight-data")
	res, err := Run(dir, Options{})
	require.NoError(t, err)

	assert.Equal(t, dir, res.Dir)
	assert.Equal(t, "flight-data", res.Name)
	assert.Equal(t, DefaultAirflowVersion, res.AirflowVersion)
	assert.Empty(t, res.Skipped)

	for _, d := range []string{"dags", "include", "plugins", "tests"} {
		info, err := os.Stat(filepath.Join(dir, d))
		require.NoError(t, err, d)
		assert.True(t, info.IsDir(), d)
		if runtime.GOOS != windowsOS {
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), d)
		}
	}
	for _, f := range []string{"pyproject.toml", ".gitignore", "AGENTS.md"} {
		info, err := os.Stat(filepath.Join(dir, f))
		require.NoError(t, err, f)
		if runtime.GOOS != windowsOS {
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), f)
		}
	}

	// The scaffolded manifest must load and carry the derived values.
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "flight-data", m.Project.Name)
	assert.Equal(t, DefaultAirflowVersion, m.Astro.AirflowVersion)
	// [project.dependencies] must install the pinned Airflow, so init → start
	// works with no hand-edit. The default pin is partial, so the
	// requirement is a prefix match.
	assert.Equal(t, []string{"apache-airflow==3.1.*"}, m.Project.Dependencies)
	// A greenfield project declares no OS packages: the manifest carries no
	// packages key at all, not an empty list.
	assert.Nil(t, m.Astro.Packages)
	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "packages")
}

// TestScaffoldedProjectLocksWithRealUv scaffolds a project and runs a real
// `uv lock` against it, proving init produces a manifest uv can actually
// resolve — the end-to-end gap an earlier fix closed. It needs uv and network, so it
// skips under -short or when uv is absent (as CI is).
func TestScaffoldedProjectLocksWithRealUv(t *testing.T) {
	if testing.Short() {
		t.Skip("real uv lock reaches the network")
	}
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not on PATH")
	}

	dir := filepath.Join(t.TempDir(), "locktest")
	_, err := Run(dir, Options{})
	require.NoError(t, err)

	client, err := uv.New(t.Context(), uv.Options{CacheDir: t.TempDir()})
	require.NoError(t, err)
	require.NoError(t, client.Lock(t.Context(), dir, uv.Stdio{}),
		"a freshly scaffolded project must uv lock cleanly")

	_, err = os.Stat(filepath.Join(dir, "uv.lock"))
	require.NoError(t, err, "uv lock must write a lockfile")
}

func TestRunHonorsNameAndAirflowVersion(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(dir, Options{Name: "etl", AirflowVersion: "3.0.2"})
	require.NoError(t, err)
	assert.Equal(t, "etl", res.Name)
	assert.Equal(t, "3.0.2", res.AirflowVersion)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "etl", m.Project.Name)
	assert.Equal(t, "3.0.2", m.Astro.AirflowVersion)
	// A full three-part pin becomes an exact requirement, not a prefix match.
	assert.Equal(t, []string{"apache-airflow==3.0.2"}, m.Project.Dependencies)
}

func TestRunRejectsInvalidNameAndVersion(t *testing.T) {
	_, err := Run(t.TempDir(), Options{Name: "-bad-"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project.name")

	_, err = Run(t.TempDir(), Options{AirflowVersion: "latest"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool.astro.airflow")
}

func TestDeriveName(t *testing.T) {
	cases := map[string]string{
		"flight-data":   "flight-data",
		"My Project":    "my-project",
		"data_pipeline": "data_pipeline",
		"v2.1":          "v2.1",
		"--weird--":     "weird",
		"...":           "astro-project",
		"héllo wörld":   "h-llo-w-rld",
	}
	for in, want := range cases {
		assert.Equal(t, want, deriveName(in), in)
	}
}

func TestRunRefusesExistingManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o600))
	_, err := Run(dir, Options{})
	require.ErrorIs(t, err, ErrManifestExists)
	assert.Contains(t, err.Error(), dir)
}

func TestRunRefusesV1Project(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".astro"), 0o700))
	_, err := Run(dir, Options{})
	require.ErrorIs(t, err, ErrV1Project)
	assert.Contains(t, err.Error(), "astro CLI 1.x")

	// A Dockerfile alone (no .astro/) is any container project, not v1.
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "Dockerfile"), []byte("FROM x\n"), 0o600))
	_, err = Run(dir2, Options{})
	require.NoError(t, err)
}

func TestRunKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	own := []byte("# mine\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), own, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Contains(t, res.Skipped, ".gitignore")
	assert.Contains(t, res.Skipped, "dags/")
	assert.NotContains(t, res.Created, ".gitignore")

	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, own, got)
}

func TestRunSymlinksClaudeMdOnUnix(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("symlink layout is unix-only")
	}
	dir := t.TempDir()
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Contains(t, res.Created, "CLAUDE.md -> AGENTS.md")

	link := filepath.Join(dir, "CLAUDE.md")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "CLAUDE.md must be a symlink")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "AGENTS.md", target)

	// An existing CLAUDE.md (the user's own) is kept.
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "CLAUDE.md"), []byte("mine"), 0o600))
	res, err = Run(dir2, Options{})
	require.NoError(t, err)
	assert.Contains(t, res.Skipped, "CLAUDE.md")
}

func TestRunWindowsGetsAgentsMdAlone(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(dir, Options{GOOS: windowsOS})
	require.NoError(t, err)

	_, err = os.Lstat(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	_, err = os.Lstat(filepath.Join(dir, "CLAUDE.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, entry := range append(res.Created, res.Skipped...) {
		assert.NotContains(t, entry, "CLAUDE.md")
	}
}

func TestRunCreatesMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "new-project")
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, "new-project", res.Name)
	_, err = os.Stat(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
}

func TestAgentsMdCarriesTheDevMapping(t *testing.T) {
	content := agentsContent()
	for _, m := range DevReplacements() {
		row := "| `astro dev " + m.Command + "` | `" + m.Replacement + "` |"
		assert.Contains(t, content, row)
	}
	assert.Contains(t, content, "pyproject.toml")
	assert.NotContains(t, content, DefaultAirflowVersion,
		"AGENTS.md must reference the manifest, not duplicate its values")
}
