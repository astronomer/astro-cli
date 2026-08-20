//go:build !windows

package localstandalone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
)

// envValue returns the value the process would see for key: the last
// occurrence wins, matching how the exec'd child resolves duplicates.
func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	value, found := "", false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			value, found = kv[len(prefix):], true
		}
	}
	return value, found
}

func buildTestEnv(t *testing.T, goos, airflowVersion string, planEnv map[string]string) []string {
	t.Helper()
	e, _, _ := testEngine(t)
	e.goos = goos
	project := t.TempDir()
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone, AirflowVersion: airflowVersion, Env: planEnv}
	return e.buildEnv(p, project, t.TempDir(), filepath.Join(project, ".astro", "standalone"), 8123)
}

func TestBuildEnvDevOverridesAreAuthoritative(t *testing.T) {
	env := buildTestEnv(t, "linux", "3.0.2", map[string]string{
		// The plan must not be able to unbind the loopback pin.
		"AIRFLOW__API__HOST": "0.0.0.0",
		"FOO":                "bar",
	})

	// Loopback pins always win (decision: loopback-only bind always).
	host, _ := envValue(env, "AIRFLOW__API__HOST")
	assert.Equal(t, "127.0.0.1", host)
	web, _ := envValue(env, "AIRFLOW__WEBSERVER__WEB_SERVER_HOST")
	assert.Equal(t, "127.0.0.1", web)

	// Fast rescan and zero retries are on; paused-at-creation stays at
	// Airflow's default (decision 13).
	rescan, _ := envValue(env, "AIRFLOW__SCHEDULER__DAG_DIR_LIST_INTERVAL")
	assert.Equal(t, "2", rescan)
	retries, _ := envValue(env, "AIRFLOW__CORE__DEFAULT_TASK_RETRIES")
	assert.Equal(t, "0", retries)
	_, found := envValue(env, "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION")
	assert.False(t, found)

	// The plan's other values layer over the base.
	foo, _ := envValue(env, "FOO")
	assert.Equal(t, "bar", foo)
}

func TestBuildEnvAF2Block(t *testing.T) {
	env := buildTestEnv(t, "linux", "2.10.5", nil)
	port, _ := envValue(env, "AIRFLOW__WEBSERVER__WEB_SERVER_PORT")
	assert.Equal(t, "8123", port)
	executor, _ := envValue(env, "AIRFLOW__CORE__EXECUTOR")
	assert.Equal(t, "LocalExecutor", executor)
	_, skip := envValue(env, "_AIRFLOW__SKIP_DATABASE_EXECUTOR_COMPATIBILITY_CHECK")
	assert.True(t, skip)

	// AF3 gets none of it.
	env = buildTestEnv(t, "linux", "3.0.2", nil)
	_, found := envValue(env, "AIRFLOW__WEBSERVER__WEB_SERVER_PORT")
	assert.False(t, found)
	_, found = envValue(env, "AIRFLOW__CORE__EXECUTOR")
	assert.False(t, found)

	// AF2 on darwin forces the fork-safe proxy skip even when a proxy is
	// configured (which makes BuildEnv leave NO_PROXY alone).
	t.Setenv("HTTPS_PROXY", "http://corp:3128")
	env = buildTestEnv(t, "darwin", "2.10.5", nil)
	noProxy, _ := envValue(env, "NO_PROXY")
	assert.Equal(t, "*", noProxy)
}

func TestBuildEnvJWTSecretIsStablePerProject(t *testing.T) {
	// AF3's api-server crash-loops when api_auth/jwt_secret is unset by the
	// time its execution API initializes; the engine sets a persisted
	// secret before Airflow ever starts.
	e, _, _ := testEngine(t)
	project := t.TempDir()
	stateDir := t.TempDir()
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone}

	env1 := e.buildEnv(p, project, stateDir, filepath.Join(project, ".astro"), 8080)
	env2 := e.buildEnv(p, project, stateDir, filepath.Join(project, ".astro"), 8080)
	s1, found := envValue(env1, "AIRFLOW__API_AUTH__JWT_SECRET")
	require.True(t, found)
	require.NotEmpty(t, s1)
	s2, _ := envValue(env2, "AIRFLOW__API_AUTH__JWT_SECRET")
	assert.Equal(t, s1, s2)

	// A plan-supplied secret wins over generation.
	p.Env = map[string]string{"AIRFLOW__API_AUTH__JWT_SECRET": "pinned"}
	env3 := e.buildEnv(p, project, stateDir, filepath.Join(project, ".astro"), 8080)
	s3, _ := envValue(env3, "AIRFLOW__API_AUTH__JWT_SECRET")
	assert.Equal(t, "pinned", s3)

	// The issuer is pinned too: with jwt_issuer unset, Airflow 3.0.x
	// builds `iss=[]` tokens that pyjwt >= 2.10 rejects, and uv installs
	// without the constraint pinning that hides it — every request 500s.
	iss, found := envValue(env1, "AIRFLOW__API_AUTH__JWT_ISSUER")
	require.True(t, found)
	assert.Equal(t, "astro-local", iss)
}

func TestBuildEnvHonorsPlanAirflowHome(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	home := filepath.Join(project, "custom-home")
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone, AirflowHome: home}
	env := e.buildEnv(p, project, t.TempDir(), home, 8080)
	got, _ := envValue(env, "AIRFLOW_HOME")
	assert.Equal(t, home, got)
}

func TestBuildEnvReadsProjectDotEnv(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, ".env"), []byte("FROM_DOTENV=yes\n"), 0o600))
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone}
	env := e.buildEnv(p, project, t.TempDir(), filepath.Join(project, ".astro"), 8080)
	got, _ := envValue(env, "FROM_DOTENV")
	assert.Equal(t, "yes", got)
}
