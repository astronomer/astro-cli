package proxy

import (
	"path/filepath"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

const proxyDir = "proxy"

// ServeSubcommand is the hidden CLI subcommand the daemon re-execs into to
// run the reverse-proxy server loop. It is pkg/proxy's, so the command that
// implements it (cmd/local), the code that spawns it (daemon.go) and any other
// host that starts the daemon name it from one place. It mirrors the
// standalone engine's `__supervise` convention.
const ServeSubcommand = pkgproxy.ServeSubcommand

// Routes returns the route store rooted at ~/.astro/proxy. Stores are cheap;
// building one per call keeps this free of package-level state and picks up
// config.HomeConfigPath changes (tests point it at temp dirs).
func Routes() *pkgproxy.Store {
	return pkgproxy.NewStore(filepath.Join(config.HomeConfigPath, proxyDir))
}
