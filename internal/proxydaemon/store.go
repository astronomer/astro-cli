package proxydaemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// ServeSubcommand is the hidden CLI subcommand the daemon re-execs into to
// run the reverse-proxy server loop. It is pkg/proxy's, so the command that
// implements it (cmd/local), the code that spawns it (daemon.go) and any other
// host that starts the daemon name it from one place. It mirrors the
// standalone engine's `__supervise` convention.
const ServeSubcommand = pkgproxy.ServeSubcommand

// errNoHome is what Dir says when there is no home directory to put the routes
// under, wrapping the cause.
const errNoHome = "finding a home directory for local Airflow's proxy routes: set HOME or ASTRO_HOME"

// Dir is where the daemon keeps routes.json and its record:
// <ASTRO_HOME or home>/.astro/proxy, the location v1 uses, since v1 and v2 must
// see each other's routes. It is config.HomeConfigPath's proxy directory,
// worked out by the same rule without reading config/: ASTRO_HOME when set,
// else fileutil.GetHomeDir (go-homedir, which falls back to getent or dscl
// when $HOME is unset), joined with .astro. config does not export that rule,
// so this is its one other copy. When there is no home at all, Dir fails
// rather than return a relative path.
func Dir() (string, error) {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		var err error
		if home, err = fileutil.GetHomeDir(); err != nil {
			return "", fmt.Errorf("%s: %w", errNoHome, err)
		}
		if home == "" {
			return "", errors.New(errNoHome)
		}
	}
	return filepath.Join(home, ".astro", "proxy"), nil
}

// Routes returns the route store rooted at dir. Stores are cheap; building one
// per call keeps this free of package-level state.
func Routes(dir string) *pkgproxy.Store {
	return pkgproxy.NewStore(dir)
}
