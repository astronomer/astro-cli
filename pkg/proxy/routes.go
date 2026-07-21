package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	routesFileName = "routes.json"
	lockFileName   = "routes.lock"
	// lockTimeout must exceed the CLI's daemon start wait (10s): EnsureRunning
	// holds this lock while a daemon starts, and concurrent route writes should
	// outwait that instead of failing.
	lockTimeout = 15 * time.Second
	FilePermRW  = 0o600 // owner read/write
	DirPermRWX  = 0o755 // owner rwx, group/other rx
)

// Route represents a registered project route.
//
// Route is a wire contract: routes.json is read and written by multiple
// tools, including CLI versions already in the field. Add fields with
// omitempty; never rename, remove, or retype existing ones.
type Route struct {
	Hostname   string            `json:"hostname"`
	Port       string            `json:"port"`
	ProjectDir string            `json:"projectDir"`
	PID        int               `json:"pid"`
	Services   map[string]string `json:"services,omitempty"` // e.g. {"postgres": "15432"}
	Mode       string            `json:"mode,omitempty"`     // "docker" or "standalone"; empty treated as "standalone"
}

// Store reads and writes routes.json (and its lock file) in a directory.
type Store struct {
	dir string
}

// NewStore returns a Store rooted at dir. The directory is created on first write.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Dir returns the store's directory.
func (s *Store) Dir() string {
	return s.dir
}

// routesFilePath returns the path to routes.json.
func (s *Store) routesFilePath() string {
	return filepath.Join(s.dir, routesFileName)
}

// lockFilePath returns the path used for the flock-based file lock.
func (s *Store) lockFilePath() string {
	return filepath.Join(s.dir, lockFileName)
}

// ReadRoutes reads routes from the routes file. Returns empty slice if file doesn't exist.
func (s *Store) ReadRoutes() ([]Route, error) {
	data, err := os.ReadFile(s.routesFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Route{}, nil
		}
		return nil, fmt.Errorf("error reading routes file: %w", err)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []Route{}, nil
	}

	var routes []Route
	if err := json.Unmarshal(trimmed, &routes); err != nil {
		return nil, fmt.Errorf("error parsing routes file: %w", err)
	}
	return routes, nil
}

// WriteRoutes writes routes to the routes file atomically using a temp file + rename.
func (s *Store) WriteRoutes(routes []Route) error {
	if err := os.MkdirAll(s.dir, DirPermRWX); err != nil {
		return fmt.Errorf("error creating proxy directory: %w", err)
	}

	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling routes: %w", err)
	}

	tmpFile := s.routesFilePath() + ".tmp"
	if err := os.WriteFile(tmpFile, data, FilePermRW); err != nil {
		return fmt.Errorf("error writing routes temp file: %w", err)
	}
	if err := os.Rename(tmpFile, s.routesFilePath()); err != nil {
		os.Remove(tmpFile) //nolint:errcheck
		return fmt.Errorf("error renaming routes temp file: %w", err)
	}
	return nil
}

// PruneStaleRoutes removes routes whose owner process is no longer alive.
// Docker routes (Mode == "docker") are never pruned by PID because the CLI
// process exits after starting containers. They are cleaned up explicitly.
func PruneStaleRoutes(routes []Route) []Route {
	alive := make([]Route, 0, len(routes))
	for _, r := range routes {
		if r.Mode == "docker" || IsPIDAlive(r.PID) {
			alive = append(alive, r)
		}
	}
	return alive
}

// AddRoute registers a new route. It acquires the file lock, prunes stale routes,
// and adds the new route. Returns an error if the hostname is already registered
// for a different project directory.
func (s *Store) AddRoute(route *Route) error {
	lockFile, err := s.AcquireLock()
	if err != nil {
		return fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer ReleaseLock(lockFile)

	routes, err := s.ReadRoutes()
	if err != nil {
		return err
	}

	routes = PruneStaleRoutes(routes)

	// Check for hostname collision
	for i, r := range routes {
		if r.Hostname == route.Hostname {
			if r.ProjectDir == route.ProjectDir {
				// Same project, update the route
				routes[i] = *route
				return s.WriteRoutes(routes)
			}
			return fmt.Errorf("hostname %q is already registered for project %s", route.Hostname, r.ProjectDir)
		}
	}

	routes = append(routes, *route)
	return s.WriteRoutes(routes)
}

// RemoveRoute deregisters a route by hostname. Returns the number of remaining routes.
func (s *Store) RemoveRoute(hostname string) (int, error) {
	lockFile, err := s.AcquireLock()
	if err != nil {
		return 0, fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer ReleaseLock(lockFile)

	routes, err := s.ReadRoutes()
	if err != nil {
		return 0, err
	}

	routes = PruneStaleRoutes(routes)

	filtered := make([]Route, 0, len(routes))
	for _, r := range routes {
		if r.Hostname != hostname {
			filtered = append(filtered, r)
		}
	}

	if err := s.WriteRoutes(filtered); err != nil {
		return 0, err
	}
	return len(filtered), nil
}

// ListRoutes returns all active routes (after pruning stale ones).
func (s *Store) ListRoutes() ([]Route, error) {
	lockFile, err := s.AcquireLock()
	if err != nil {
		return nil, fmt.Errorf("error acquiring routes lock: %w", err)
	}
	defer ReleaseLock(lockFile)

	routes, err := s.ReadRoutes()
	if err != nil {
		return nil, err
	}

	routes = PruneStaleRoutes(routes)

	// Write back pruned routes
	if err := s.WriteRoutes(routes); err != nil {
		return nil, err
	}

	return routes, nil
}

// GetRoute returns the route for a given hostname, or nil if not found.
// This is a read-only operation that does not acquire the write lock or
// prune stale routes, making it safe to call on the hot path (e.g. per
// HTTP request in the proxy handler).
func (s *Store) GetRoute(hostname string) (*Route, error) {
	routes, err := s.ReadRoutes()
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

// GetRouteByProject returns the route for a given project directory, or nil if not found.
func (s *Store) GetRouteByProject(projectDir string) (*Route, error) {
	routes, err := s.ReadRoutes()
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].ProjectDir == projectDir {
			return &routes[i], nil
		}
	}
	return nil, nil
}
