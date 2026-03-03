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
	lockFileName   = "routes.lock"
	lockTimeout    = 5 * time.Second
	filePermRW     = 0o600 // owner read/write
	dirPermRWX     = 0o755 // owner rwx, group/other rx
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

// lockFilePath returns the path used for the flock-based file lock.
func lockFilePath() string {
	return filepath.Join(proxyDirPath(), lockFileName)
}

// acquireLock acquires an exclusive file lock (flock) with timeout.
// Returns the lock file which must be passed to releaseLock.
func acquireLock() (*os.File, error) {
	if err := os.MkdirAll(proxyDirPath(), dirPermRWX); err != nil {
		return nil, fmt.Errorf("error creating proxy directory: %w", err)
	}

	f, err := os.OpenFile(lockFilePath(), os.O_CREATE|os.O_RDWR, filePermRW)
	if err != nil {
		return nil, fmt.Errorf("error opening lock file: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timed out waiting for routes lock")
		}
		time.Sleep(50 * time.Millisecond) //nolint:mnd
	}
}

// releaseLock releases the flock and closes the lock file.
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	f.Close()
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

// writeRoutes writes routes to the routes file atomically using a temp file + rename.
func writeRoutes(routes []Route) error {
	if err := os.MkdirAll(proxyDirPath(), dirPermRWX); err != nil {
		return fmt.Errorf("error creating proxy directory: %w", err)
	}

	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling routes: %w", err)
	}

	tmpFile := routesFilePath() + ".tmp"
	if err := os.WriteFile(tmpFile, data, filePermRW); err != nil {
		return fmt.Errorf("error writing routes temp file: %w", err)
	}
	if err := os.Rename(tmpFile, routesFilePath()); err != nil {
		os.Remove(tmpFile) //nolint:errcheck
		return fmt.Errorf("error renaming routes temp file: %w", err)
	}
	return nil
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
func AddRoute(route *Route) error {
	lockFile, err := acquireLock()
	if err != nil {
		return fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer releaseLock(lockFile)

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
				routes[i] = *route
				return writeRoutes(routes)
			}
			return fmt.Errorf("hostname %q is already registered for project %s", route.Hostname, r.ProjectDir)
		}
	}

	routes = append(routes, *route)
	return writeRoutes(routes)
}

// RemoveRoute deregisters a route by hostname. Returns the number of remaining routes.
func RemoveRoute(hostname string) (int, error) {
	lockFile, err := acquireLock()
	if err != nil {
		return 0, fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer releaseLock(lockFile)

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
	lockFile, err := acquireLock()
	if err != nil {
		return nil, fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer releaseLock(lockFile)

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
// This is a read-only operation that does not acquire the write lock or
// prune stale routes, making it safe to call on the hot path (e.g. per
// HTTP request in the proxy handler).
func GetRoute(hostname string) (*Route, error) {
	routes, err := readRoutes()
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
