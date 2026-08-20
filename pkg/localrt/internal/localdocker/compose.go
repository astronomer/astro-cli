package localdocker

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// The compose file is a port of v1's airflow/include/airflow3 template,
// thinned for v2: ports are always parametrized (v1 hardcoded 8080 in
// places — an earlier fix), everything binds loopback-only, Airflow env comes
// fully layered from the Plan instead of env_file/settings plumbing, and
// postgres always persists to a named volume so a plain stop keeps the
// database and only --clean drops it.
//
//go:embed compose.yaml.tmpl
var composeTemplate string

const (
	// postgresImage matches v1's default metadata database image.
	postgresImage = "docker.io/postgres:12.6"
	// airflowHomeInImage is where the runtime image keeps AIRFLOW_HOME.
	airflowHomeInImage = "/usr/local/airflow"
	// devFernetKey is the fixed development-only Fernet key v1 ships in its
	// compose template. Local Airflow is loopback-only and single-user.
	devFernetKey = "d6Vefz3G9U_ynXB3cr7y_Ak35tAHkEGAVxuz_B-jzWw="

	postgresConn = "postgresql://postgres:postgres@postgres:5432" //nolint:gosec // fixed local-dev credentials for the loopback-only metadata DB

	composeFileName = "docker-compose.yaml"
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

// composeInput is the fully resolved data the template renders. Everything
// here derives from the Plan; no config or manifest reads happen at
// generation time.
type composeInput struct {
	ProjectName   string
	Image         string
	PostgresImage string
	APIServerPort int
	PostgresPort  int
	Env           []envVar
	// PassEnv is env-var names rendered with no value, which compose
	// resolves from the CLI's own environment at invocation time. This is
	// how a value satisfied only by the caller's shell reaches the
	// containers without ever being written into the compose file.
	PassEnv []string
	Mounts  []mount
}

// composeProjectName derives the compose project name for a project
// directory: a readable label plus a hash prefix, so identically named
// directories in different paths never collide (same idea as v1's
// ProjectNameUnique, keyed by rt.ProjectID instead of md5).
func composeProjectName(projectPath string) (string, error) {
	id, err := rt.ProjectID(projectPath)
	if err != nil {
		return "", err
	}
	label := proxy.SanitizeLabel(filepath.Base(projectPath))
	if label == "" {
		return "astro-" + id[:6], nil
	}
	return "astro-" + label + "-" + id[:6], nil
}

// airflowEnv layers the template's baseline Airflow settings under the
// Plan's fully layered environment, so anything the user sets wins.
func airflowEnv(projectName string, apiServerPort int, planEnv map[string]string) []envVar {
	m := map[string]string{
		// The UI builds links from BASE_URL, so it carries the host port;
		// in-container Airflow always listens on 8080.
		"AIRFLOW__API__BASE_URL":                        fmt.Sprintf("http://localhost:%d", apiServerPort),
		"AIRFLOW__API__PORT":                            "8080",
		"AIRFLOW__API_AUTH__JWT_SECRET":                 projectName,
		"AIRFLOW__API__SECRET_KEY":                      projectName,
		"AIRFLOW__CORE__AUTH_MANAGER":                   "airflow.api_fastapi.auth.managers.simple.simple_auth_manager.SimpleAuthManager",
		"AIRFLOW__CORE__SIMPLE_AUTH_MANAGER_ALL_ADMINS": "True",
		"AIRFLOW__CORE__EXECUTION_API_SERVER_URL":       "http://api-server:8080/execution/",
		"AIRFLOW__CORE__EXECUTOR":                       "LocalExecutor",
		"AIRFLOW__CORE__FERNET_KEY":                     devFernetKey,
		"AIRFLOW__CORE__LOAD_EXAMPLES":                  "False",
		"AIRFLOW__CORE__SQL_ALCHEMY_CONN":               postgresConn,
		"AIRFLOW__DATABASE__SQL_ALCHEMY_CONN":           postgresConn,
		"AIRFLOW__SCHEDULER__STANDALONE_DAG_PROCESSOR":  "True",
		"ASTRONOMER_ENVIRONMENT":                        "local",
	}
	for k, v := range planEnv {
		m[k] = v
	}
	out := make([]envVar, 0, len(m))
	for k, v := range m {
		out = append(out, envVar{Name: k, Value: quoteYAML(v)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// quoteYAML single-quotes a scalar so arbitrary env values cannot change
// the YAML structure. Single-quoted YAML has exactly one escape: a quote
// doubles itself.
func quoteYAML(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// passEnv is the pass-through names to render, sorted, minus any name the
// value-carrying env already holds — a duplicate YAML key would be invalid,
// and a name with a value on disk does not need passing through.
func passEnv(names []string, env []envVar) []string {
	held := make(map[string]bool, len(env))
	for _, e := range env {
		held[e.Name] = true
	}
	var out []string
	for _, n := range names {
		if !held[n] {
			out = append(out, n)
		}
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
