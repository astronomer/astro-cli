package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
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

	// Discriminator is what AddRoute folds into Hostname when another
	// project already holds it — see DisambiguateHostname. Six hex
	// characters of the project's path hash, from the caller that has one.
	//
	// An input to registration rather than part of the record, so it is not
	// serialized: what routes.json needs is the name that was chosen, and
	// AddRoute writes that back into Hostname.
	//
	// Empty means the caller has no way to tell its project apart from the
	// holder's, and AddRoute refuses the duplicate as it always did.
	Discriminator string `json:"-"`
}

// Store reads and writes routes.json (and its lock file) in a directory.
//
// Reads are served from an in-memory cache of the parsed file, invalidated
// by an os.Stat mtime+size check, so the hot path (GetRoute per proxied
// request) does not re-read and re-parse routes.json until it changes.
type Store struct {
	dir string

	// routeAlive decides whether a route survives pruning. When nil the
	// Store uses defaultRouteAlive (docker kept, others PID-checked). The CLI
	// injects a record-aware predicate through WithRouteLiveness so a route
	// is never evicted while its owning state record reports the runtime
	// alive; pkg/proxy stays free of astro-cli imports because the predicate
	// arrives from outside.
	routeAlive func(Route) bool

	// cacheMu guards the cached parse below. It is a plain mutex, not the
	// cross-process file lock: cached reads never touch routes.lock.
	cacheMu     sync.Mutex
	cached      []Route
	cachedTime  time.Time // mtime of the file the cache was parsed from
	cachedSize  int64     // size of the file the cache was parsed from
	cachedValid bool
}

// StoreOption configures a Store at construction.
type StoreOption func(*Store)

// WithRouteLiveness sets the predicate that decides whether a route survives
// pruning. It replaces the default PID check for every prune this Store does
// (AddRoute, RemoveRoute, ListRoutes).
func WithRouteLiveness(alive func(Route) bool) StoreOption {
	return func(s *Store) { s.routeAlive = alive }
}

// NewStore returns a Store rooted at dir. The directory is created on first write.
func NewStore(dir string, opts ...StoreOption) *Store {
	s := &Store{dir: dir}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

	// Through fsatomic for the same reason the write is: the other proxy
	// publishes this file by renaming onto it, and on Windows that makes it
	// briefly unopenable. This read is on the request path — GetRoute calls it
	// to route a proxied request — so failing it turns a publish on one side
	// into a failed request on the other.
	data, err := fsatomic.ReadFile(path)
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

	// Through fsatomic rather than an inline temp-and-rename: this file has two
	// writers by design — the CLI's proxy on 6563 and Astro Desktop's on 6564 —
	// and on Windows a rename onto a file the other one has open fails outright.
	// The copy this replaced did not retry at all, so that collision simply lost
	// a route.
	if err := fsatomic.WriteFile(s.routesFilePath(), data, FilePermRW); err != nil {
		return err
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

// defaultRouteAlive is the prune predicate a Store uses when none is
// injected. Docker routes are never pruned by PID because the CLI process
// exits after starting containers; they are cleaned up explicitly.
func defaultRouteAlive(r Route) bool { //nolint:gocritic // hugeParam: matches the public WithRouteLiveness seam, which takes Route by value
	return r.Mode == RouteModeDocker || IsPIDAlive(r.PID)
}

// PruneStaleRoutes removes routes whose owner is no longer alive, by the
// default PID check. Kept for callers that prune outside a Store.
func PruneStaleRoutes(routes []Route) []Route {
	return prune(routes, defaultRouteAlive)
}

// prune returns the routes for which alive reports true.
func prune(routes []Route, alive func(Route) bool) []Route {
	kept := make([]Route, 0, len(routes))
	for _, r := range routes {
		if alive(r) {
			kept = append(kept, r)
		}
	}
	return kept
}

// pruneStale drops the Store's stale routes, using the injected liveness
// predicate when set and the default PID check otherwise.
func (s *Store) pruneStale(routes []Route) []Route {
	alive := s.routeAlive
	if alive == nil {
		alive = defaultRouteAlive
	}
	return prune(routes, alive)
}

// AddRoute registers a route under a hostname nobody else is using.
//
// The caller's Hostname is a preference. If another project already holds it,
// the route's Discriminator is folded in — analytics.localhost becomes
// analytics-a1b2c3.localhost — and the name actually chosen is written back
// into route.Hostname, which is what the caller should record and later
// deregister by. A caller with no Discriminator gets the older answer: an
// error naming the project that holds the name.
//
// Resolving here rather than in the caller is deliberate. This is where the
// collision is detected and where the routes lock is held, so the name a
// project gets and the row that claims it are decided in the same breath.
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

	routes = s.pruneStale(routes)

	// The name is settled here, inside the lock the write already holds,
	// and not by the caller beforehand. A caller that looked first and
	// registered afterwards would be guessing: between the two, another
	// project can take the name, and the whole point of this is that the
	// loser of that race must still end up with a name of its own.
	chosen, err := resolveHostname(routes, route)
	if err != nil {
		return err
	}
	route.Hostname = chosen

	for i := range routes {
		if routes[i].Hostname == route.Hostname {
			// Same project, update the route. resolveHostname only returns a
			// name held by somebody else if it returned an error, so this is
			// the project's own row.
			routes[i] = *route
			return s.WriteRoutes(routes)
		}
	}

	routes = append(routes, *route)
	return s.WriteRoutes(routes)
}

// maxQualifiedAttempts bounds the search for a free name. The first candidate
// is the project's own path hash, so reaching even the second means two
// different projects whose hashes agree in six hex characters; the counter
// exists so that a directory literally named after somebody's qualified
// hostname cannot wedge a start, not because it is expected to be used.
const maxQualifiedAttempts = 10

// resolveHostname returns the name route should register under: its own if
// nobody else holds it, otherwise one qualified by its discriminator.
//
// "Somebody else" is the test that matters. A project re-registering its own
// route — a restart, a port change — is not colliding with anything and keeps
// its name.
func resolveHostname(routes []Route, route *Route) (string, error) {
	mine := canonicalDir(route.ProjectDir)
	holder := func(name string) (string, bool) {
		for i := range routes {
			if routes[i].Hostname == name && canonicalDir(routes[i].ProjectDir) != mine {
				return routes[i].ProjectDir, true
			}
		}
		return "", false
	}

	held, taken := holder(route.Hostname)
	if !taken {
		return route.Hostname, nil
	}
	if route.Discriminator == "" {
		return "", fmt.Errorf("hostname %q is already registered for project %s", route.Hostname, held)
	}

	candidate := DisambiguateHostname(route.Hostname, route.Discriminator)
	for n := 2; ; n++ {
		if _, taken := holder(candidate); !taken {
			return candidate, nil
		}
		if n > maxQualifiedAttempts {
			return "", fmt.Errorf("hostname %q is already registered for project %s, and no name derived from it is free", route.Hostname, held)
		}
		candidate = DisambiguateHostname(route.Hostname, fmt.Sprintf("%s-%d", route.Discriminator, n))
	}
}

// canonicalDir resolves a project directory for comparison: absolute, with
// symlinks resolved. Two spellings of one directory have to read as one
// project, or a project reached by a symlink would be told its own name
// belongs to somebody else.
//
// Falls back to the closest thing it managed, so a directory that has since
// been deleted still compares equal to itself. This mirrors rt.CanonicalPath,
// which this module cannot import — pkg/localrt depends on pkg/proxy, not the
// other way round.
func canonicalDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
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

	routes = s.pruneStale(routes)

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

	routes = s.pruneStale(routes)

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
