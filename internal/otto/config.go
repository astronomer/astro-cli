package otto

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// Config holds the environment configuration for spawning Otto.
type Config struct {
	Token        string
	Domain       string
	Organization string
	AirflowURL   string
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
		cfg.Organization = ctx.Organization
	}

	return cfg
}

// DetectAirflow returns a URL to the Airflow belonging to the current project
// directory, or "" if there is none. The nearest enclosing v2 project's
// running Airflow wins — even over a v1 route registered on cwd itself;
// otherwise the v1 proxy routes decide.
func DetectAirflow() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if url := detectV2Airflow(cwd); url != "" {
		return url
	}
	return detectV1Airflow(cwd)
}

// detectV2Airflow reports the running local Airflow of the v2 project cwd
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
			logger.Debugf("otto: discovering v2 project: %v", err)
		}
		return ""
	}
	rec, err := localstate.Load(proj.Dir)
	if err != nil {
		if !errors.Is(err, localstate.ErrNotRunning) {
			logger.Debugf("otto: reading local state for %s: %v", proj.Dir, err)
		}
		return ""
	}
	// Airflow 2's standalone generates its own admin password, which otto
	// doesn't read — BuildEnv would pair the URL with admin/admin and the
	// token exchange would fail. Better no URL than a half-wired one.
	if rec.AirflowMajor == "2" || rec.Port == 0 {
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

	// Prepend ~/.astro/bin to PATH so Otto's bash tool can resolve the `af`
	// wrapper we install in EnsureAfWrapper. Without this we'd be at the mercy
	// of the user having added ~/.astro/bin to their shell PATH manually.
	prependPath := func(dir string) {
		prefix := "PATH="
		for i, e := range env {
			if strings.HasPrefix(e, prefix) {
				existing := strings.TrimPrefix(e, prefix)
				env[i] = prefix + dir + string(os.PathListSeparator) + existing
				return
			}
		}
		env = append(env, prefix+dir)
	}
	prependPath(BinDir())

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

	if c.AirflowURL != "" {
		// Default local Airflow credentials for af CLI token exchange
		set("AIRFLOW_USERNAME", "admin")
		set("AIRFLOW_PASSWORD", "admin")
	} else {
		// We couldn't match the current project to a running Airflow.
		// Point the af CLI at an empty config so it doesn't silently fall
		// back to ~/.af/config.yaml's `current-instance`, which is a
		// globally-scoped pointer that usually references whatever project
		// last ran `astro dev start` — not this one.
		set("AF_CONFIG", os.DevNull)
	}

	return env
}
