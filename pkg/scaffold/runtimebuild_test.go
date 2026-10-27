package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// A patch pin resolves to the newest build carrying that release, which can
// be older than the series' newest and ship other Pythons: the Python is the
// resolved build's, not the newest build's.
func TestRuntimeBuildForTakesThePythonOfTheResolvedBuild(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
    "3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}},
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-02-01", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}}
  }}`))
	require.NoError(t, err)
	m, err := manifest.Parse([]byte("[project]\nname = 'demo'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.1']\n\n[tool.astro]\n"))
	require.NoError(t, err)

	build, python, err := RuntimeBuildFor(t.TempDir(), m, c)
	require.NoError(t, err)
	assert.Equal(t, "3.3-7", build)
	assert.Equal(t, "3.13", python, "3.3-7's default, not 3.3-9's 3.14")
}
