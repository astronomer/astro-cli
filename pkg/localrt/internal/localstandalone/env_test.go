//go:build !windows

package localstandalone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
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
	unsetDevDefaults(t)
	e, _, _ := testEngine(t)
	e.goos = goos
	project := t.TempDir()
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone, AirflowVersion: airflowVersion, Env: planEnv}
	return e.buildEnv(p, project, t.TempDir(), filepath.Join(project, ".astro", "standalone"), 8123)
}

func unsetDevDefaults(t *testing.T) {
	t.Helper()
	for _, kv := range devDefaults {
		key, _, _ := strings.Cut(kv, "=")
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func TestBuildEnvAppliesTheDevSettings(t *testing.T) {
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

	// Fast rescan and zero retries are on, and DAGs are created unpaused with
	// the scheduler's own runs off: a local DAG runs when the person who just
	// wrote it triggers it, and not on its schedule.
	rescan, _ := envValue(env, "AIRFLOW__SCHEDULER__DAG_DIR_LIST_INTERVAL")
	assert.Equal(t, "2", rescan)
	refresh, _ := envValue(env, "AIRFLOW__DAG_PROCESSOR__REFRESH_INTERVAL")
	assert.Equal(t, "2", refresh)
	perFile, _ := envValue(env, "AIRFLOW__DAG_PROCESSOR__MIN_FILE_PROCESS_INTERVAL")
	assert.Equal(t, "3", perFile)
	af2PerFile, _ := envValue(env, "AIRFLOW__SCHEDULER__MIN_FILE_PROCESS_INTERVAL")
	assert.Equal(t, "3", af2PerFile)
	retries, _ := envValue(env, "AIRFLOW__CORE__DEFAULT_TASK_RETRIES")
	assert.Equal(t, "0", retries)
	paused, found := envValue(env, "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION")
	assert.True(t, found, "a local start should decide this rather than inherit Airflow's deployment default")
	assert.Equal(t, "False", paused)
	schedule, _ := envValue(env, "AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE")
	assert.Equal(t, "False", schedule)

	// Color off, because the destination is a capped file rather than a
	// terminal. Left on, `airflow standalone` colors the component name it
	// prefixes each line with, which spends a third of the retained history on
	// escape sequences and leaves every reader stripping them back out.
	noColor, _ := envValue(env, "NO_COLOR")
	assert.Equal(t, "1", noColor)
	colored, _ := envValue(env, "AIRFLOW__LOGGING__COLORED_CONSOLE_LOG")
	assert.Equal(t, "False", colored)

	// The plan's other values layer over the base.
	foo, _ := envValue(env, "FOO")
	assert.Equal(t, "bar", foo)
}

// A project that sets a dev default itself keeps its own value, from its .env
// or from the plan. The loopback bind is the exception: it guards the
// instance, so no project setting reaches it.
func TestBuildEnvLetsTheProjectOverrideDevDefaults(t *testing.T) {
	unsetDevDefaults(t)
	e, _, _ := testEngine(t)
	project := t.TempDir()
	dotenv := "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION=True\nAIRFLOW__SCHEDULER__USE_JOB_SCHEDULE=True\nAIRFLOW__WEBSERVER__WEB_SERVER_HOST=0.0.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(project, ".env"), []byte(dotenv), 0o600))
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone, AirflowVersion: "3.0.2", Env: map[string]string{
		"AIRFLOW__CORE__DEFAULT_TASK_RETRIES": "2",
	}}
	env := e.buildEnv(p, project, t.TempDir(), filepath.Join(project, ".astro"), 8080)

	paused, _ := envValue(env, "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION")
	assert.Equal(t, "True", paused)
	schedule, _ := envValue(env, "AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE")
	assert.Equal(t, "True", schedule)
	retries, _ := envValue(env, "AIRFLOW__CORE__DEFAULT_TASK_RETRIES")
	assert.Equal(t, "2", retries)
	rescan, _ := envValue(env, "AIRFLOW__SCHEDULER__DAG_DIR_LIST_INTERVAL")
	assert.Equal(t, "2", rescan, "a default the project does not set still applies")

	web, _ := envValue(env, "AIRFLOW__WEBSERVER__WEB_SERVER_HOST")
	assert.Equal(t, "127.0.0.1", web)
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

// The JWT secret signs every API token, so it is owner-only, like the
// airflow.cfg Airflow writes into AIRFLOW_HOME.
func TestBuildEnvWritesTheJWTSecretOwnerOnly(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	stateDir := t.TempDir()
	p := rt.Plan{ProjectPath: project, Mode: rt.ModeStandalone}
	e.buildEnv(p, project, stateDir, filepath.Join(project, ".astro"), 8080)

	info, err := os.Stat(filepath.Join(stateDir, jwtSecretFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// The engine adds no Fernet key of its own. Airflow keeps the project's key in
// airflow.cfg under AIRFLOW_HOME, and an environment value would outrank it and
// orphan every row encrypted with it. A key the user supplies is not covered
// here: it passes through like any other value.
func TestBuildEnvAddsNoFernetKeyOfItsOwn(t *testing.T) {
	t.Setenv("AIRFLOW__CORE__FERNET_KEY", "")
	require.NoError(t, os.Unsetenv("AIRFLOW__CORE__FERNET_KEY"))

	for _, version := range []string{"2.10.5", "3.0.2"} {
		t.Run(version, func(t *testing.T) {
			env := buildTestEnv(t, "linux", version, nil)
			_, found := envValue(env, "AIRFLOW__CORE__FERNET_KEY")
			assert.False(t, found, "buildEnv set AIRFLOW__CORE__FERNET_KEY, which overrides airflow.cfg")
		})
	}
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
