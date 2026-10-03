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
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// goosDarwin names the platform whose _scproxy fork-safety workaround the
// AF2 blocks below depend on.
const goosDarwin = "darwin"

// buildEnv constructs the environment for the standalone Airflow process:
// pkg/airflowrt's BuildEnv as the base (inherited env, the project's .env,
// the standalone-critical settings, the macOS proxy workaround), the plan's
// fully layered Env on top, then the version- and platform-specific blocks,
// the per-project JWT secret, and finally the dev-mode settings: defaults
// wherever the environment so far left a key unset, and the loopback bind
// over everything (docs/v2-architecture.md, "Defaults").
//
// The engine adds no AIRFLOW__CORE__FERNET_KEY of its own, deliberately. When
// no key is configured the first time Airflow (2 or 3) is imported under
// AIRFLOW_HOME, it generates one and writes it into airflow.cfg there,
// owner-only, so connections and Variables in the metadata DB beside it are
// encrypted, and every later process run with the same AIRFLOW_HOME decrypts
// them with the same key. An environment value outranks airflow.cfg, so adding
// one here would orphan every row already encrypted under the airflow.cfg key.
// A key the user supplies (shell, .env or the plan) passes through like any
// other value; keeping it stable is then theirs to do.
func (e *Engine) buildEnv(p rt.Plan, projectPath, stateDir, airflowHome string, port int) []string {
	env := airflowrt.BuildEnv(projectPath, strconv.Itoa(port), "")
	// BuildEnv hardcodes AIRFLOW_HOME to the default location; honor the
	// plan's.
	env = setEnv(env, "AIRFLOW_HOME", airflowHome)
	for _, k := range sortedKeys(p.Env) {
		env = setEnv(env, k, p.Env[k])
	}
	// SecretEnv is ordinary environment here. The distinction exists because
	// docker mode persists a compose file and standalone persists nothing, so
	// there is no second treatment for this engine to give it — and applying it
	// after Env means a key in both resolves the way the contract says, secret
	// wins.
	for _, k := range sortedKeys(p.SecretEnv) {
		env = setEnv(env, k, p.SecretEnv[k])
	}
	if airflowMajor(p.AirflowVersion) == "2" {
		env = append(env, af2Env(port)...)
		// Python's _scproxy calls SCDynamicStoreCopyProxies, which is not
		// fork-safe on macOS and can spin at 100% CPU; NO_PROXY=* tells
		// Python to skip _scproxy entirely. BuildEnv already sets this when
		// no proxy is configured; under AF2's LocalExecutor forking it must
		// hold unconditionally.
		if e.goos == goosDarwin {
			env = append(env, "NO_PROXY=*", "no_proxy=*")
		}
	}
	return devEnv(jwtEnv(env, stateDir))
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

// devEnv gap-fills devDefaults, so a project that sets one of those keys
// itself keeps its value, and then pins loopbackBind, which nothing overrides.
func devEnv(env []string) []string {
	for _, kv := range devDefaults {
		if key, _, _ := strings.Cut(kv, "="); !hasKey(env, key) {
			env = append(env, kv)
		}
	}
	return append(env, loopbackBind...)
}

// devDefaults are the Airflow settings local dev runs with unless the project
// or the shell sets them, per the design: fast DAG rescan and zero
// default task retries. In Airflow 3 the dag-processor is a separate
// component with its own config section, where dag_dir_list_interval is named
// refresh_interval, so both the scheduler and dag_processor intervals are set.
//
// The per-file interval is 3s rather than 0: at 0 the dag-processor re-parses
// every file in a tight loop and holds about 45% of a core on an idle
// one-DAG project; at 3 it idles at 1-2% and a saved change shows within ~5s.
//
// DAGs are created UNPAUSED but the scheduler creates no runs of its own,
// which reverses what decision 13 originally said; docs/v2-architecture.md
// carries the amended wording.
//
// Paused-at-creation is a deployment default. It stops a DAG that lands on a
// shared scheduler from running before anyone has looked at it. Locally there
// is nothing to protect from: the person who wrote the file is watching the
// UI, started Airflow themselves, and is waiting to see it run. Left paused,
// a run they trigger from the UI or the API sits queued behind a toggle
// nothing pointed at. Astro Desktop has forced False for its whole life for
// that reason, so this is also what local users already have.
//
// With use_job_schedule off, nothing runs on a cron clock and asset-triggered
// DAGs do not cascade, while a triggered run still starts at once. Left on, a
// repo's schedules queue runs, catchup included, against whatever its DAGs
// talk to the moment Airflow starts; local dev is for running what you
// trigger. Astro Desktop leaves schedules on today, so its users lose them
// once it provisions through this engine.
var devDefaults = []string{
	"AIRFLOW__SCHEDULER__DAG_DIR_LIST_INTERVAL=2",
	"AIRFLOW__SCHEDULER__MIN_FILE_PROCESS_INTERVAL=3",
	"AIRFLOW__DAG_PROCESSOR__REFRESH_INTERVAL=2",
	"AIRFLOW__DAG_PROCESSOR__MIN_FILE_PROCESS_INTERVAL=3",
	"AIRFLOW__CORE__DEFAULT_TASK_RETRIES=0",
	"AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION=False",
	"AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE=False",
	// The log file is not a terminal, so nothing in it should be colored.
	// `airflow standalone` colors the component name it prefixes each line
	// with and structlog colors the body, which costs about forty wasted bytes
	// a line of a CAPPED file — roughly a third of the retained history — and
	// leaves every reader to strip escapes back out. NO_COLOR is the
	// cross-tool convention; the Airflow setting is the one that governs its
	// own console handler.
	"NO_COLOR=1",
	"AIRFLOW__LOGGING__COLORED_CONSOLE_LOG=False",
}

// loopbackBind pins the api-server (AF3) and webserver (AF2) to the IPv4
// loopback whatever the project sets. It is a security guard, not a
// preference: Airflow's own default bind is 0.0.0.0, which would expose the
// instance — effectively unauthenticated under SIMPLE_AUTH_MANAGER_ALL_ADMINS
// — to every device on the LAN.
var loopbackBind = []string{
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

// shellEnv is the environment for Run, Shell and Env: the same
// BuildEnv-derived env the Airflow process runs with (so commands share its
// config and metadata DB) plus venv activation. BuildEnv already prepends
// .venv/bin to PATH; VIRTUAL_ENV completes the activation.
//
// Rebuilt from the state record rather than the plan, because the plan is
// gone by the time anyone asks. What the record cannot carry is therefore
// missing, and the gaps are real rather than theoretical:
//
//   - Plan.Env and Plan.SecretEnv. Hand-set variables reach a command only
//     through the project's .env. A project whose plan supplies a
//     SQL_ALCHEMY_CONN or a FERNET_KEY gets a command talking to a different
//     metadata DB, or unable to decrypt connections.
//   - Plan.StateDir and Plan.AirflowHome. Both fall back to the canonical
//     locations, so an embedder that relocates either gets a different
//     AIRFLOW_HOME and a JWT secret the running api-server will reject.
//
// Closing those needs the record extended to carry them, which is a change
// to what a start persists rather than to this function.
// TODO(localrt): carry the plan's env layer and relocations on the record.
//
// The generation-specific block IS recoverable, because the record names it,
// and leaving it out was a live divergence: an AF2 project's `airflow tasks
// test` ran under SequentialExecutor with the REST auth backends unset, and
// on macOS without the _scproxy fork-safety workaround AF2 needs.
func (e *Engine) shellEnv(rec localstate.Record) ([]string, error) {
	stateDir, err := rt.StateDir(rec.ProjectPath)
	if err != nil {
		return nil, err
	}
	env := airflowrt.BuildEnv(rec.ProjectPath, strconv.Itoa(rec.Port), "")
	if rec.AirflowMajor == "2" {
		env = append(env, af2Env(rec.Port)...)
		if e.goos == goosDarwin {
			env = append(env, "NO_PROXY=*", "no_proxy=*")
		}
	}
	env = devEnv(jwtEnv(env, stateDir))
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
