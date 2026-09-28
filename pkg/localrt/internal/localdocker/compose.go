package localdocker

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// The compose file is a port of v1's airflow/include templates, thinned for
// v2: ports are always parametrized (v1 hardcoded 8080 in places — an earlier fix),
// everything binds loopback-only, Airflow env comes fully layered from the
// Plan instead of env_file/settings plumbing, and postgres always persists to
// a named volume so a plain stop keeps the database and only --clean drops it.
//
// One template serves both Airflow generations. Everything they share —
// postgres, the one-shot database service, the shape of a component — is
// literal in the template; everything they differ on is resolved here, in Go,
// where it is testable: which components run (airflowServices), what the
// database service does (dbCommand), and the baseline environment
// (generationEnv).
//
//go:embed compose.yaml.tmpl
var composeTemplate string

const (
	// postgresImage matches v1's default metadata database image.
	postgresImage = "docker.io/postgres:12.6"
	// airflowHomeInImage is where the runtime image keeps AIRFLOW_HOME. It is
	// rt's exported constant because a RunInImage caller builds container-side
	// paths from it, and two spellings of the mount root would put a caller's
	// arguments somewhere the mounts are not.
	airflowHomeInImage = rt.ProjectDirInImage
	// devFernetKey is the fixed development-only Fernet key v1 ships in its
	// compose template. Local Airflow is loopback-only and single-user.
	devFernetKey = "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="

	postgresConn = "postgresql://postgres:postgres@postgres:5432" //nolint:gosec // fixed local-dev credentials for the loopback-only metadata DB

	composeFileName = "docker-compose.yaml"

	// airflow2 and airflow3 are the generations this engine runs, spelled the
	// way the state record spells them.
	airflow2 = "2"
	airflow3 = "3"
)

// mountDirs are the project subdirectories mounted into the Airflow
// containers when they exist.
var mountDirs = []string{"dags", "plugins", "include", "tests"}

type envVar struct {
	Name, Value string
}

type mount struct {
	Host, Container string
}

// service is one long-running Airflow component. The generations disagree
// about which ones exist: Airflow 3 splits the API and the dag processor into
// their own components, Airflow 2 serves the API from the webserver and
// processes dags inside the scheduler.
type service struct {
	// Name is both the compose service name and the log component, so
	// `astro local logs --component scheduler` reads the same either way.
	Name string
	// Command is the container's argv.
	Command []string
	// Publish gives this component the published web port. Exactly one
	// service sets it: the one that serves the UI.
	Publish bool
}

// composeInput is the fully resolved data the template renders. Everything
// here derives from the Plan; no config or manifest reads happen at
// generation time.
type composeInput struct {
	ProjectName   string
	Image         string
	PostgresImage string
	// WebPort is the host port the UI and the API are published on — the
	// api-server on Airflow 3, the webserver on Airflow 2. In the container
	// both listen on 8080.
	WebPort      int
	PostgresPort int
	Env          []envVar
	// PassEnv is env-var names rendered with no value, which compose
	// resolves from the CLI's own environment at invocation time. This is
	// how a value satisfied only by the caller's shell reaches the
	// containers without ever being written into the compose file.
	PassEnv []string
	Mounts  []mount
	// DBCommand is the one-shot database service's argv, and Services the
	// components that wait on it.
	DBCommand []string
	Services  []service
}

// airflowServices lists the components to run for an Airflow generation.
func airflowServices(major string) []service {
	if major == airflow2 {
		return []service{
			{Name: "scheduler", Command: []string{"airflow", "scheduler"}},
			{Name: "webserver", Command: []string{"airflow", "webserver"}, Publish: true},
			{Name: "triggerer", Command: []string{"airflow", "triggerer"}},
		}
	}
	return []service{
		{Name: "scheduler", Command: []string{"airflow", "scheduler"}},
		{Name: "dag-processor", Command: []string{"airflow", "dag-processor"}},
		{Name: "api-server", Command: []string{"airflow", "api-server"}, Publish: true},
		{Name: "triggerer", Command: []string{"airflow", "triggerer"}},
	}
}

// dbCommand is what the one-shot database service runs before the components
// start. Airflow 3 only migrates. Airflow 2 also seeds the admin account,
// which has to happen here: the Flask-AppBuilder tables it writes to are
// created by the migration, and the webserver needs the account to exist
// before anyone logs in. The roles come in between — the migration creates the
// tables but leaves them empty, so `users create --role Admin` without
// sync-perm fails with "Admin is not a valid role". Both commands are safe to
// repeat: creating a user that already exists prints so and exits 0.
func dbCommand(major string) []string {
	if major == airflow2 {
		// Only the username and password come from the constant. Deriving the
		// email and firstname from it looked tidier and was worse: the macOS shim
		// hardcodes admin@example.com and "Admin", so a changed username would
		// have made docker mode and the shim create differently-shaped users —
		// manufacturing a divergence the literals do not have. Neither field is
		// used to authenticate.
		createAdmin := fmt.Sprintf(
			"airflow users create --role Admin --username %s --password %s --email admin@example.com --firstname admin --lastname user",
			airflowrt.Airflow2AdminUser, airflowrt.Airflow2AdminPassword)
		return []string{"bash", "-c", "airflow db migrate && airflow sync-perm && " + createAdmin}
	}
	return []string{"airflow", "db", "migrate"}
}

