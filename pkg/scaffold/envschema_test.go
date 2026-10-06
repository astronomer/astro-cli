package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func project1xDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM quay.io/astronomer/astro-runtime:9\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))
	return dir
}

// A project with nothing to declare gets no [tool.astro.env] at all, rather
// than an empty table — an empty table is a claim, and it would make the app
// treat the manifest as the source before anything declares there.
func TestRunWritesNoEnvSectionWithoutA1xFile(t *testing.T) {
	dir := project1xDir(t)

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "[tool.astro.env")
}

// .astro/env.schema.yaml is not a 1.x file the conversion reads. Only older
// Astro Desktop builds ever wrote it, so it is treated like any file the
// conversion does not know: nothing is carried from it, nothing names it, and
// it is left where it is.
func TestRunIgnoresAnOldDesktopEnvSchema(t *testing.T) {
	dir := project1xDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o700))
	file := filepath.Join(dir, ".astro", "env.schema.yaml")
	body := []byte("env_vars:\n  - { key: API_URL, required: true }\n")
	require.NoError(t, os.WriteFile(file, body, 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	manifestBody, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(manifestBody), "[tool.astro.env")
	assert.NotContains(t, string(manifestBody), "API_URL")
	for _, n := range res.Notes {
		assert.NotContains(t, n, "env.schema.yaml")
	}
	assert.NotContains(t, res.Deleted, ".astro/env.schema.yaml")
	kept, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, body, kept)
}
