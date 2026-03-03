package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/config"
)

const (
	proxyDir       = "proxy"
	routesFileName = "routes.json"
	lockDirName    = "routes.lock"
	lockTimeout    = 5 * time.Second
	lockPoll       = 50 * time.Millisecond
)

// Route represents a registered project route.
type Route struct {
	Hostname   string            `json:"hostname"`
	Port       string            `json:"port"`
	ProjectDir string            `json:"projectDir"`
	PID        int               `json:"pid"`
	Services   map[string]string `json:"services,omitempty"` // e.g. {"postgres": "15432"}
	Mode       string            `json:"mode,omitempty"`     // "docker" or "standalone"; empty treated as "standalone"
}

// proxyDirPath returns the path to ~/.astro/proxy/.
var proxyDirPath = func() string {
	return filepath.Join(config.HomeConfigPath, proxyDir)
}

// routesFilePath returns the path to ~/.astro/proxy/routes.json.
func routesFilePath() string {
	return filepath.Join(proxyDirPath(), routesFileName)
}

// lockPath returns the path used for the directory-based file lock.
func lockPath() string {
	return filepath.Join(proxyDirPath(), lockDirName)
}

// acquireLock acquires a directory-based lock with timeout.
func acquireLock() error {
	// Ensure the proxy directory exists
	if err := os.MkdirAll(proxyDirPath(), 0o755); err != nil {
		return fmt.Errorf("error creating proxy directory: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := os.Mkdir(lockPath(), 0o755)
		if err == nil {
			return nil // lock acquired
		}
		if time.Now().After(deadline) {
			// Force-remove stale lock after timeout
			os.Remove(lockPath())
			return os.Mkdir(lockPath(), 0o755)
		}
		time.Sleep(lockPoll)
	}
}

// releaseLock releases the directory-based lock.
func releaseLock() {
	os.Remove(lockPath())
}

// readRoutes reads routes from the routes file. Returns empty slice if file doesn't exist.
func readRoutes() ([]Route, error) {
	data, err := os.ReadFile(routesFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Route{}, nil
		}
		return nil, fmt.Errorf("error reading routes file: %w", err)
	}
	if len(data) == 0 {
		return []Route{}, nil
	}

	var routes []Route
	if err := json.Unmarshal(data, &routes); err != nil {
		return nil, fmt.Errorf("error parsing routes file: %w", err)
	}
	return routes, nil
}

// writeRoutes writes routes to the routes file atomically.
func writeRoutes(routes []Route) error {
	if err := os.MkdirAll(proxyDirPath(), 0o755); err != nil {
		return fmt.Errorf("error creating proxy directory: %w", err)
	}

	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling routes: %w", err)
	}

	return os.WriteFile(routesFilePath(), data, 0o644)
}

// isPIDAlive checks if a process with the given PID is still running.
var isPIDAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// pruneStaleRoutes removes routes whose owner process is no longer alive.
// Docker routes (Mode == "docker") are never pruned by PID because the CLI
// process exits after starting containers. They are cleaned up explicitly
// by "astro dev stop" / "astro dev kill".
func pruneStaleRoutes(routes []Route) []Route {
	alive := make([]Route, 0, len(routes))
	for _, r := range routes {
		if r.Mode == "docker" || isPIDAlive(r.PID) {
			alive = append(alive, r)
		}
	}
	return alive
}

// AddRoute registers a new route. It acquires the file lock, prunes stale routes,
// and adds the new route. Returns an error if the hostname is already registered
// for a different project directory.
func AddRoute(route Route) error {
	if err := acquireLock(); err != nil {
		return fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer releaseLock()

	routes, err := readRoutes()
	if err != nil {
		return err
	}

	routes = pruneStaleRoutes(routes)

	// Check for hostname collision
	for i, r := range routes {
		if r.Hostname == route.Hostname {
			if r.ProjectDir == route.ProjectDir {
				// Same project, update the route
				routes[i] = route
				return writeRoutes(routes)
			}
			return fmt.Errorf("hostname %q is already registered for project %s", route.Hostname, r.ProjectDir)
		}
	}

	routes = append(routes, route)
	return writeRoutes(routes)
}

// RemoveRoute deregisters a route by hostname. Returns the number of remaining routes.
func RemoveRoute(hostname string) (int, error) {
	if err := acquireLock(); err != nil {
		return 0, fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer releaseLock()

	routes, err := readRoutes()
	if err != nil {
		return 0, err
	}

	routes = pruneStaleRoutes(routes)

	filtered := make([]Route, 0, len(routes))
	for _, r := range routes {
		if r.Hostname != hostname {
			filtered = append(filtered, r)
		}
	}

	if err := writeRoutes(filtered); err != nil {
		return 0, err
	}
	return len(filtered), nil
}

// ListRoutes returns all active routes (after pruning stale ones).
func ListRoutes() ([]Route, error) {
	if err := acquireLock(); err != nil {
		return nil, fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer releaseLock()

	routes, err := readRoutes()
	if err != nil {
		return nil, err
	}

	routes = pruneStaleRoutes(routes)

	// Write back pruned routes
	if err := writeRoutes(routes); err != nil {
		return nil, err
	}

	return routes, nil
}

// GetRoute returns the route for a given hostname, or nil if not found.
func GetRoute(hostname string) (*Route, error) {
	routes, err := ListRoutes()
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].Hostname == hostname {
			return &routes[i], nil
		}
	}
	return nil, nil
}
