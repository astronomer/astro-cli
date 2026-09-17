package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Airflow 2 cannot run on Python 3.13, and nothing but requires-python stops
// uv choosing one: rt.Params leaves PythonVersion empty so uv picks from the
// manifest, and Airflow 2's own metadata carries no upper bound.
func TestRequiresPythonBoundsAirflowTwo(t *testing.T) {
	for _, tc := range []struct{ airflow, want string }{
		// A bare "2" is the newest Airflow 2, which is past 2.9.
		{"2", ">=3.10,<3.13"},
		// Python 3.12 support arrived in 2.9. docs/install.md says the same:
		// "Airflow 2.7 wants 3.11 or lower; later 2.x releases reach further."
		{"2.7", ">=3.10,<3.12"},
		{"2.8", ">=3.10,<3.12"},
		{"2.8.4", ">=3.10,<3.12"},
		{"2.9", ">=3.10,<3.13"},
		{"2.10", ">=3.10,<3.13"},
		{"2.10.5", ">=3.10,<3.13"},
		{"2.11", ">=3.10,<3.13"},
		// Airflow 3 tracks new interpreters, so it is left open rather than
		// capped at whatever was current when this was written.
		{"3", ">=3.10"},
		{"3.1", ">=3.10"},
		{"3.1.2", ">=3.10"},
	} {
		assert.Equal(t, tc.want, requiresPython(tc.airflow), tc.airflow)
	}
}

// The scaffolded manifest carries it, so a project pinned to Airflow 2 is
// unstartable no longer: without the bound uv builds the venv on the newest
// Python present, Airflow 2 installs against it, and the first start fails
// inside werkzeug with "module 'ast' has no attribute 'Str'".
func TestInitPinsAnInterpreterTheAirflowCanRun(t *testing.T) {
	for _, tc := range []struct{ airflow, want string }{
		{"2.10", ">=3.10,<3.13"},
		{"3.1", ">=3.10"},
	} {
		t.Run(tc.airflow, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Run(dir, Options{AirflowVersion: tc.airflow})
			require.NoError(t, err)

			m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, m.Project.RequiresPython)
		})
	}
}

// A v1 project converts through the same scaffold, and every v1 project is an
// Airflow 2 one — so this is the path a migrating user actually takes.
func TestConvertingAV1ProjectBoundsTheInterpreter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM quay.io/astronomer/astro-runtime:11.8.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"),
		[]byte("pandas>=2.0\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "2", res.AirflowVersion, "the Dockerfile pins Airflow 2")

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, ">=3.10,<3.13", m.Project.RequiresPython)
}

// Adoption fills it when the manifest states none, and leaves one its author
// wrote alone: which interpreters a project supports is their decision, and
// this run is not in a position to overrule it.
func TestAdoptOnlyFillsAMissingRequiresPython(t *testing.T) {
	t.Run("absent, so it is filled", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
			[]byte("FROM quay.io/astronomer/astro-runtime:11.8.0\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
			[]byte("[project]\nname = 'x'\nversion = '1.0'\n"), 0o600))

		_, err := Run(dir, Options{})
		require.NoError(t, err)

		m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
		require.NoError(t, err)
		assert.Equal(t, ">=3.10,<3.13", m.Project.RequiresPython)
	})

	t.Run("stated, so it is left alone", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
			[]byte("FROM quay.io/astronomer/astro-runtime:11.8.0\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
			[]byte("[project]\nname = 'x'\nversion = '1.0'\nrequires-python = '>=3.11'\n"), 0o600))

		_, err := Run(dir, Options{})
		require.NoError(t, err)

		m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
		require.NoError(t, err)
		assert.Equal(t, ">=3.11", m.Project.RequiresPython, "the author's own bound stands")
	})
}
