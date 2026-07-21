package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// Route.Mode wire values. localrt.Mode carries the same strings in the
// state record; changing one breaks tools already in the field.
const (
	RouteModeStandalone = "standalone"
	RouteModeDocker     = "docker"
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
	Mode       string            `json:"mode,omitempty"`     // RouteModeDocker or RouteModeStandalone; empty treated as standalone
}

// Store reads and writes routes.json (and its lock file) in a directory.
//
// Reads are served from an in-memory cache of the parsed file, invalidated
// by an os.Stat mtime+size check, so the hot path (GetRoute per proxied
// request) does not re-read and re-parse routes.json until it changes.
type Store struct {
	dir string

	// cacheMu guards the cached parse below. It is a plain mutex, not the
	// cross-process file lock: cached reads never touch routes.lock.
	cacheMu     sync.Mutex
	cached      []Route
	cachedTime  time.Time // mtime of the file the cache was parsed from
	cachedSize  int64     // size of the file the cache was parsed from
	cachedValid bool
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

// cacheGet returns a copy of the cached routes if the cache was parsed from
// a file with this mtime and size.
func (s *Store) cacheGet(mtime time.Time, size int64) ([]Route, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if !s.cachedValid || !s.cachedTime.Equal(mtime) || s.cachedSize != size {
		return nil, false
	}
	return append([]Route(nil), s.cached...), true
}

// cacheSet records routes as the parse of a file with this mtime and size.
func (s *Store) cacheSet(routes []Route, mtime time.Time, size int64) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cached = append([]Route(nil), routes...)
	s.cachedTime = mtime
	s.cachedSize = size
	s.cachedValid = true
}

// cacheInvalidate drops the cached parse.
func (s *Store) cacheInvalidate() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cached = nil
	s.cachedValid = false
}

// ReadRoutes reads routes from the routes file. Returns empty slice if file
// doesn't exist. The result is the caller's to keep: it never aliases the
// cache.
func (s *Store) ReadRoutes() ([]Route, error) {
	path := s.routesFilePath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cacheInvalidate()
			return []Route{}, nil
		}
		return nil, fmt.Errorf("reading routes file: %w", err)
	}
	if routes, ok := s.cacheGet(info.ModTime(), info.Size()); ok {
		return routes, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cacheInvalidate()
			return []Route{}, nil
		}
		return nil, fmt.Errorf("reading routes file: %w", err)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		s.cacheSet(nil, info.ModTime(), info.Size())
		return []Route{}, nil
	}

	var routes []Route
	if err := json.Unmarshal(trimmed, &routes); err != nil {
		return nil, fmt.Errorf("parsing routes file: %w", err)
	}
	s.cacheSet(routes, info.ModTime(), info.Size())
	return routes, nil
}

// WriteRoutes writes routes to the routes file atomically using a temp file + rename.
func (s *Store) WriteRoutes(routes []Route) error {
	if err := os.MkdirAll(s.dir, DirPermRWX); err != nil {
		return fmt.Errorf("creating proxy directory: %w", err)
	}

	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling routes: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, "."+routesFileName+".*")
	if err != nil {
		return fmt.Errorf("creating routes temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		// CreateTemp creates 0o600; keep the published file's mode explicit.
		werr = os.Chmod(tmpPath, FilePermRW)
	}
	if werr == nil {
		werr = os.Rename(tmpPath, s.routesFilePath())
	}
	if werr != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("writing routes file: %w", werr)
	}
	// Refresh the cache from the file just published, so writers through
	// the Store keep readers current without another parse.
	if info, statErr := os.Stat(s.routesFilePath()); statErr == nil {
		s.cacheSet(routes, info.ModTime(), info.Size())
	} else {
		s.cacheInvalidate()
	}
	return nil
}

// PruneStaleRoutes removes routes whose owner process is no longer alive.
// Docker routes (Mode == RouteModeDocker) are never pruned by PID because
// the CLI process exits after starting containers. They are cleaned up
// explicitly.
func PruneStaleRoutes(routes []Route) []Route {
	alive := make([]Route, 0, len(routes))
	for _, r := range routes {
		if r.Mode == RouteModeDocker || IsPIDAlive(r.PID) {
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
		return fmt.Errorf("acquiring routes lock: %w", err)
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
		return 0, fmt.Errorf("acquiring routes lock: %w", err)
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
		return nil, fmt.Errorf("acquiring routes lock: %w", err)
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
// This is a read-only operation that does not acquire the file lock or
// prune stale routes, making it safe to call on the hot path (e.g. per
// HTTP request in the proxy handler); the cached parse in ReadRoutes keeps
// it from re-reading an unchanged routes.json.
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
