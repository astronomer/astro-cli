package checks

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A caller-supplied environment is what the DAGs are imported under.
//
// Without this the parse inherits the process environment, and a consumer whose
// environment is not the user's shell — a GUI that composes the project's own
// from a .env, a vault and declared defaults — would report a DAG that reads a
// variable at module scope as broken while the real Airflow imports it fine.
func TestParseImportsUnderTheSuppliedEnvironment(t *testing.T) {
	ex := &fakeExec{stdout: []byte(`{"schema_version":1,"dags":[],"import_errors":[],"files":[]}`)}
	r := runnerWithPython(t, ex)

	_, err := r.Parse(context.Background(), ParseInput{
		ProjectPath: "proj",
		DagsDir:     "dags",
		Env:         []string{"SNOWFLAKE_ACCOUNT=acme", "PATH=/usr/bin"},
	})
	require.NoError(t, err)

	assert.Contains(t, ex.gotEnv, "SNOWFLAKE_ACCOUNT=acme")
	// And the process environment is NOT mixed in: a caller supplying one is
	// stating what the DAGs should see, not adding to it.
	for _, kv := range ex.gotEnv {
		if strings.HasPrefix(kv, "ASTRO_PARSE_TEST_MARKER=") {
			t.Errorf("the process environment leaked into a supplied one: %q", kv)
		}
	}
}

// The four variables that keep the parse side-effect-free win over a caller
// that tries to set them, so no supplied environment can point it at a real
// Airflow home or turn the examples on.
func TestTheParseOwnedVariablesWinOverTheCaller(t *testing.T) {
	ex := &fakeExec{stdout: []byte(`{"schema_version":1,"dags":[],"import_errors":[],"files":[]}`)}
	r := runnerWithPython(t, ex)

	_, err := r.Parse(context.Background(), ParseInput{
		ProjectPath: "proj",
		DagsDir:     "dags",
		Env: []string{
			"AIRFLOW_HOME=/definitely/not/this",
			"AIRFLOW__CORE__LOAD_EXAMPLES=True",
		},
	})
	require.NoError(t, err)

	// Later entries win in exec's env, so the parse's own must come after.
	lastValue := func(key string) string {
		var out string
		for _, kv := range ex.gotEnv {
			if v, ok := strings.CutPrefix(kv, key+"="); ok {
				out = v
			}
		}
		return out
	}
	assert.NotEqual(t, "/definitely/not/this", lastValue("AIRFLOW_HOME"))
	assert.Equal(t, "False", lastValue("AIRFLOW__CORE__LOAD_EXAMPLES"))
}

// An empty Env inherits the process environment, which is what the CLI wants:
// it runs from the project directory and its environment is the user's shell.
func TestAnEmptyEnvInheritsTheProcessEnvironment(t *testing.T) {
	t.Setenv("ASTRO_PARSE_TEST_MARKER", "inherited")

	ex := &fakeExec{stdout: []byte(`{"schema_version":1,"dags":[],"import_errors":[],"files":[]}`)}
	r := runnerWithPython(t, ex)

	_, err := r.Parse(context.Background(), ParseInput{ProjectPath: "proj", DagsDir: "dags"})
	require.NoError(t, err)
	assert.True(t, slices.Contains(ex.gotEnv, "ASTRO_PARSE_TEST_MARKER=inherited"),
		"an empty Env must inherit the process environment")
}