// airflowMajor is the generation a plan's Airflow version names. The version
// arrives validated — imagebuild.LocalRuntimeImageWith refuses anything but
// the two generations before this is asked — so a leading segment is all it
// takes.
func airflowMajor(version string) string {
	major, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	return major
}

// composeProjectName derives the compose project name for a project
// directory: a readable label plus a hash prefix, so identically named
// directories in different paths never collide (same idea as v1's
// ProjectNameUnique, keyed by rt.ProjectID instead of md5).
func composeProjectName(projectPath string) (string, error) {
	// The label comes off the canonical path, not the one the caller typed.
	// SanitizeLabel keeps [a-z0-9-] and drops the rest, and the two Unicode
	// spellings of one name do not survive that the same way: café composed
	// loses the é and becomes "caf", while decomposed it keeps the e and
	// becomes "cafe". One directory would get two compose projects, two
	// postgres volumes and two Airflows — the id being equal is not enough on
	// its own, because the id is only half the name.
	canonical, err := rt.CanonicalPath(projectPath)
	if err != nil {
		return "", err
	}
	id, err := rt.ProjectID(canonical)
	if err != nil {
		return "", err
	}
	label := proxy.SanitizeLabel(filepath.Base(canonical))
	if label == "" {
		return "astro-" + id[:6], nil
	}
	return "astro-" + label + "-" + id[:6], nil
}

// devDefaults start DAGs unpaused with the scheduler's own runs off, as
// internal/localstandalone.devDefaults explains: they run when triggered, not
// on their schedules. A project value from Env or SecretEnv replaces them.
var devDefaults = map[string]string{
	"AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION": "False",
	"AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE":       "False",
}

