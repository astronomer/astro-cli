package imageinstall

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

const lockedAt332 = `lock-version = "1.0"
[[packages]]
name = "apache-airflow"
version = "3.3.2+astro.1"
`

const recorded = `[project]
name = "p"
dependencies = ["apache-airflow==3.3.*"]

[tool.astro]

[tool.uv]
constraint-dependencies = [
    # Astro Runtime 3.3-11 (Python 3.12) constrains these.
    "sqlalchemy==1.4.54",
    "apache-airflow==3.3.2+astro.1",
]
`

// A lock for another Airflow release than the image's is refused before the
// build, naming both ways out; the same release, a local version aside,
// passes, and so does an image whose Airflow is not known.
func TestAirflowMismatch(t *testing.T) {
	err := airflowMismatch([]byte(lockedAt332), []byte(recorded), []string{"apache-airflow==3.1.7+astro.1"}, nil, "3.1-12", "img:3.1-12-base")
	require.Error(t, err)
	assert.Equal(t, "uv.lock was made for Airflow 3.3.2 (Astro Runtime 3.3-11), but the image builds on Astro Runtime 3.1-12, which has Airflow 3.1.7. "+
		"To keep the lock's Airflow, run astro local upgrade runtime 3.3-11. To use the image's, run astro init, which relocks uv.lock", err.Error())

	assert.NoError(t, airflowMismatch([]byte(lockedAt332), []byte(recorded), []string{"apache-airflow==3.3.2+astro.4"}, nil, "3.3-12", ""))
	assert.NoError(t, airflowMismatch([]byte(lockedAt332), []byte(recorded), nil, nil, "", "img"), "unknown image Airflow")
}

// Without a freeze, the catalog names the image's Airflow; without a record,
// the message says what to pick.
func TestAirflowMismatchFromTheCatalog(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
		"3.1-12": {"metadata": {"airflowVersion": "3.1.7", "channel": "stable", "releaseDate": "2025-06-01", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}}}}`))
	require.NoError(t, err)
	err = airflowMismatch([]byte(lockedAt332), []byte("[project]\nname = \"p\"\n"), nil, c, "3.1-12", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uv.lock was made for Airflow 3.3.2, but the image builds on Astro Runtime 3.1-12, which has Airflow 3.1.7. "+
		"To keep the lock's Airflow, pick a runtime version with Airflow 3.3.2 (astro local upgrade runtime <version>)")
}
