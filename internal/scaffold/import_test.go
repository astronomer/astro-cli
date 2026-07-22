package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// writeFiles lays out a fixture repo: each map entry is a path relative to dir
// (a trailing slash means an empty directory).
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		if content == "" && name[len(name)-1] == '/' {
			require.NoError(t, os.MkdirAll(path, 0o755))
			continue
		}
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

// okLocker locks without complaint.
type okLocker struct{ called bool }

func (l *okLocker) Lock(_ context.Context, _ string, _ uv.Stdio) error {
	l.called = true
	return nil
}

// failLocker fails with a typed resolution error, as uv would on a conflict.
type failLocker struct{}

func (failLocker) Lock(_ context.Context, _ string, _ uv.Stdio) error {
	return &uv.ResolutionError{
		Op:          "lock",
		Packages:    []string{"flask", "werkzeug"},
		Constraints: []string{"flask>=2.2", "werkzeug<2"},
		Summary:     "flask>=2.2 needs werkzeug>=2, but werkzeug<2 is pinned",
	}
}

func TestImportPlainRepo(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/etl.py":      "# a dag\n",
		"dags/util/aux.py": "# helper\n",
		"requirements.txt": "flask==2.0\nrequests\n",
	})
	dst := filepath.Join(t.TempDir(), "imported")

	loc := &okLocker{}
	res, err := Import(context.Background(), src, dst, ImportOptions{Lock: loc})
	require.NoError(t, err)

	assert.Equal(t, "imported", res.Name)
	assert.Equal(t, DefaultAirflowVersion, res.Airflow)
	assert.Equal(t, "default", res.AirflowFrom)
	assert.Equal(t, 2, res.Dags)
	// The source named no Airflow, so the import adds the pin's requirement
	// alongside the two carried lines.
	assert.Equal(t, 3, res.Dependencies)
	assert.Contains(t, strings.Join(res.Warnings, "\n"), "added apache-airflow==3.1.*")
	assert.True(t, res.Lock.Attempted)
	assert.True(t, res.Lock.Locked)
	assert.True(t, loc.called)

	// The manifest loads, carries the requirements verbatim, and gains the
	// apache-airflow line so the project can start with no hand-edit.
	m, err := manifest.Load(filepath.Join(dst, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, []string{"flask==2.0", "requests", "apache-airflow==3.1.*"}, m.Project.Dependencies)

	// Dags are copied (source untouched), preserving the subtree.
	for _, f := range []string{"dags/etl.py", "dags/util/aux.py", "AGENTS.md", ".gitignore"} {
		_, err := os.Stat(filepath.Join(dst, f))
		require.NoError(t, err, f)
	}
	// The greenfield layout is scaffolded too.
	for _, d := range projectDirs {
		info, err := os.Stat(filepath.Join(dst, d))
		require.NoError(t, err, d)
		assert.True(t, info.IsDir(), d)
	}
}

func TestImportLiftsAirflowPinAndCarriesGarbage(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py": "# dag\n",
		"requirements.txt": "# base deps\n" +
			"apache-airflow==2.9.1\n" +
			"pandas==2.2.0  # data\n" +
			"-r other.txt\n" +
			"!!!nonsense!!!\n",
	})
	dst := filepath.Join(t.TempDir(), "proj")

	res, err := Import(context.Background(), src, dst, ImportOptions{})
	require.NoError(t, err)

	assert.Equal(t, "2.9.1", res.Airflow)
	assert.Equal(t, "requirements", res.AirflowFrom)
	// apache-airflow, pandas -> two dependencies; the -r and garbage lines
	// are carried, not counted.
	assert.Equal(t, 2, res.Dependencies)

	// Both carried lines surface as warnings, nothing dropped.
	warned := strings.Join(res.Warnings, "\n")
	assert.Contains(t, warned, "other.txt")
	assert.Contains(t, warned, "nonsense")

	m, err := manifest.Load(filepath.Join(dst, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "2.9.1", m.Astro.AirflowVersion)
	assert.Equal(t, []string{"apache-airflow==2.9.1", "pandas==2.2.0"}, m.Project.Dependencies)

	// The carried lines live in the file as comments.
	raw, err := os.ReadFile(filepath.Join(dst, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "other.txt")
	assert.Contains(t, string(raw), "nonsense")

	// No locker was passed: the lock is skipped and reported as such.
	assert.False(t, res.Lock.Attempted)
}

func TestImportDagsAtRoot(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"pipeline.py":      "# dag\n",
		"helpers.py":       "# not really a dag but a .py at root\n",
		"README.md":        "hi\n",
		"requirements.txt": "requests\n",
	})
	dst := filepath.Join(t.TempDir(), "proj")

	res, err := Import(context.Background(), src, dst, ImportOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Dags)
	for _, f := range []string{"dags/pipeline.py", "dags/helpers.py"} {
		_, err := os.Stat(filepath.Join(dst, f))
		require.NoError(t, err, f)
	}
	// A non-.py root file is not swept in.
	_, err = os.Stat(filepath.Join(dst, "dags", "README.md"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestImportCopiesPlugins(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py":            "# dag\n",
		"plugins/my_plugin.py": "# plugin\n",
		"plugins/sub/other.py": "# nested plugin\n",
		"requirements.txt":     "requests\n",
	})
	dst := filepath.Join(t.TempDir(), "proj")

	res, err := Import(context.Background(), src, dst, ImportOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Plugins)
	_, err = os.Stat(filepath.Join(dst, "plugins", "sub", "other.py"))
	require.NoError(t, err)
}

