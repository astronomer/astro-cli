//go:build !windows

package localstandalone

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/localrt/supervise"
)

// A secret moves off the launch environment only when the environment still
// holds its value; one the engine overrode stays, with the engine's value, as
// it would have in a single environment.
func TestSplitSecretEnvKeepsPrecedence(t *testing.T) {
	const engineBind = "AIRFLOW__API__HOST=127.0.0.1"
	env := []string{
		"PATH=/bin",
		"AIRFLOW_CONN_DB=postgres://u:pw@h/db",
		"AIRFLOW__API__HOST=0.0.0.0",
		engineBind, // a later block, the engine's, wins
	}
	secret := map[string]string{
		"AIRFLOW_CONN_DB":    "postgres://u:pw@h/db",
		"AIRFLOW__API__HOST": "0.0.0.0",
	}
	rest, moved := splitSecretEnv(env, secret)
	assert.Equal(t, map[string]string{"AIRFLOW_CONN_DB": "postgres://u:pw@h/db"}, moved)
	assert.Equal(t, []string{"PATH=/bin", "AIRFLOW__API__HOST=0.0.0.0", engineBind}, rest)

	rest, moved = splitSecretEnv(env, nil)
	assert.Equal(t, env, rest)
	assert.Nil(t, moved)
}

// The wiring: a start with secret values launches the supervisor without them
// in its environment or on its command line, hands them over as the payload,
// and tells the supervisor which descriptor to read.
func TestStartHandsSecretValuesToTheSupervisor(t *testing.T) {
	e, procs, _ := testEngine(t)
	var gotEnv []string
	var gotPayload []byte
	var gotArgs string
	e.launch = func(_ string, env []string, payload []byte, name string, args ...string) (int, error) {
		gotEnv, gotPayload, gotArgs = env, payload, name+" "+strings.Join(args, " ")
		procs.alive[fakePID] = true
		return fakePID, nil
	}
	p := testPlan(t)
	p.SecretEnv = map[string]string{
		"AIRFLOW_CONN_DB":    "postgres://u:pw@h/db",
		"AIRFLOW__API__HOST": "0.0.0.0", // the engine binds loopback over everything
	}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	assert.JSONEq(t, `{"AIRFLOW_CONN_DB":"postgres://u:pw@h/db"}`, string(gotPayload))
	assert.Contains(t, gotArgs, supervise.EnvFDFlag+" "+strconv.Itoa(secretEnvFD))
	assert.NotContains(t, gotArgs, "pw@h")
	for _, kv := range gotEnv {
		assert.NotContains(t, kv, "pw@h", "a secret value reached the supervisor's environment")
	}
	host, _ := envValue(gotEnv, "AIRFLOW__API__HOST")
	assert.Equal(t, "127.0.0.1", host, "a setting the engine keeps must not become the secret's")
	_, plain := envValue(gotEnv, "FOO")
	assert.True(t, plain, "ordinary environment still travels as environment")
}

// A start with no secret values launches exactly as before: no payload, no
// descriptor flag.
func TestStartWithoutSecretsHasNoHandoff(t *testing.T) {
	e, procs, _ := testEngine(t)
	var gotPayload []byte
	var gotArgs string
	e.launch = func(_ string, _ []string, payload []byte, name string, args ...string) (int, error) {
		gotPayload, gotArgs = payload, name+" "+strings.Join(args, " ")
		procs.alive[fakePID] = true
		return fakePID, nil
	}
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.Nil(t, gotPayload)
	assert.NotContains(t, gotArgs, supervise.EnvFDFlag)
}

// The real launch: a child started by launchDetached reads the payload on
// secretEnvFD, to end of file, and the payload is in nobody's environment.
func TestLaunchDetachedHandsThePayloadOnFD3(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "got")
	payload := `{"A":"pa$$ 'w\"o"}`
	script := "cat <&" + strconv.Itoa(secretEnvFD) + " > " + out + ".tmp && mv " + out + ".tmp " + out

	pid, err := launchDetached(dir, []string{"PATH=" + os.Getenv("PATH")}, []byte(payload), "/bin/sh", "-c", script)
	require.NoError(t, err)
	require.Positive(t, pid)

	var got []byte
	require.Eventually(t, func() bool {
		got, err = os.ReadFile(out)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "the child never finished reading the pipe")
	assert.Equal(t, payload, string(got))
}
