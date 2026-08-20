//go:build !windows

package localstandalone

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
)

// buildEnv constructs the environment for the standalone Airflow process:
// pkg/airflowrt's BuildEnv as the base (inherited env, the project's .env,
// the standalone-critical settings, the macOS proxy workaround), the plan's
// fully layered Env on top, then the version- and platform-specific blocks,
// the per-project JWT secret, and finally the dev-mode overrides — which
// come last so they are authoritative: the loopback bind in particular must
// win over everything (docs/v2-architecture.md, "Defaults").
func (e *Engine) buildEnv(p rt.Plan, projectPath, stateDir, airflowHome string, port int) []string {
	env := airflowrt.BuildEnv(projectPath, strconv.Itoa(port), "")
	// BuildEnv hardcodes AIRFLOW_HOME to the default location; honor the
	// plan's.
	env = setEnv(env, "AIRFLOW_HOME", airflowHome)
	for _, k := range sortedKeys(p.Env) {
		env = setEnv(env, k, p.Env[k])
	}
	if airflowMajor(p.AirflowVersion) == "2" {
		env = append(env, af2Env(port)...)
		// Python's _scproxy calls SCDynamicStoreCopyProxies, which is not
		// fork-safe on macOS and can spin at 100% CPU; NO_PROXY=* tells
		// Python to skip _scproxy entirely. BuildEnv already sets this when
		// no proxy is configured; under AF2's LocalExecutor forking it must
		// hold unconditionally.
		if e.goos == "darwin" {
			env = append(env, "NO_PROXY=*", "no_proxy=*")
		}
	}
	return append(jwtEnv(env, stateDir), devConfigOverrides...)
}

// jwtEnv gap-fills the JWT signing settings AF3's api-server needs set
// before it starts (see jwtSecret). The issuer is pinned too: with
// jwt_issuer unset, Airflow 3.0.x builds tokens with `iss=[]`, which pyjwt
// >= 2.10 rejects — and uv installs from the project's own pyproject with
// no constraint pinning to hide that, so every API request 500s.
func jwtEnv(env []string, stateDir string) []string {
	if !hasKey(env, "AIRFLOW__API_AUTH__JWT_SECRET") {
		env = append(env, "AIRFLOW__API_AUTH__JWT_SECRET="+jwtSecret(stateDir))
	}
	if !hasKey(env, "AIRFLOW__API_AUTH__JWT_ISSUER") {
		env = append(env, "AIRFLOW__API_AUTH__JWT_ISSUER=astro-local")
	}
	return env
}

// af2Env is what Airflow 2 needs beyond BuildEnv (which targets AF3),
// matching desktop and v1's standalone behavior.
func af2Env(port int) []string {
	return []string{
		// AF2's webserver reads WEB_SERVER_PORT, not AF3's API__PORT;
		// without it the webserver sits on 8080 and the health check on the
		// chosen port times out.
		"AIRFLOW__WEBSERVER__WEB_SERVER_PORT=" + strconv.Itoa(port),
		// LocalExecutor lets the scheduler heartbeat while tasks run.
		// SQLite is fine for local dev; the skip-check flag allows it.
		"AIRFLOW__CORE__EXECUTOR=LocalExecutor",
		"_AIRFLOW__SKIP_DATABASE_EXECUTOR_COMPATIBILITY_CHECK=1",
		// Run each task as a fresh subprocess instead of fork/spawn.
		"AIRFLOW__CORE__EXECUTE_TASKS_NEW_PYTHON_INTERPRETER=True",
		// Basic auth on the stable REST API plus `session` for cookie
		// logins through the proxy. AF3 uses /auth/token and ignores this.
		"AIRFLOW__API__AUTH_BACKENDS=airflow.api.auth.backend.session,airflow.api.auth.backend.basic_auth",
	}
}

