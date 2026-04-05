package proxy

import (
	"path/filepath"
	"sync"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

const (
	proxyDir = "proxy"
)

// Route is an alias for the shared Route type.
type Route = pkgproxy.Route

var initOnce sync.Once

// ensureInit sets the routes directory from the CLI config on first use.
func ensureInit() {
	initOnce.Do(func() {
		pkgproxy.SetRoutesDir(filepath.Join(config.HomeConfigPath, proxyDir))
	})
}

// proxyDirPath returns the path to ~/.astro/proxy/.
var proxyDirPath = func() string {
	ensureInit()
	return pkgproxy.RoutesDir()
}

// AddRoute registers a new route.
func AddRoute(route *Route) error {
	ensureInit()
	return pkgproxy.AddRoute(route)
}

// RemoveRoute deregisters a route by hostname.
func RemoveRoute(hostname string) (int, error) {
	ensureInit()
	return pkgproxy.RemoveRoute(hostname)
}

// ListRoutes returns all active routes (after pruning stale ones).
func ListRoutes() ([]Route, error) {
	ensureInit()
	return pkgproxy.ListRoutes()
}

// GetRoute returns the route for a given hostname, or nil if not found.
func GetRoute(hostname string) (*Route, error) {
	ensureInit()
	return pkgproxy.GetRoute(hostname)
}

// GetRouteByProject returns the route for a given project directory, or nil if not found.
func GetRouteByProject(projectDir string) (*Route, error) {
	ensureInit()
	return pkgproxy.GetRouteByProject(projectDir)
}
