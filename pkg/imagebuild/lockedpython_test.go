package imagebuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// The image of the lock's build runs the recorded Python, not
// the build's default, so it is the image the lock was made for. A
// requires-python that excludes that Python, or a build the catalog says
// carries a release the pin no longer covers, builds on the newest build of
// the series instead, and the notices say why.
func TestTheLocksBuildRunsTheLocksPython(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(lockedCatalog))
	require.NoError(t, err)
	catalog := func() *runtimeversions.Catalog { return c }

	m := ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3", LockedRuntime: "3.3-10", LockedPython: "3.13", RequiresPython: ">=3.12"}
	req, err := ForManifest(m, catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-10-python-3.13", req.BaseImage, "the lock's Python, not the build's default 3.12")
	assert.True(t, LockedRuntimeUsed(m, c))

	m.RequiresPython = "<3.13"
	req, err = ForManifest(m, catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3", req.BaseImage, "the series, on a Python requires-python admits")
	assert.False(t, LockedRuntimeUsed(m, c))
	notices := LockedRuntimeNotices(m, c)
	require.Len(t, notices, 1)
	assert.Contains(t, notices[0], "excludes Python 3.13, the one this project's pyproject.toml records for Astro Runtime 3.3-10")

	moved := ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3.1", LockedRuntime: "3.3-8"}
	req, err = ForManifest(moved, catalog)
	require.NoError(t, err)
	assert.NotEqual(t, RuntimeImageRepo+":3.3-8", req.BaseImage, "3.3-8 carries Airflow 3.3.2, which the pin 3.3.1 no longer covers")
	assert.False(t, LockedRuntimeUsed(moved, c))
	notices = LockedRuntimeNotices(moved, c)
	require.Len(t, notices, 1)
	assert.Contains(t, notices[0], "carries Airflow 3.3.2, which the requirement 3.3.1 no longer covers")
}

// A lock's build the image cannot start FROM gives way to the newest build of
// the series only when that one carries the lock's Airflow release. When it
// carries another, the build is refused before it starts, in one message
// naming both builds, both Airflows and why, and no notice says it goes ahead.
func TestALockedBuildFallsBackOnlyToItsAirflow(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
    "3.3-5": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.12"}}
  }}`))
	require.NoError(t, err)
	catalog := func() *runtimeversions.Catalog { return c }

	m := ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3", LockedRuntime: "3.3-5", LockedPython: "3.12", LockedAirflow: "3.3.1+astro.1", RequiresPython: "==3.13.*"}
	_, err = ForManifest(m, catalog)
	var refused *LockedBuildError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "3.3-10", refused.Fallback)
	assert.Contains(t, err.Error(), "uv.lock was made for Astro Runtime 3.3-5 (Airflow 3.3.1+astro.1), and requires-python ==3.13.* excludes Python 3.12")
	assert.Contains(t, err.Error(), "so the image would build on 3.3-10, which has Airflow 3.3.2")
	assert.Contains(t, err.Error(), "Run astro init to move the project to 3.3-10")
	assert.Empty(t, LockedRuntimeNotices(m, c), "no notice that the build goes ahead")

	m.LockedAirflow = "3.3.2+astro.1"
	req, err := ForManifest(m, catalog)
	require.NoError(t, err, "the same release builds on the fallback")
	assert.Equal(t, RuntimeImageRepo+":3.3-10-python-3.13", req.BaseImage)
	assert.Len(t, LockedRuntimeNotices(m, c), 1)
}

// Releases are one when they differ only in a local version, and an unknown
// side is not held against the other.
func TestSameAirflowRelease(t *testing.T) {
	assert.True(t, SameAirflowRelease("3.3.2+astro.1", "3.3.2"))
	assert.True(t, SameAirflowRelease("3.3.2+astro.1", "3.3.2+astro.4"))
	assert.False(t, SameAirflowRelease("3.3.1+astro.1", "3.3.2"))
	assert.True(t, SameAirflowRelease("", "3.3.2"))
}
