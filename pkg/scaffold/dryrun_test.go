package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDryRunReportsTheFixAndWritesNothing(t *testing.T) {
	t.Run("AlignRuntime", func(t *testing.T) {
		dir, path := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.2-4"), 0o644)
		before := readFile(t, path)
		opts := AirflowPinOptions{Catalog: runtimeCatalog(t), DryRun: true}

		preview, err := AlignRuntime(dir, nil, opts)
		require.NoError(t, err)
		assert.Equal(t, RuntimeChange{Previous: "3.2-4", Runtime: "3.3-8", Changed: true}, preview)
		assert.Equal(t, before, readFile(t, path), "a dry run writes nothing")

		opts.DryRun = false
		applied, err := AlignRuntime(dir, nil, opts)
		require.NoError(t, err)
		assert.Equal(t, preview, applied, "the preview is what the edit then does")
		assert.NotEqual(t, before, readFile(t, path))
	})
	t.Run("MatchAirflowToDockerfile", func(t *testing.T) {
		dir, path := declaredDockerfileProject(t, "apache-airflow==3.3.*", "FROM astrocrpublic.azurecr.io/runtime:3.2-4\n")
		before := readFile(t, path)

		preview, err := MatchAirflowToDockerfile(dir, nil, AirflowPinOptions{DryRun: true})
		require.NoError(t, err)
		assert.True(t, preview.Changed)
		assert.Equal(t, []string{"apache-airflow==3.2.*"}, preview.Requirements)
		assert.Equal(t, before, readFile(t, path), "a dry run writes nothing")

		applied, err := MatchAirflowToDockerfile(dir, nil, AirflowPinOptions{})
		require.NoError(t, err)
		assert.Equal(t, preview, applied, "the preview is what the edit then does")
		assert.NotEqual(t, before, readFile(t, path))
	})
	t.Run("a refusal is the same in a dry run", func(t *testing.T) {
		dir, path := writeEditFixture(t, withRuntime("apache-airflow>=3.1", "3.3-8"), 0o644)
		before := readFile(t, path)
		_, err := AlignRuntime(dir, nil, AirflowPinOptions{DryRun: true})
		require.ErrorIs(t, err, errAirflowUnclear)
		assert.Equal(t, before, readFile(t, path))
	})
	t.Run("a dry run runs inside the wrapper", func(t *testing.T) {
		dir, _ := writeEditFixture(t, withRuntime("apache-airflow==3.3.*", "3.2-4"), 0o644)
		wrapped := false
		_, err := AlignRuntime(dir, func(run func() error) error { wrapped = true; return run() }, AirflowPinOptions{DryRun: true})
		require.NoError(t, err)
		assert.True(t, wrapped)
	})
}
