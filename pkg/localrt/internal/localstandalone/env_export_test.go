//go:build !windows

package localstandalone

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Env is what an embedder running its own process in the project's
// environment gets — a terminal, where the user's shell is never the
// engine's to run.
//
// Asserted as EQUAL to what Run hands the commander, which is the only claim
// worth making: a terminal whose environment merely resembles the running
// Airflow's is the failure this exists to prevent. Spot-checking PATH and
// AIRFLOW_HOME is what the first version did, and it survived replacing the
// whole body with a bare BuildEnv call — no JWT secret, no dev overrides, no
// VIRTUAL_ENV.
func TestEnvIsExactlyWhatRunUses(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        rt.ModeStandalone,
		Port:        8080,
		Hostname:    filepath.Base(project) + ".localhost",
	}))

	var runEnv []string
	e.cmd = commanderFunc(func(_ context.Context, _ string, env []string, _ rt.Stdio, _ string, _ ...string) error {
		runEnv = env
		return nil
	})

	af, err := e.Attach(project)
	require.NoError(t, err)

	require.NoError(t, af.Run(context.Background(), []string{"airflow", "version"}, rt.Stdio{}))
	require.NotEmpty(t, runEnv)

	env, err := af.Env()
	require.NoError(t, err)
	// As a set: BuildEnv is not order-stable between calls, and os/exec does
	// not care — there are no duplicate keys for a last-wins rule to bite on.
	require.ElementsMatch(t, runEnv, env, "a terminal would run in a different environment from the engine's own commands")

	// And the thing that makes it the project's rather than the user's.
	var path string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == "PATH" {
			path = v
		}
	}
	assert.True(t, strings.HasPrefix(path, filepath.Join(project, ".venv", "bin")+":"),
		"PATH = %q, want the project venv's bin first", path)
}

// The record names the generation, so leaving its settings out of Env was a
// live divergence rather than a limit: an AF2 project's `airflow tasks test`
// ran under SequentialExecutor with the REST auth backends unset, differing
// from the scheduler it is supposed to share an environment with.
func TestEnvCarriesTheGenerationSpecificSettings(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath:  project,
		Mode:         rt.ModeStandalone,
		Port:         8080,
		AirflowMajor: "2",
		Hostname:     filepath.Base(project) + ".localhost",
	}))

	af, err := e.Attach(project)
	require.NoError(t, err)
	env, err := af.Env()
	require.NoError(t, err)

	joined := strings.Join(env, "\n")
	assert.Contains(t, joined, "AIRFLOW__CORE__EXECUTOR=LocalExecutor",
		"an AF2 command would run under SequentialExecutor, not the executor its scheduler uses")
	assert.Contains(t, joined, "AIRFLOW__WEBSERVER__WEB_SERVER_PORT=8080")
}
