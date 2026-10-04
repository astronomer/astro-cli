package imagebuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// pythonCatalogJSON has two builds of 3.3 that run Python 3.14 by default, a
// newer yanked one, a newer still that is not released yet, and an Airflow 2
// build that lists no Python.
const pythonCatalogJSON = `{
  "runtimeVersions": {
    "13.11.0": {"metadata": {"airflowVersion": "2.11.2", "channel": "stable"}}
  },
  "runtimeVersionsV3": {
    "3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "yanked": true, "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.3", "channel": "stable", "releaseDate": "2999-01-01", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}},
    "3.2-10": {"metadata": {"airflowVersion": "3.2.2", "channel": "stable", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.13"}},
    "3.1-2": {"metadata": {"airflowVersion": "3.1.0", "channel": "stable"}}
  }
}`

func pythonCatalog(t *testing.T) func() *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(pythonCatalogJSON))
	require.NoError(t, err)
	return func() *runtimeversions.Catalog { return c }
}

func TestRuntimeImageForPythonKeepsTheTagForTheDefaultPython(t *testing.T) {
	catalog := pythonCatalog(t)
	for _, tt := range []struct{ pin, runtime, requires, want string }{
		{"3.3", "", ">=3.12", RuntimeImageRepo + ":3.3"},
		{"3.3", "", "==3.14.*", RuntimeImageRepo + ":3.3"},
		{"3.3", "3.3-7", ">=3.12", RuntimeImageRepo + ":3.3-7"},
		{"3.2", "", ">=3.12", RuntimeImageRepo + ":3.2"},
		{"3.2", "", "==3.14.*", RuntimeImageRepo + ":3.2-10-python-3.14"},
	} {
		ref, err := RuntimeImageForPython(tt.pin, tt.runtime, tt.requires, catalog)
		require.NoError(t, err)
		assert.Equal(t, tt.want, ref, "%+v", tt)
	}
}

func TestRuntimeImageForPythonNamesAnotherPythonOnTheExactBuild(t *testing.T) {
	catalog := pythonCatalog(t)

	ref, err := RuntimeImageForPython("3.3", "3.3-7", "==3.13.*", catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-7-python-3.13", ref)

	ref, err = RuntimeImageForPython("3.3", "", "==3.13.*", catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-8-python-3.13", ref, "no runtime pinned: the newest build of the pin that is neither yanked nor unreleased")

	ref, err = RuntimeImageForPython("3.3.1", "", ">=3.12,<3.14", catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-8-python-3.13", ref,
		"a patch pin builds the series, as runtime:3.3 does, so requires-python does not choose the Airflow")
}

// A build whose catalog entry names no default Python still refuses a
// requires-python it ships nothing for, rather than reading the empty answer
// as the default.
func TestRuntimeImageForPythonRefusesWithNoDefaultListed(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13"]}}
  }}`))
	require.NoError(t, err)
	_, err = RuntimeImageForPython("3.3", "", "==3.11.*", func() *runtimeversions.Catalog { return c })
	assert.ErrorContains(t, err, "admits none of the Pythons runtime 3.3-8 ships (3.12, 3.13)")
}

func TestRuntimeImageForPythonRefusesARequiresPythonNoBuildPythonMeets(t *testing.T) {
	_, err := RuntimeImageForPython("3.3", "", "==3.11.*", pythonCatalog(t))
	assert.EqualError(t, err, "requires-python ==3.11.* in pyproject.toml admits none of the Pythons runtime 3.3-8 ships (3.12, 3.13, 3.14). "+
		"Change requires-python to admit one of them, or build from a runtime that ships one it admits")
}

func TestRuntimeImageForPythonKeepsTheTagWithNothingToDecideOn(t *testing.T) {
	calls := 0
	counted := func() *runtimeversions.Catalog { calls++; return pythonCatalog(t)() }
	ref, err := RuntimeImageForPython("3.3", "", "", counted)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3", ref)
	assert.Zero(t, calls, "no requires-python, so the catalog is not read")

	for _, tt := range []struct {
		name    string
		pin     string
		runtime string
		catalog func() *runtimeversions.Catalog
	}{
		{name: "no catalog reader", pin: "3.3"},
		{name: "catalog unreadable", pin: "3.3", catalog: func() *runtimeversions.Catalog { return nil }},
		{name: "no build of the pin", pin: "3.5", catalog: pythonCatalog(t)},
		{name: "build not listed", pin: "3.3", runtime: "3.3-12", catalog: pythonCatalog(t)},
		{name: "build lists no Python", pin: "3.1", runtime: "3.1-2", catalog: pythonCatalog(t)},
	} {
		want, err := RuntimeImageFor(tt.pin, tt.runtime)
		require.NoError(t, err, tt.name)
		ref, err := RuntimeImageForPython(tt.pin, tt.runtime, "==3.13.*", tt.catalog)
		require.NoError(t, err, tt.name)
		assert.Equal(t, want, ref, tt.name)
	}
}

func TestForManifestPicksThePythonRequiresPythonAdmits(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.3",
		RequiresPython: "==3.13.*",
	}, pythonCatalog(t))
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-8-python-3.13", req.BaseImage)

	calls := 0
	req, err = ForManifest(ManifestBuild{
		ProjectDir:     declaredProject(t),
		AirflowVersion: "3.3",
		RequiresPython: "==3.13.*",
		Dockerfile:     "docker/Dockerfile",
	}, func() *runtimeversions.Catalog { calls++; return nil })
	require.NoError(t, err)
	assert.Empty(t, req.BaseImage)
	assert.Zero(t, calls, "a declared Dockerfile names its own base, so the catalog is not read")
}

func TestLocalRuntimeImageAirflow3ReadsRequiresPython(t *testing.T) {
	_, cache := withService(t, pythonCatalogJSON, false)
	ref, err := LocalRuntimeImageWith(t.Context(), "3.3", "", "==3.13.*", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-8-python-3.13", ref)

	_, cache = withService(t, "", true)
	ref, err = LocalRuntimeImageWith(t.Context(), "3.3", "", "==3.13.*", runtimeversions.Options{CacheDir: cache})
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3", ref, "offline, a start builds as it did before")
}

func TestStandalonePythonFollowsTheImage(t *testing.T) {
	catalog := pythonCatalog(t)
	offline := func() *runtimeversions.Catalog { return nil }
	for _, tt := range []struct {
		name, pin, requires string
		catalog             func() *runtimeversions.Catalog
		want                string
	}{
		{"the build's default", "3.2", ">=3.12", catalog, "3.13"},
		{"another Python", "3.3", "==3.13.*", catalog, "3.13"},
		{"offline keeps uv's choice", "3.3", "==3.13.*", offline, ""},
		{"offline, no requires-python", "3.3", "", offline, "3.12"},
		{"airflow 2", "2.11", ">=3.10", catalog, ""},
		{"airflow 2 before 2.9, no requires-python", "2.8", "", catalog, "3.11"},
	} {
		python, err := StandalonePython(tt.pin, "", tt.requires, tt.catalog)
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, python, tt.name)
	}
}

// What refuses an image warns a venv: the fallback comes back with the error.
func TestStandalonePythonReportsWhatAnImageRefuses(t *testing.T) {
	python, err := StandalonePython("3.3", "", "==3.11.*", pythonCatalog(t))
	var notShipped *runtimeversions.PythonNotShippedError
	require.ErrorAs(t, err, &notShipped)
	assert.Equal(t, "3.3-8", notShipped.Build)
	assert.Empty(t, python, "uv picks within requires-python")
}
