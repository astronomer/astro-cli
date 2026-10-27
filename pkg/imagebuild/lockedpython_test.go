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
