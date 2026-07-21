package proxy

import (
	"path/filepath"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

const proxyDir = "proxy"

// Routes returns the route store rooted at ~/.astro/proxy. Stores are cheap;
// building one per call keeps this free of package-level state and picks up
// config.HomeConfigPath changes (tests point it at temp dirs).
func Routes() *pkgproxy.Store {
	return pkgproxy.NewStore(filepath.Join(config.HomeConfigPath, proxyDir))
}

// isPIDAlive wraps pkgproxy.IsPIDAlive so test overrides of that variable
// take effect here too.
var isPIDAlive = func(pid int) bool { return pkgproxy.IsPIDAlive(pid) }
