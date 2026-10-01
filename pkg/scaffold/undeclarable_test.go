package scaffold

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

func TestCheckUndeclarable(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)
	before := readFile(t, path)

	require.NoError(t, CheckUndeclarable(dir, envschema.SectionEnvVar, "LOG_LEVEL"))
	// Found the way the writer finds it: an env-form key folds.
	require.NoError(t, CheckUndeclarable(dir, envschema.SectionAirflowVariable, "AIRFLOW_VAR_REGION"))

	err := CheckUndeclarable(dir, envschema.SectionEnvVar, "NOPE")
	require.ErrorIs(t, err, ErrNotDeclared)
	assert.NotErrorIs(t, err, ErrManifestUnloadable)
	assert.Equal(t, "NOPE is not declared in "+path, err.Error())

	// A name declared in another section is not declared in this one.
	require.ErrorIs(t, CheckUndeclarable(dir, envschema.SectionConnection, "LOG_LEVEL"), ErrNotDeclared)

	assert.Equal(t, before, readFile(t, path), "the check wrote the manifest")
}

func TestCheckUndeclarableRefusesAManifestThatDoesNotLoad(t *testing.T) {
	for name, body := range map[string]string{
		"toml":   "[project]\nname = 'x'\n\n[tool.astro.env\nAPI_URL = {}\n",
		"schema": unparseableEnv,
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := writeEditFixture(t, body, 0o644)
			err := CheckUndeclarable(dir, envschema.SectionEnvVar, "API_URL")
			require.ErrorIs(t, err, ErrManifestUnloadable)
			assert.NotErrorIs(t, err, ErrNotDeclared)
			// The text is the load or parse error's own.
			assert.NotContains(t, err.Error(), ErrManifestUnloadable.Error())
			assert.NotEmpty(t, err.Error())
			assert.Equal(t, body, readFile(t, path))
		})
	}
}

// unparseableEnv loads as a manifest; only its [tool.astro.env] is refused.
const unparseableEnv = "[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n" +
	"[tool.astro]\n\n[tool.astro.env]\nAPI_URL = { type = 'nope' }\n"

// The env section's parse refuses it, with the parse error's own text.
func TestCheckUndeclarableRefusesAnUnparseableEnvSection(t *testing.T) {
	dir, path := writeEditFixture(t, unparseableEnv, 0o644)
	m, err := manifest.Load(path)
	require.NoError(t, err, "the fixture must load, or the parse is not what refuses it")
	_, parseErr := envschema.ParseSchema(m.Astro.Env)
	require.Error(t, parseErr)

	err = CheckUndeclarable(dir, envschema.SectionEnvVar, "API_URL")
	require.ErrorIs(t, err, ErrManifestUnloadable)
	assert.Equal(t, parseErr.Error(), err.Error())
}

func TestCheckUndeclarableWithNoManifest(t *testing.T) {
	dir := t.TempDir()
	err := CheckUndeclarable(dir, envschema.SectionEnvVar, "API_URL")
	require.ErrorIs(t, err, ErrManifestUnloadable)
	assert.True(t, errors.Is(err, manifest.ErrNotFound), "%v", err)
	// The text is Load's own.
	_, loadErr := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.Error(t, loadErr)
	assert.Equal(t, loadErr.Error(), err.Error())
}