// devConfigOverrides are the Airflow settings forced for local dev, per the
// design's decision 13: fast DAG rescan yes, zero default task
// retries yes, loopback-only bind always — and DAGs stay paused at
// creation, so no DAGS_ARE_PAUSED_AT_CREATION override here. In Airflow 3
// the dag-processor is a separate component with its own config section, so
// both the scheduler and dag_processor intervals are set; the key for
// whichever Airflow major is not running is ignored.
//
// The api-server (AF3) and webserver (AF2) are pinned to the IPv4 loopback:
// Airflow's own default bind is 0.0.0.0, which would expose the instance —
// effectively unauthenticated under SIMPLE_AUTH_MANAGER_ALL_ADMINS — to
// every device on the LAN.
var devConfigOverrides = []string{
	"AIRFLOW__SCHEDULER__DAG_DIR_LIST_INTERVAL=2",
	"AIRFLOW__SCHEDULER__MIN_FILE_PROCESS_INTERVAL=0",
	"AIRFLOW__DAG_PROCESSOR__DAG_DIR_LIST_INTERVAL=2",
	"AIRFLOW__DAG_PROCESSOR__MIN_FILE_PROCESS_INTERVAL=0",
	"AIRFLOW__CORE__DEFAULT_TASK_RETRIES=0",
	"AIRFLOW__API__HOST=127.0.0.1",
	"AIRFLOW__WEBSERVER__WEB_SERVER_HOST=127.0.0.1",
}

const (
	// jwtSecretFile persists the project's Airflow API JWT signing secret
	// in the project's runtime state dir.
	jwtSecretFile = ".jwt-secret"
	// jwtSecretBytes of randomness, matching desktop's secret size.
	jwtSecretBytes = 16
	// secretFilePerm is owner-only, matching internal/localstate.
	secretFilePerm = 0o600
)

// jwtSecret returns a stable per-project secret for
// AIRFLOW__API_AUTH__JWT_SECRET, generating and persisting one on first
// use. Airflow 3's api-server can generate this itself when missing — but
// only lazily, per process, and its execution-API sub-app requires the
// value already set by the time it initializes ("The value api_auth/
// jwt_secret must be set!"), which crash-loops the api-server forever since
// the same missing value hits every restart. Setting it before Airflow ever
// starts avoids that; persisting it keeps every consumer of the env — the
// running server, `astro local run`, the shell — on the same signing key.
func jwtSecret(stateDir string) string {
	path := filepath.Join(stateDir, jwtSecretFile)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return string(data)
	}

	b := make([]byte, jwtSecretBytes)
	_, _ = rand.Read(b)
	secret := base64.RawURLEncoding.EncodeToString(b)
	_ = os.WriteFile(path, []byte(secret), secretFilePerm) //nolint:errcheck // best-effort persistence; a failed write just regenerates the secret next run
	return secret
}

// shellEnv is the environment for Run and Shell: the same BuildEnv-derived
// env the Airflow process runs with (so commands share its config and
// metadata DB) plus venv activation. BuildEnv already prepends .venv/bin to
// PATH; VIRTUAL_ENV completes the activation. The plan's Env layer is not
// available from a state record — the plan builder (a later issue) will
// carry it — so hand-set env vars reach Run only through the project's
// .env for now.
func (e *Engine) shellEnv(rec localstate.Record) ([]string, error) {
	stateDir, err := rt.StateDir(rec.ProjectPath)
	if err != nil {
		return nil, err
	}
	env := airflowrt.BuildEnv(rec.ProjectPath, strconv.Itoa(rec.Port), "")
	env = append(jwtEnv(env, stateDir), devConfigOverrides...)
	return append(env, "VIRTUAL_ENV="+filepath.Join(rec.ProjectPath, ".venv")), nil
}

// setEnv replaces key's entry in a KEY=VALUE slice, appending when absent.
func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// hasKey reports whether a KEY=VALUE slice defines key.
func hasKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

// sortedKeys orders a map's keys so env construction is deterministic.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
