package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// buildCatalog has several builds of 3.3 carrying two patches, one of 3.4,
// each shipping Pythons, and one Airflow 2 build.
func buildCatalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{
  "runtimeVersions": {
    "13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable", "releaseDate": "2026-09-17"}}
  },
  "runtimeVersionsV3": {
    "3.3-3": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-06-01", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
    "3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-08-01", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-09-01", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.13"}},
    "3.4-1": {"metadata": {"airflowVersion": "3.4.0", "channel": "stable", "releaseDate": "2026-10-01", "pythonVersions": ["3.13", "3.14"], "defaultPythonVersion": "3.14"}}
  }
}`))
	require.NoError(t, err)
	return c
}

func seriesProject(requirement, extra string) string {
	return "[project]\nname = 'x'\nrequires-python = '>=3.12'\ndependencies = ['" + requirement + "']\n\n[tool.astro]\n" + extra
}

func TestSetRuntimeBuildWithinTheSeriesKeepsTheRequirement(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", ""), 0o644)

	change, err := SetRuntimeBuildWith(dir, nil, "3.3-10", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, RuntimeBuildChange{Runtime: "3.3-10", AirflowVersion: "3.3.2", Python: "3.13", Changed: true}, change)
	m, err := manifest.Parse([]byte(readFile(t, path)))
	require.NoError(t, err)
	assert.Equal(t, manifest.Airflow{Pin: "3.3", Runtime: "3.3-10"}, m.Airflow())
}

func TestSetRuntimeBuildGoesBackToAnOlderBuild(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", "runtime = '3.3-10'  # pinned\n"), 0o644)

	change, err := SetRuntimeBuildWith(dir, nil, "3.3-3", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "3.3-10", change.Previous)
	assert.True(t, change.Changed)
	assert.Contains(t, readFile(t, path), "runtime = '3.3-3'  # pinned", "edited in place, comment kept")
}

func TestSetRuntimeBuildOfAnotherSeriesMovesTheRequirement(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", "runtime = '3.3-7'\n"), 0o644)

	change, err := SetRuntimeBuildWith(dir, nil, "3.4-1", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	require.NotNil(t, change.Airflow)
	assert.Equal(t, "3.3", change.Airflow.Previous)
	assert.Equal(t, "3.4", change.Airflow.Version)
	assert.Empty(t, change.Airflow.Runtime, "the runtime line is this edit's, not the pin change's")
	m, err := manifest.Parse([]byte(readFile(t, path)))
	require.NoError(t, err)
	assert.Equal(t, manifest.Airflow{Pin: "3.4", Runtime: "3.4-1"}, m.Airflow())
}

// An exact pin stays exact: it moves to the patch the build carries, so the
// image and standalone run one Airflow.
func TestSetRuntimeBuildMovesAnExactPinToTheBuildsPatch(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.1", ""), 0o644)

	change, err := SetRuntimeBuildWith(dir, nil, "3.3-10", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	require.NotNil(t, change.Airflow)
	assert.Equal(t, "3.3.2", change.Airflow.Version)
	m, err := manifest.Parse([]byte(readFile(t, path)))
	require.NoError(t, err)
	assert.Equal(t, manifest.Airflow{Pin: "3.3.2", Runtime: "3.3-10"}, m.Airflow())
}

func TestSetRuntimeBuildPicksAPython(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", ""), 0o644)

	change, err := SetRuntimeBuildWith(dir, nil, "3.3-10", "3.12", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, "==3.12.*", change.RequiresPython)
	assert.Equal(t, "3.12", change.Python)
	assert.Contains(t, readFile(t, path), "requires-python = '==3.12.*'")

	// Without --python again, the build's default comes back.
	change, err = SetRuntimeBuildWith(dir, nil, "3.3-10", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, ">=3.12", change.RequiresPython, "the series' bound, from the catalog")
	assert.Equal(t, "3.13", change.Python)
}

// A Python given across a series move is the one requires-python reports,
// once: the pin change's own bound was overwritten, so it is not reported.
func TestSetRuntimeBuildReportsThePythonGivenAcrossASeriesMove(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", ""), 0o644)
	change, err := SetRuntimeBuildWith(dir, nil, "3.4-1", "3.13", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	require.NotNil(t, change.Airflow)
	assert.Empty(t, change.Airflow.RequiresPython)
	assert.Equal(t, "==3.13.*", change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '==3.13.*'")
}

func TestSetRuntimeBuildKeepsAChosenRangeThatAdmitsTheDefault(t *testing.T) {
	dir, _ := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", ""), 0o644)
	change, err := SetRuntimeBuildWith(dir, nil, "3.3-10", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	assert.Empty(t, change.RequiresPython, ">=3.12 admits 3.13 and stays")
}

func TestSetRuntimeBuildRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, src, build, python string
		catalog                  bool
		want                     string
	}{
		{"a floating tag", seriesProject("apache-airflow==3.3.*", ""), "3.3", "", true, "not one build"},
		{"not a tag", seriesProject("apache-airflow==3.3.*", ""), "latest", "", true, "not one build"},
		{"a Python the build lacks", seriesProject("apache-airflow==3.3.*", ""), "3.3-7", "3.14", true, "ships Python 3.12, 3.13"},
		{"a Python that is not one", seriesProject("apache-airflow==3.3.*", ""), "3.3-7", "3", true, "not a Python version"},
		{"a Python for Airflow 2", seriesProject("apache-airflow==2.11.*", ""), "13.11.0", "3.12", true, "Airflow 2 runtime"},
		{"an unlisted Airflow 2 build", seriesProject("apache-airflow==2.11.*", ""), "13.99.0", "", true, "lists no build 13.99.0"},
		{"an unlisted build of another series", seriesProject("apache-airflow==3.3.*", ""), "3.9-1", "", true, "not moved to Airflow 3.9"},
		{"an Airflow 2 build offline", seriesProject("apache-airflow==2.11.*", ""), "13.11.0", "", false, "could not be read"},
		{"a declared Dockerfile", seriesProject("apache-airflow==3.3.*", "dockerfile = 'Dockerfile'\n"), "3.3-7", "", true, "FROM line"},
		{"a range no build Python meets", "[project]\nname = 'x'\nrequires-python = '>=3.14'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n", "3.3-7", "", true, "Pass --python"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := writeEditFixture(t, tc.src, 0o644)
			opts := AirflowPinOptions{}
			if tc.catalog {
				opts.Catalog = buildCatalog(t)
			}
			_, err := SetRuntimeBuildWith(dir, nil, tc.build, tc.python, opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, tc.src, readFile(t, path), "nothing written")
		})
	}
}

// Offline, an Airflow 3 build's tag still names its series, so the
// requirement can follow it.
func TestSetRuntimeBuildOfflineReadsTheSeriesOffTheTag(t *testing.T) {
	dir, path := writeEditFixture(t, seriesProject("apache-airflow==3.3.*", ""), 0o644)

	change, err := SetRuntimeBuildWith(dir, nil, "3.4-9", "", AirflowPinOptions{})
	require.NoError(t, err)
	require.NotNil(t, change.Airflow)
	assert.Equal(t, "3.4", change.Airflow.Version)
	assert.Empty(t, change.Python, "no catalog, no Python to tell")
	m, err := manifest.Parse([]byte(readFile(t, path)))
	require.NoError(t, err)
	assert.Equal(t, manifest.Airflow{Pin: "3.4", Runtime: "3.4-9"}, m.Airflow())
}

func TestSetRuntimeBuildTheSameBuildWritesNothing(t *testing.T) {
	src := seriesProject("apache-airflow==3.3.*", "runtime = '3.3-10'\n")
	dir, path := writeEditFixture(t, src, 0o644)
	change, err := SetRuntimeBuildWith(dir, nil, "3.3-10", "", AirflowPinOptions{Catalog: buildCatalog(t)})
	require.NoError(t, err)
	assert.False(t, change.Changed)
	assert.Equal(t, src, readFile(t, path))
}
