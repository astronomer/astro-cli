package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dockerfileProject is a v1 project (an Airflow 2 runtime tag) around a
// manifest the caller supplies.
func dockerfileProject(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM quay.io/astronomer/astro-runtime:11.8.0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600))
	return dir
}

// A field listed in [project] dynamic is supplied by the build backend, and PEP
// 621 forbids stating it statically as well — so writing one turns a buildable
// project into one that errors at build time. ensureProjectVersion has guarded
// this since it was written; this key did not.
func TestAdoptLeavesADynamicRequiresPythonAlone(t *testing.T) {
	dir := dockerfileProject(t, "[project]\nname = 'x'\nversion = '1.0'\ndynamic = [\"requires-python\"]\n")

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "requires-python = ",
		"a dynamic field must not also be stated statically")
	assert.Contains(t, string(raw), "dynamic", "the declaration itself stands")
}

// An author's own requires-python stands — but when it still admits a Python
// the pinned Airflow cannot run, the run says so. Leaving it silent means the
// manifest is in exactly the broken state this work is about, and the first
// `astro local start` reports it from inside werkzeug.
func TestAdoptSaysWhenTheStatedPythonIsTooLooseForAirflowTwo(t *testing.T) {
	dir := dockerfileProject(t, "[project]\nname = 'x'\nversion = '1.0'\nrequires-python = '>=3.11'\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), ">=3.11", "the author's own bound still stands")

	var found string
	for _, n := range res.Notes {
		if strings.Contains(n, "requires-python admits a Python") {
			found = n
		}
	}
	require.NotEmpty(t, found, "expected a note about the loose bound, got: %v", res.Notes)
	assert.Contains(t, found, "<3.13", "the note names the bound the pin needs")
}

// And it stays quiet when the author's bound is already tight enough, so the
// note means something when it does appear.
func TestAdoptSaysNothingWhenTheStatedPythonIsBounded(t *testing.T) {
	dir := dockerfileProject(t, "[project]\nname = 'x'\nversion = '1.0'\nrequires-python = '>=3.10,<3.12'\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	for _, n := range res.Notes {
		assert.NotContains(t, n, "requires-python admits a Python")
	}
}

// Writing it is a change worth reviewing: it decides which interpreters the
// project may ever use. A change performed but unreported cannot be reviewed,
// and the preview a GUI shows is built from these labels.
func TestAdoptReportsTheRequiresPythonItWrote(t *testing.T) {
	dir := dockerfileProject(t, "[project]\nname = 'x'\nversion = '1.0'\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	labels := append(append([]string{}, res.Updated...), res.Created...)
	var found string
	for _, l := range labels {
		if strings.Contains(l, "requires-python") {
			found = l
		}
	}
	require.NotEmpty(t, found, "no label mentions requires-python: %v", labels)
	assert.Contains(t, found, ">=3.10,<3.13", "the label names the bound that was written")
}
