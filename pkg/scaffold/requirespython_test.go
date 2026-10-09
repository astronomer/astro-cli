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
		// capped at whatever was current when this was written. The floor is
		// the runtime's: 3.12 from runtime 3.2 on, and a bare "3" is the newest.
		{"3", ">=3.12"},
		{"3.0", ">=3.10"},
		{"3.1", ">=3.10"},
		{"3.1.2", ">=3.10"},
		{"3.2", ">=3.12"},
		{"3.3", ">=3.12"},
		{"3.3.2", ">=3.12"},
		{"3.10", ">=3.12"},
		{"4", ">=3.12"},
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
		{"3.3", ">=3.12"},
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

// A 1.x project converts through the same scaffold, and every 1.x project is an
// Airflow 2 one — so this is the path a migrating user actually takes.
func TestConvertingA1xProjectBoundsTheInterpreter(t *testing.T) {
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

// A Dockerfile that stays the build runs the Python its base's tag names, so
// the manifest pins that minor: uv then locks for, and standalone mode runs,
// the interpreter the image runs, and no other.
func TestADeclaredDockerfilesPythonIsTheProjects(t *testing.T) {
	for _, tc := range []struct{ name, dockerfile, want string }{
		{
			"an Airflow 3 base naming its Python",
			"FROM astrocrpublic.azurecr.io/runtime:3.3-2-python-3.13\nRUN pip install --no-cache-dir uv\n",
			"==3.13.*",
		},
		{
			"an Airflow 2 base naming its Python",
			"FROM quay.io/astronomer/astro-runtime:12.1.0-python-3.11\nUSER root\n",
			"==3.11.*",
		},
		{
			"a base naming none keeps the rule",
			"FROM astrocrpublic.azurecr.io/runtime:3.3-2\nRUN pip install --no-cache-dir uv\n",
			">=3.12",
		},
		{
			// The file goes, so its Python choice is carried, and a
			// generated image runs the Python requires-python admits.
			"a Dockerfile that is only a pin carries its Python",
			"FROM astrocrpublic.azurecr.io/runtime:3.3-2-python-3.13\n",
			"==3.13.*",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(tc.dockerfile), 0o600))
			require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))

			_, err := Run(dir, Options{})
			require.NoError(t, err)

			m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, m.Project.RequiresPython)
		})
	}
}

// Adoption leaves a stated requires-python alone here too, and says so when
// it is not the image's.
func TestAdoptNotesARequiresPythonThatIsNotTheImages(t *testing.T) {
	const dockerfile = "FROM astrocrpublic.azurecr.io/runtime:3.3-2-python-3.13\nRUN pip install --no-cache-dir uv\n"
	for _, tc := range []struct{ name, stated, want, note string }{
		{"absent, so the image's is written", "", "==3.13.*", ""},
		{"the image's, so nothing is said", "requires-python = '==3.13.*'\n", "==3.13.*", ""},
		{
			"another, so it stays and is noted", "requires-python = '>=3.12'\n", ">=3.12",
			"[project] requires-python is >=3.12, and the Dockerfile's base image runs Python 3.13: " +
				"uv locks for every Python requires-python allows, and standalone mode can run one the image does not. " +
				"Set it to ==3.13.* to match the image",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
				[]byte("[project]\nname = 'x'\nversion = '1.0'\n"+tc.stated), 0o600))

			res, err := Run(dir, Options{})
			require.NoError(t, err)

			m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, m.Project.RequiresPython)
			if tc.note == "" {
				for _, n := range res.Notes {
					assert.NotContains(t, n, "requires-python")
				}
				return
			}
			assert.Contains(t, res.Notes, tc.note)
		})
	}
}

// An Airflow 2 bound with no "<" is too loose only when nothing names the
// Python: the image's own minor, written by its author, is not.
func TestAnAirflowTwoImagesPythonIsNotTooLoose(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM quay.io/astronomer/astro-runtime:12.1.0-python-3.11\nUSER root\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
		[]byte("[project]\nname = 'x'\nversion = '1.0'\nrequires-python = '==3.11.*'\n"), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	for _, n := range res.Notes {
		assert.NotContains(t, n, "requires-python")
	}
}
