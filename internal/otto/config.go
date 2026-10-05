package otto

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/manifest"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// Config holds the environment configuration for spawning Otto.
type Config struct {
	Token        string
	Domain       string
	Organization string
	AirflowURL   string
	// AirflowV2 is true when AirflowURL is a project's Airflow, which Otto
	// authenticates to itself: no username and password are injected for it.
	AirflowV2 bool
}

// NewConfigFromContext builds a Config from the current astro login context.
// AirflowURL stays empty here: detection costs health-probe round trips, so
// Start fills it in only after the launch is past the gates that would
// discard it (not logged in, --help, --version).
func NewConfigFromContext() *Config {
	cfg := &Config{}

	ctx, err := config.GetCurrentContext()
	if err == nil {
		cfg.Token = strings.TrimPrefix(ctx.Token, "Bearer ")
		cfg.Domain = ctx.Domain
		cfg.Organization = projectOrganization(ctx.Organization)
	}

	return cfg
}

// projectOrganization is the organization Otto runs in: the one the current
// directory's project names for its workspace, else fallback, the login's.
// It is chosen the way the workspace reads choose it
// (manifest.Astro.WorkspaceOrganization), so Otto asks about the organization
// the project's values come from. A manifest that does not load names none.
func projectOrganization(fallback string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return fallback
	}
	proj, err := project.Discover(cwd)
	if err != nil {
		return fallback
	}
	m, err := manifest.Load(filepath.Join(proj.Dir, project.Marker))
	if err != nil {
		return fallback
	}
	return m.Astro.WorkspaceOrganization(fallback)
}

// DetectAirflow returns a URL to the Airflow belonging to the current project
// directory, or "" if there is none, and whether it is a project's. The nearest enclosing project's
// running Airflow wins — even over a v1 route registered on cwd itself;
// otherwise the v1 proxy routes decide.
func DetectAirflow() (url string, v2 bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	if url := detectV2Airflow(cwd); url != "" {
		return url, true
	}
	return detectV1Airflow(cwd), false
}

// detectV2Airflow reports the running local Airflow of the project cwd
// sits in, or "" when there is none. The health probe subsumes an engine
// liveness check — a stale record fails it the same way a stopped Airflow
// does. localhost:<port> rather than the record's hostname URL: the hostname
// needs the proxy daemon up, and the record is rewritten on every start, so
// the direct port is always current.
func detectV2Airflow(cwd string) string {
	proj, err := project.Discover(cwd)
	if err != nil {
		var notFound *project.NotFoundError
		if !errors.As(err, &notFound) {
			logger.Debugf("otto: discovering the project: %v", err)
		}
		return ""
	}
	rec, err := localrt.RecordedStatus(proj.Dir)
	if err != nil {
		if !localrt.IsNotRunning(err) {
			logger.Debugf("otto: reading local state for %s: %v", proj.Dir, err)
		}
		return ""
	}
	// Airflow 2's standalone generates its own admin password, which otto
	// doesn't read, so its token exchange would fail. Better no URL than a
	// half-wired one. Docker mode creates admin/admin itself.
	if (rec.AirflowMajor == "2" && rec.Mode != localrt.ModeDocker) || rec.Port == 0 {
		return ""
	}
	url := fmt.Sprintf("http://localhost:%d", rec.Port)
	if !isAirflowHealthy(url) {
		return ""
	}
	return url
}

// detectV1Airflow resolves through the v1 proxy routes. It prefers the proxy
// hostname URL because it's stable across `astro dev restart` — the container
// port rotates, the hostname doesn't.
func detectV1Airflow(cwd string) string {
	route, err := proxy.Routes().GetRouteByProject(cwd)
	if err != nil || route == nil || route.Port == "" {
		return ""
	}

	if route.Hostname != "" {
		proxyPort := proxy.BoundPort()
		if proxyPort == "" {
			proxyPort = pkgproxy.DefaultPort
		}
		hostnameURL := fmt.Sprintf("http://%s:%s", route.Hostname, proxyPort)
		if isAirflowHealthy(hostnameURL) {
			return hostnameURL
		}
	}

	portURL := fmt.Sprintf("http://localhost:%s", route.Port)
	if !isAirflowHealthy(portURL) {
		return ""
	}
	return portURL
}

// isAirflowHealthy checks if an Airflow instance is reachable at the given URL.
func isAirflowHealthy(url string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	for _, path := range []string{"/api/v2/monitor/health", "/api/v1/health"} {
		resp, err := client.Get(url + path)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
	}
	return false
}