// airflowEnv layers the plan's environment over the baseline and returns what the
// compose file records. The settings both generations share are here; the rest
// come from generationEnv. secretEnv keys are deliberately absent from the result:
// they are declared through passEnv instead, so the file names them without their
// values. A key in both maps is treated as secret.
func airflowEnv(projectName string, webPort int, major string, planEnv, secretEnv map[string]string) []envVar {
	m := map[string]string{
		"AIRFLOW__CORE__EXECUTOR":             "LocalExecutor",
		"AIRFLOW__CORE__FERNET_KEY":           devFernetKey,
		"AIRFLOW__CORE__LOAD_EXAMPLES":        "False",
		"AIRFLOW__CORE__SQL_ALCHEMY_CONN":     postgresConn,
		"AIRFLOW__DATABASE__SQL_ALCHEMY_CONN": postgresConn,
		"ASTRONOMER_ENVIRONMENT":              "local",
	}
	for k, v := range generationEnv(projectName, webPort, major) {
		m[k] = v
	}
	// Track what the caller may replace — its own values and the dev defaults —
	// so a secret can displace those without displacing the engine's.
	yields := make(map[string]bool, len(devDefaults)+len(planEnv))
	for k, v := range devDefaults {
		m[k] = v
		yields[k] = true
	}
	for k, v := range planEnv {
		m[k] = v
		yields[k] = true
	}
	// A secret is declared, not recorded — but only where the value was the
	// caller's to give. A SecretEnv key colliding with a baseline or
	// generation setting leaves the baseline in place and is simply not applied,
	// which is what standalone does too: there, the engine-critical blocks are
	// appended after the plan and win the same way. Deleting unconditionally
	// would make one plan produce two different Airflow configurations depending
	// on mode, with nothing reporting it.
	for k := range secretEnv {
		if yields[k] {
			delete(m, k)
		}
	}
	out := make([]envVar, 0, len(m))
	for k, v := range m {
		out = append(out, envVar{Name: k, Value: quoteYAML(v)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// generationEnv is the baseline that belongs to one Airflow generation
// alone. Airflow 3 runs the simple auth manager with everyone an admin, and
// its components find each other over the execution API. Airflow 2 has
// neither: it authenticates through Flask-AppBuilder against the account the
// database service creates, and its REST API needs the basic-auth backend
// turned on for a token to be mintable at all —
// internal/localstandalone.af2Env sets the same backends for the same reason.
//
// Both generations build UI links from a base URL, which is why that setting
// carries the host port; in the container Airflow always listens on 8080.
//
// Both also get standalone's DAG rescan intervals, in place of Airflow's
// 30s re-parse and 300s listing defaults, so an edit to a mounted DAG shows
// in seconds.
func generationEnv(projectName string, webPort int, major string) map[string]string {
	baseURL := fmt.Sprintf("http://localhost:%d", webPort)
	if major == airflow2 {
		return map[string]string{
			"AIRFLOW__API__AUTH_BACKENDS":                   "airflow.api.auth.backend.session,airflow.api.auth.backend.basic_auth",
			"AIRFLOW__SCHEDULER__DAG_DIR_LIST_INTERVAL":     "2",
			"AIRFLOW__SCHEDULER__MIN_FILE_PROCESS_INTERVAL": "3",
			"AIRFLOW__WEBSERVER__BASE_URL":                  baseURL,
			"AIRFLOW__WEBSERVER__RBAC":                      "True",
			"AIRFLOW__WEBSERVER__SECRET_KEY":                projectName,
		}
	}
	return map[string]string{
		"AIRFLOW__API__BASE_URL":                            baseURL,
		"AIRFLOW__API__PORT":                                "8080",
		"AIRFLOW__API__SECRET_KEY":                          projectName,
		"AIRFLOW__API_AUTH__JWT_SECRET":                     projectName,
		"AIRFLOW__CORE__AUTH_MANAGER":                       "airflow.api_fastapi.auth.managers.simple.simple_auth_manager.SimpleAuthManager",
		"AIRFLOW__CORE__EXECUTION_API_SERVER_URL":           "http://api-server:8080/execution/",
		"AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS":     "True",
		"AIRFLOW__DAG_PROCESSOR__MIN_FILE_PROCESS_INTERVAL": "3",
		"AIRFLOW__DAG_PROCESSOR__REFRESH_INTERVAL":          "2",
		"AIRFLOW__SCHEDULER__STANDALONE_DAG_PROCESSOR":      "True",
	}
}

// quoteYAML single-quotes a scalar so arbitrary env values cannot change
// the YAML structure. Single-quoted YAML has exactly one escape: a quote
// doubles itself.
func quoteYAML(s string) string {
	// Two escapes, for two different readers of the same bytes.
	//
	// Doubling the quote is YAML's. Doubling the dollar is Compose's: it
	// interpolates the PARSED value, so single quotes do not protect it, and a
	// literal dollar has to arrive as "$$". Without this a password like
	// "se$cret" silently becomes "se" — compose substitutes the unset $cret with
	// nothing and only warns — while the file still reads correctly, and a value
	// containing "${" aborts the start with an interpolation error instead. A
	// dollar in a connection password is common enough that this is the failure
	// this whole field is about.
	//
	// Escaping unconditionally is right because Plan.Env and Plan.SecretEnv are
	// resolved values, never templates: nothing in them is meant to be
	// interpolated by compose.
	escaped := strings.ReplaceAll(s, "$", "$$")
	return "'" + strings.ReplaceAll(escaped, "'", "''") + "'"
}

// passEnv returns the keys the compose file declares with no value: the plan's
// PassthroughEnv, which the runtime's own environment satisfies, plus every
// SecretEnv key, whose value the engine hands to the compose process instead.
//
// Both end up as the same YAML — `KEY:` — because compose resolves a valueless
// entry from its own environment and omits the variable entirely when unset.
//
// A stop or a down needs none of these values, and not for the reason it first
// looks: those commands are run with no --file at all. Down resolves the project
// from container labels, so the declarations are never even read. The valueless
// form would also be harmless if they were, which is the belt to that braces.
func passEnv(names []string, secretEnv map[string]string, env []envVar) []string {
	held := make(map[string]bool, len(env))
	for _, e := range env {
		held[e.Name] = true
	}
	var out []string
	// held doubles as the emitted set: a key already carrying a value on disk is
	// skipped for the same reason a key already declared is — a duplicate YAML key
	// is invalid either way.
	add := func(n string) {
		if held[n] {
			return
		}
		held[n] = true
		out = append(out, n)
	}
	for _, n := range names {
		add(n)
	}
	for n := range secretEnv {
		add(n)
	}
	sort.Strings(out)
	return out
}

// secretEnviron renders SecretEnv as KEY=VALUE for a child process environment.
// Sorted so a command line is reproducible; the compose child reads these to
// resolve the valueless entries passEnv declared.
func secretEnviron(secretEnv map[string]string) []string {
	if len(secretEnv) == 0 {
		return nil
	}
	out := make([]string, 0, len(secretEnv))
	for k, v := range secretEnv {
		// A key that cannot survive KEY=VALUE is dropped rather than rendered.
		// "A=B" would reach the OS as key "A" holding "B=<value>", while the
		// compose file declared "A=B" — so the variable would be declared, never
		// resolved, and Airflow would start silently missing it. An empty key
		// makes the file itself invalid. Neither is worth a half-delivery.
		if k == "" || strings.Contains(k, "=") {
			continue
		}
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// projectMounts lists the project directories to mount into the Airflow
// containers. Only directories that exist are mounted: docker would create
// missing ones on the host as root-owned directories.
func projectMounts(projectPath string) []mount {
	var out []mount
	for _, d := range mountDirs {
		host := filepath.Join(projectPath, d)
		if info, err := os.Stat(host); err == nil && info.IsDir() {
			out = append(out, mount{Host: host, Container: airflowHomeInImage + "/" + d})
		}
	}
	return out
}

// generateCompose renders the compose file for a plan with all ports and
// the image already resolved.
func generateCompose(in composeInput) (string, error) {
	tmpl, err := template.New("compose").Funcs(template.FuncMap{"quote": quoteYAML}).Parse(composeTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing compose template: %w", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, in); err != nil {
		return "", fmt.Errorf("rendering compose file: %w", err)
	}
	return b.String(), nil
}
