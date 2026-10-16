package proxydaemon

import (
	"os"
	"path/filepath"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// ServeSubcommand is the hidden CLI subcommand the daemon re-execs into to
// run the reverse-proxy server loop. It is pkg/proxy's, so the command that
// implements it (cmd/local), the code that spawns it (daemon.go) and any other
// host that starts the daemon name it from one place. It mirrors the
// standalone engine's `__supervise` convention.
const ServeSubcommand = pkgproxy.ServeSubcommand

// Dir is where the daemon keeps routes.json and its record: <astro home>/proxy,
// the same location v1 uses, honoring the same ASTRO_HOME override, since v1
// and v2 must see each other's routes. It is config.HomeConfigPath's proxy
// directory worked out without reading config/, so the core tree can name it.
func Dir() string {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home, _ = os.UserHomeDir() //nolint:errcheck // falls back to a relative path, matching v1's ASTRO_HOME handling
	}
	return filepath.Join(home, ".astro", "proxy")
}

// Routes returns the route store rooted at dir. Stores are cheap; building one
// per call keeps this free of package-level state.
func Routes(dir string) *pkgproxy.Store {
	return pkgproxy.NewStore(dir)
}