// BuildEnv constructs the environment variables for the Otto process.
func (c *Config) BuildEnv() []string {
	env := os.Environ()
	set := func(key, value string) {
		if value == "" {
			return
		}
		prefix := key + "="
		for i, e := range env {
			if strings.HasPrefix(e, prefix) {
				env[i] = prefix + value
				return
			}
		}
		env = append(env, prefix+value)
	}

	// PATH gets two directories in front of it. The launcher directory comes
	// first, so the `astro` Otto's bash tool runs is the CLI that launched it
	// rather than whichever one the shell PATH finds first (a v1 install
	// beside a v2 build, say). It holds that one `astro` and nothing else: the
	// CLI's own directory may be ~/.local/bin or ~/go/bin, and putting that
	// first would shadow python, uv and airflow for Otto. Then ~/.astro/bin,
	// where Otto itself is installed.
	foldCase := runtime.GOOS == windowsGOOS
	env = prependPath(env, BinDir(), foldCase)
	// Named as well, so Otto asks this CLI for the login token (`astro auth
	// token`) whatever PATH it ends up with: the launcher entry, or the
	// binary itself when that entry could not be made.
	if dir := ensureLauncherBin(); dir != "" {
		env = prependPath(env, dir, foldCase)
		set(CLIPathEnv, filepath.Join(dir, launcherName()))
	} else if exe, err := executable(); err == nil {
		set(CLIPathEnv, exe)
	}

	// Auth context — Otto reads these instead of parsing config.yaml
	set("ASTRO_TOKEN", c.Token)
	set("ASTRO_DOMAIN", c.Domain)
	set("ASTRO_ORGANIZATION", c.Organization)

	// Stop `astro dev start` from popping a browser when Otto runs it.
	// The user is already having a conversation in the TUI — surprise browser
	// tabs steal focus and break the flow. Any `astro dev start` invoked by
	// Otto (or by the user from the bash tool) inherits this env var; the
	// airflow subcommands already honor it alongside their `--no-browser` flag.
	set("ASTRONOMER_NO_BROWSER", "1")

	// Airflow connection
	set("AIRFLOW_API_URL", c.AirflowURL)

	if c.AirflowURL != "" && !c.AirflowV2 {
		// A v1 route: the account the engines provision, from pkg/airflowrt
		// rather than spelled again here. Right for docker mode either
		// version, and for macOS standalone. NOT right for a non-macOS
		// standalone Airflow 2, whose password `airflow standalone` generates:
		// detectV1Airflow has no Airflow-major or mode information to refuse
		// it with, so such a route still gets a pair that will 401. See the
		// gap noted in pkg/airflowrt/account.go.
		//
		// A project's Airflow gets no pair: Otto reads that project's
		// credentials itself, and a default pair here would override them.
		set("AIRFLOW_USERNAME", airflowrt.Airflow2AdminUser)
		set("AIRFLOW_PASSWORD", airflowrt.Airflow2AdminPassword)
	}
	if c.AirflowURL == "" {
		// We couldn't match the current project to a running Airflow.
		// Point the af CLI at an empty config so it doesn't silently fall
		// back to ~/.af/config.yaml's `current-instance`, which is a
		// globally-scoped pointer that usually references whatever project
		// last ran `astro dev start` — not this one.
		set("AF_CONFIG", os.DevNull)
	}

	return env
}

// prependPath puts dir in front of env's PATH, adding one if there is none.
// foldCase matches the key case-insensitively, as Windows does: its
// environment usually spells it "Path", and a second, upper-case entry would
// leave the child with two variables and no telling which one it reads.
func prependPath(env []string, dir string, foldCase bool) []string {
	for i, e := range env {
		eq := strings.IndexByte(e, '=')
		if eq < 0 {
			continue
		}
		key := e[:eq]
		if key == "PATH" || (foldCase && strings.EqualFold(key, "PATH")) {
			env[i] = key + "=" + dir + string(os.PathListSeparator) + e[eq+1:]
			return env
		}
	}
	return append(env, "PATH="+dir)
}

// CLIPathEnv names, for Otto, the CLI that launched it. Otto runs it to get a
// current login token when config.yaml does not hold the login.
const CLIPathEnv = "ASTRO_CLI_PATH"

func launcherName() string {
	if runtime.GOOS == windowsGOOS {
		return "astro.exe"
	}
	return "astro"
}

// executable is os.Executable, a var so a test can stand in a launcher.
var executable = os.Executable

// LauncherBinDir holds the one `astro` Otto should run: the CLI that
// launched it. Nothing else is ever put there.
func LauncherBinDir() string {
	return filepath.Join(config.HomeConfigPath, "otto", "launcher-bin")
}

// ensureLauncherBin refreshes LauncherBinDir so it holds only an `astro`
// pointing at the running CLI, resolved through symlinks: a symlink, or a copy
// on Windows, where creating one needs a privilege most users lack. Refreshed
// every launch, so it follows whichever CLI ran last. Returns the directory, or
// "" when it could not be prepared, and then Otto gets no launcher entry
// rather than a stale one.
func ensureLauncherBin() string {
	exe, err := executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := LauncherBinDir()
	if err := os.RemoveAll(dir); err != nil {
		logger.Debugf("otto: clearing %s: %v", dir, err)
		return ""
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		logger.Debugf("otto: creating %s: %v", dir, err)
		return ""
	}
	name := launcherName()
	if runtime.GOOS == windowsGOOS {
		err = copyExecutable(exe, filepath.Join(dir, name))
	} else {
		err = os.Symlink(exe, filepath.Join(dir, name))
	}
	if err != nil {
		logger.Debugf("otto: linking the launcher into %s: %v", dir, err)
		return ""
	}
	return dir
}

// copyExecutable copies src to dst with the binary's mode.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, binPerm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
