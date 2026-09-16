package scaffold

import (
	"os"
	"path/filepath"
	"strings"
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

// A manifest must never come out of init declaring two different Airflows.
//
// The dependency pin is what uv resolves and [tool.astro] airflow is what the
// CLI installs and runs, so a disagreement is a project that runs one Airflow
// while declaring another — and nothing downstream compares them, so it
// surfaces much later as a failure with no obvious cause.
//
// Adopting a project pinned as a series produced exactly that: the series was
// unreadable, so the pin fell through to the Dockerfile's image tag and
// airflow = '2' landed beside apache-airflow==3.0.*.
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

			assert.Equal(t, tc.want, m.Astro.AirflowVersion,
				"[tool.astro] airflow must come from the manifest's own pin, not the Dockerfile")
			assert.Equal(t, tc.want, res.AirflowVersion)

			// The dependency the project already had is still the one it has,
			// so the two halves of the manifest agree.
			raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
			require.NoError(t, err)
			assert.Contains(t, string(raw), "apache-airflow=="+tc.want,
				"the dependency and the [tool.astro] pin must name the same Airflow")
		})
	}
}

// A range names no single version, so no pin can be read out of it and the
// version has to come from somewhere else. That is allowed — but it is the
// remaining way the two halves can disagree, so the run says so rather than
// leaving it to be found at start.
func TestAdoptSaysWhenItPinnedAnAirflowTheManifestDidNotName(t *testing.T) {
	dir := adoptable(t, "'apache-airflow>=3.0'")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	assert.Equal(t, "2", res.AirflowVersion, "the Dockerfile is the only readable source here")

	var found string
	for _, n := range res.Notes {
		if strings.Contains(n, "no single version reads out of") {
			found = n
		}
	}
	require.NotEmpty(t, found, "expected a note about the unread pin, got: %v", res.Notes)
	assert.Contains(t, found, "airflow = '2'", "the note names the pin that was written")
}

// The quiet case: nothing to warn about when the manifest's own pin was used.
func TestAdoptSaysNothingWhenThePinCameFromTheManifest(t *testing.T) {
	dir := adoptable(t, "'apache-airflow==3.0.*'")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	for _, n := range res.Notes {
		assert.NotContains(t, n, "no single version reads out of",
			"the manifest's own pin was read, so there is nothing to warn about")
	}
}