func TestImportMissingRequirements(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{"dags/d.py": "# dag\n"})
	dst := filepath.Join(t.TempDir(), "proj")

	res, err := Import(context.Background(), src, dst, ImportOptions{})
	require.NoError(t, err)
	// No requirements.txt, but the import still lands the Airflow pin so the
	// project can start.
	assert.Equal(t, 1, res.Dependencies)
	assert.Contains(t, res.Warnings, "no requirements.txt in the source")
	assert.Contains(t, strings.Join(res.Warnings, "\n"), "added apache-airflow==3.1.*")

	m, err := manifest.Load(filepath.Join(dst, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow==3.1.*"}, m.Project.Dependencies)
}

func TestImportLockFailureIsReportedNotRolledBack(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py":        "# dag\n",
		"requirements.txt": "flask>=2.2\nwerkzeug<2\n",
	})
	dst := filepath.Join(t.TempDir(), "proj")

	res, err := Import(context.Background(), src, dst, ImportOptions{Lock: failLocker{}})
	require.NoError(t, err) // a lock failure is a report, not an error
	assert.True(t, res.Lock.Attempted)
	assert.False(t, res.Lock.Locked)
	assert.Equal(t, []string{"flask", "werkzeug"}, res.Lock.Packages)
	assert.NotEmpty(t, res.Lock.Summary)

	// The project stays on disk.
	_, err = os.Stat(filepath.Join(dst, "pyproject.toml"))
	require.NoError(t, err)
}

func TestImportHonorsAirflowFlagOverPin(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py":        "# dag\n",
		"requirements.txt": "apache-airflow==2.9.1\n",
	})
	dst := filepath.Join(t.TempDir(), "proj")
	res, err := Import(context.Background(), src, dst, ImportOptions{AirflowVersion: "3.0.2"})
	require.NoError(t, err)
	assert.Equal(t, "3.0.2", res.Airflow)
	assert.Equal(t, "flag", res.AirflowFrom)
}

func TestImportRefusesAlreadyProjectSource(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py":      "# dag\n",
		"pyproject.toml": "[project]\nname='x'\n",
	})
	_, err := Import(context.Background(), src, filepath.Join(t.TempDir(), "proj"), ImportOptions{})
	require.ErrorIs(t, err, ErrSourceIsProject)
}

func TestImportRefusesV1Source(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py":  "# dag\n",
		"Dockerfile": "FROM astro\n",
		".astro/":    "",
	})
	_, err := Import(context.Background(), src, filepath.Join(t.TempDir(), "proj"), ImportOptions{})
	require.ErrorIs(t, err, ErrV1Project)
}

func TestImportRefusesNonAirflowRepo(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{"README.md": "hi\n", "requirements.txt": "flask\n"})
	_, err := Import(context.Background(), src, filepath.Join(t.TempDir(), "proj"), ImportOptions{})
	require.ErrorIs(t, err, ErrNotAirflowRepo)
}

func TestImportRefusesInPlace(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{"dags/d.py": "# dag\n"})
	// Target equal to source.
	_, err := Import(context.Background(), src, src, ImportOptions{})
	require.ErrorIs(t, err, ErrImportInPlace)
	// Target inside source.
	_, err = Import(context.Background(), src, filepath.Join(src, "out"), ImportOptions{})
	require.ErrorIs(t, err, ErrImportInPlace)
}

func TestImportRefusesExistingManifestTarget(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{"dags/d.py": "# dag\n"})
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dst, "pyproject.toml"), []byte("[project]\n"), 0o644))
	_, err := Import(context.Background(), src, dst, ImportOptions{})
	require.ErrorIs(t, err, ErrManifestExists)
}

func TestImportDoesNotTouchSource(t *testing.T) {
	src := t.TempDir()
	writeFiles(t, src, map[string]string{
		"dags/d.py":        "# dag\n",
		"requirements.txt": "flask\n",
	})
	before, err := os.ReadDir(src)
	require.NoError(t, err)
	_, err = Import(context.Background(), src, filepath.Join(t.TempDir(), "proj"), ImportOptions{})
	require.NoError(t, err)
	after, err := os.ReadDir(src)
	require.NoError(t, err)
	assert.Equal(t, len(before), len(after), "source directory must be unchanged")
	_, err = os.Stat(filepath.Join(src, "pyproject.toml"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
