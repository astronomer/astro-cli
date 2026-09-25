package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// pinCatalog lists two future-looking series whose runtimes ship a Python floor
// the built-in rule would not derive (it gives every Airflow 3 from 3.2 on
// >=3.12), so a bound this test sees can only have come from the catalog.
func pinCatalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
		"3.40-1": {"metadata": {"airflowVersion": "3.40.0", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.13", "3.14"]}},
		"3.41-1": {"metadata": {"airflowVersion": "3.41.0", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.14", "3.15"]}}}}`))
	require.NoError(t, err)
	return c
}

const catalogPinned = "[project]\nname = 'x'\nrequires-python = '>=3.13'\ndependencies = ['apache-airflow==3.40.*']\n\n[tool.astro]\n"

// init wrote >=3.13 from the catalog for 3.40. With the catalog in hand that is
// a bound this package wrote, so it moves, to the catalog's bound for the new
// series.
func TestSetAirflowVersionWithMovesACatalogBound(t *testing.T) {
	dir, path := writeEditFixture(t, catalogPinned, 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.41", AirflowPinOptions{Catalog: pinCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, ">=3.14", change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.14'")
}

// Without the catalog, the same bound is not one the built-in rule derives, so
// it reads as the user's and stays.
func TestSetAirflowVersionWithoutTheCatalogKeepsACatalogBound(t *testing.T) {
	dir, path := writeEditFixture(t, catalogPinned, 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.41", AirflowPinOptions{})
	require.NoError(t, err)
	assert.Empty(t, change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.13'")
}

// A series the catalog does not list, and Airflow 2 always, take the built-in
// rule's bound.
func TestSetAirflowVersionWithFallsBackToTheBuiltInRule(t *testing.T) {
	for version, want := range map[string]string{
		"3.2":  ">=3.12",
		"2.10": ">=3.10,<3.13",
	} {
		dir, path := writeEditFixture(t, catalogPinned, 0o644)
		change, err := SetAirflowVersionWith(dir, nil, version, AirflowPinOptions{Catalog: pinCatalog(t)})
		require.NoError(t, err)
		assert.Equal(t, want, change.RequiresPython, version)
		assert.Contains(t, readFile(t, path), "requires-python = '"+want+"'", version)
	}
}

// A built-in bound still moves when the caller has the catalog, and lands on
// the catalog's bound for the new series.
func TestSetAirflowVersionWithMovesABuiltInBound(t *testing.T) {
	dir, path := writeEditFixture(t,
		"[project]\nname = 'x'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n",
		0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.40", AirflowPinOptions{Catalog: pinCatalog(t)})
	require.NoError(t, err)
	assert.Equal(t, ">=3.13", change.RequiresPython)
	assert.Contains(t, readFile(t, path), "requires-python = '>=3.13'")
}

// Setting the pin a project already has writes nothing, even when the catalog's
// bound for it differs from the built-in one that was written offline.
func TestSetAirflowVersionWithTheSamePinWritesNothing(t *testing.T) {
	body := "[project]\nname = 'x'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.40.*']\n\n[tool.astro]\n"
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.40", AirflowPinOptions{Catalog: pinCatalog(t)})
	require.NoError(t, err)
	assert.False(t, change.Changed)
	assert.Equal(t, body, readFile(t, path))
}

// SetAirflowVersionWith is the one implementation, so the catalog-aware call
// carries the repairs too: on a manifest init wrote before the requirement was
// the version, with the leftover key, it reads the previous version from the
// requirement, deletes the key, and moves the catalog's bound.
func TestSetAirflowVersionWithRepairsALeftoverKey(t *testing.T) {
	body := "[project]\nname = 'x'\nrequires-python = '>=3.13'\ndependencies = ['apache-airflow==3.40.*']\n\n[tool.astro]\nairflow = '3.40'\n"
	dir, path := writeEditFixture(t, body, 0o644)

	change, err := SetAirflowVersionWith(dir, nil, "3.41", AirflowPinOptions{Catalog: pinCatalog(t)})
	require.NoError(t, err)

	assert.Equal(t, "3.40", change.Previous)
	assert.True(t, change.RemovedAirflowKey)
	assert.Equal(t, []string{"apache-airflow==3.41.*"}, change.Requirements)
	assert.Equal(t, ">=3.14", change.RequiresPython)
	assert.Equal(t, "[project]\nname = 'x'\nrequires-python = '>=3.14'\ndependencies = ['apache-airflow==3.41.*']\n\n[tool.astro]\n", readFile(t, path))
}
