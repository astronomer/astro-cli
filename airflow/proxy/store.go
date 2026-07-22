package proxy

import (
	"path/filepath"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

const proxyDir = "proxy"

// ServeSubcommand is the hidden CLI subcommand the daemon re-execs into to
// run the reverse-proxy server loop (see StartDaemon). It lives here, next to
// the daemon, so the command that implements it (cmd/local) and the code that
// spawns it (daemon.go) name it from one place. It mirrors the standalone
// engine's `__supervise` convention.
const ServeSubcommand = "__proxy-serve"

// Routes returns the route store rooted at ~/.astro/proxy. Stores are cheap;
// building one per call keeps this free of package-level state and picks up
// config.HomeConfigPath changes (tests point it at temp dirs).
func Routes() *pkgproxy.Store {
	return pkgproxy.NewStore(filepath.Join(config.HomeConfigPath, proxyDir))
}

// isPIDAlive wraps pkgproxy.IsPIDAlive so test overrides of that variable
// take effect here too.
var isPIDAlive = func(pid int) bool { return pkgproxy.IsPIDAlive(pid) }
