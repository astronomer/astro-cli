package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A conversion that keeps the Dockerfile as the build refuses to write a
// project whose requirement disagrees with that file's FROM, which every run
// path would then refuse. Only a version from somewhere other than the FROM can
// get there: --airflow-version, or the pin an adopted manifest already has.

const buildingDockerfile = "FROM astrocrpublic.azurecr.io/runtime:3.1-12\nRUN pip install --no-cache-dir unixodbc\n"

func isDockerfileMismatch(err error) bool {
	var ve *manifest.ValidationError
	return errors.As(err, &ve) && len(ve.Problems) == 1 && ve.Problems[0].Code == manifest.CodeDockerfileAirflowMismatch
}

func TestConversionRefusesAFlagTheKeptDockerfileDisagreesWith(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(buildingDockerfile), 0o600))

	_, err := Run(dir, Options{AirflowVersion: "3.3"})
	require.True(t, isDockerfileMismatch(err), "err = %v", err)
	assert.Contains(t, err.Error(), "--airflow-version 3.3")
	assert.Contains(t, err.Error(), "runtime:3.1-12")
	assert.NoFileExists(t, filepath.Join(dir, "pyproject.toml"), "nothing is written")

	// A flag of the FROM's own series converts.
	_, err = Run(dir, Options{AirflowVersion: "3.1.5"})
	require.NoError(t, err)
}

// A Dockerfile that only names a base is retired rather than kept, so the flag
// decides the project alone and nothing disagrees.
func TestConversionLetsAFlagReplaceAPinOnlyDockerfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"), 0o600))
	_, err := Run(dir, Options{AirflowVersion: "3.3"})
	require.NoError(t, err)
}

func TestAdoptRefusesAManifestPinTheKeptDockerfileDisagreesWith(t *testing.T) {
	dir := adoptable(t, "'apache-airflow==3.3.*'")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(buildingDockerfile), 0o600))
	before, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)

	_, err = Run(dir, Options{})
	require.True(t, isDockerfileMismatch(err), "err = %v", err)
	assert.Contains(t, err.Error(), "apache-airflow==3.3.* already in pyproject.toml")
	after, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the manifest is left as it was")

	// The flag can repair it, naming the FROM's series.
	_, err = Run(dir, Options{AirflowVersion: "3.1"})
	require.NoError(t, err)

	// And a flag that disagrees is refused, naming the flag.
	dir = adoptable(t, "'apache-airflow==3.1.*'")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(buildingDockerfile), 0o600))
	_, err = Run(dir, Options{AirflowVersion: "3.3"})
	require.True(t, isDockerfileMismatch(err), "err = %v", err)
	assert.Contains(t, err.Error(), "--airflow-version 3.3")
}
