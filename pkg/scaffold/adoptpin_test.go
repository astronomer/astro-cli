package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// adoptable writes a v1 project (a Dockerfile naming an Airflow 2 runtime)
// around a pyproject.toml pinned however the caller says.
func adoptable(t *testing.T, deps string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM quay.io/astronomer/astro-runtime:11.8.0\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
		[]byte("[project]\nname = 'already-here'\nversion = '1.0'\ndependencies = ["+deps+"]\n"), 0o600))
	return dir
}

// Adopting a manifest keeps the Airflow its requirement pins, over a
// Dockerfile's tag: the requirement is the project's version, and the only
// place the adopted manifest states it.
func TestAdoptKeepsTheManifestsOwnAirflowOverTheDockerfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		deps string
		want string
	}{
		{"a series, the shape a scaffolded project carries", "'apache-airflow==3.0.*'", "3.0"},
		{"an exact pin", "'apache-airflow==3.0.1'", "3.0.1"},
		{"a bare major series", "'apache-airflow==3.*'", "3"},
		{"alongside other dependencies", "'pandas>=2', 'apache-airflow==3.1.*'", "3.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := adoptable(t, tc.deps)

			res, err := Run(dir, Options{})
			require.NoError(t, err)

			m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)

			assert.Equal(t, tc.want, m.Airflow().Pin,
				"the version must come from the manifest's own pin, not the Dockerfile")
			assert.Equal(t, tc.want, res.AirflowVersion)

			// The dependency the project already had is still the one it has,
			// and nothing beside it states a second version.
			raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			assert.Contains(t, string(raw), "apache-airflow=="+tc.want)
			assert.NotContains(t, string(raw), "airflow = ", "adopt wrote a [tool.astro] airflow key")
		})
	}
}

// --airflow-version names the version, and the requirement is where it lives,
// so a clean pin the manifest already had moves to the flag's version, keeping
// its extras and marker. Before, the pin stayed on 3.1 while init reported
// 3.3 and bounded requires-python for 3.3.
func TestAdoptMovesTheManifestsPinToTheFlag(t *testing.T) {
	dir := adoptable(t, `'pandas', "apache-airflow[celery]==3.1.* ; sys_platform == 'linux'"`)

	res, err := Run(dir, Options{AirflowVersion: "3.3"})
	require.NoError(t, err)
	assert.Equal(t, "3.3", res.AirflowVersion)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "3.3", m.Airflow().Pin, "the manifest still runs the old Airflow")
	assert.Equal(t, []string{"pandas", "apache-airflow[celery]==3.3.*; sys_platform == 'linux'"}, m.Project.Dependencies)
	assert.Contains(t, res.Updated, "pyproject.toml (set the Airflow requirement to apache-airflow[celery]==3.3.*; sys_platform == 'linux')")
}

// The flag replaces a range too: it states the version the range did not, so
// the requirement can say it. Without the flag the range is refused, above.
func TestAdoptReplacesAnAirflowRangeWithTheFlag(t *testing.T) {
	dir := adoptable(t, "'apache-airflow[celery]>=3.0'")

	res, err := Run(dir, Options{AirflowVersion: "3.1"})
	require.NoError(t, err)
	assert.Equal(t, "3.1", res.AirflowVersion)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow[celery]==3.1.*"}, m.Project.Dependencies)
}

// Without the flag, the manifest's own pin is the version, and is not touched.
func TestAdoptLeavesTheManifestsPinWithoutTheFlag(t *testing.T) {
	dir := adoptable(t, "'apache-airflow == 3.1.*'")

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "'apache-airflow == 3.1.*'")
}

// A range names no single version, and the requirement is the only place a
// project states its version, so adopting a manifest pinned that way is
// refused, naming the line, whatever else could have supplied a version: the
// Dockerfile's tag here, or --airflow-version. Nothing is written.
func TestAdoptRefusesAnAirflowRange(t *testing.T) {
	for name, opts := range map[string]Options{
		"version from the Dockerfile": {},
	} {
		t.Run(name, func(t *testing.T) {
			dir := adoptable(t, "'apache-airflow>=3.0'")
			before, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)

			_, err = Run(dir, opts)

			var ve *manifest.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Contains(t, err.Error(), "apache-airflow>=3.0 does not pin an Airflow series")
			after, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "a refused adoption changed the manifest")
			assert.NoFileExists(t, filepath.Join(dir, "AGENTS.md"), "a refused adoption scaffolded the project")
		})
	}
}

// The quiet case: nothing to warn about when the manifest's own pin was used.
func TestAdoptSaysNothingWhenThePinCameFromTheManifest(t *testing.T) {
	dir := adoptable(t, "'apache-airflow==3.0.*'")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	for _, n := range res.Notes {
		assert.NotContains(t, n, "is the default, not this project's version",
			"the manifest's own pin was read, so there is nothing to warn about")
	}
}
