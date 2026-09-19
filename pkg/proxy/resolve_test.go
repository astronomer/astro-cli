package proxy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A hostname is the project directory's base name, so two projects in
// directories called the same thing ask for the same one. AddRoute used to
// refuse the second outright, and its callers treated that as a log line: the
// project came up with no route while the name went on resolving to the
// first. Opening it showed somebody else's Airflow.
//
// The name is settled here rather than by the caller because this is where
// the lock is. A caller that looked first and registered afterwards would be
// guessing, and the loser of that race is exactly who must still get a name.

// liveStore returns a store whose routes are all considered alive, so a
// fixture is not pruned out from under the case it exists for.
func liveStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir(), WithRouteLiveness(func(Route) bool { return true }))
}

func held(t *testing.T, s *Store, hostname, projectDir string) {
	t.Helper()
	require.NoError(t, s.WriteRoutes([]Route{{Hostname: hostname, ProjectDir: projectDir, Port: "8080"}}))
}

// Nobody holds the name, so the project keeps it. The ordinary case, and the
// one that must not change: qualifying every hostname would be a new URL for
// every project that exists.
func TestAddRouteKeepsAFreeName(t *testing.T) {
	s := liveStore(t)
	route := &Route{Hostname: "analytics.localhost", ProjectDir: t.TempDir(), Port: "8081", Discriminator: "a1b2c3"}

	require.NoError(t, s.AddRoute(route))
	assert.Equal(t, "analytics.localhost", route.Hostname)
}

// The bug. Another project holds the name, so this one takes a qualified
// version rather than no name at all.
func TestAddRouteQualifiesANameAnotherProjectHolds(t *testing.T) {
	s := liveStore(t)
	held(t, s, "analytics.localhost", t.TempDir())

	mine := t.TempDir()
	route := &Route{Hostname: "analytics.localhost", ProjectDir: mine, Port: "8081", Discriminator: "a1b2c3"}
	require.NoError(t, s.AddRoute(route))

	// The chosen name is written back, because the caller has to record it:
	// it is what `astro local status` prints and what stop deregisters by.
	assert.Equal(t, "analytics-a1b2c3.localhost", route.Hostname)

	stored, err := s.GetRouteByProject(mine)
	require.NoError(t, err)
	require.NotNil(t, stored, "the second project must get a route of its own")
	assert.Equal(t, "analytics-a1b2c3.localhost", stored.Hostname)

	// And the holder keeps what it had.
	incumbent, err := s.GetRoute("analytics.localhost")
	require.NoError(t, err)
	require.NotNil(t, incumbent)
	assert.NotEqual(t, mine, incumbent.ProjectDir)
}

// A project re-registering its own route is not colliding with anything. It
// keeps its name, and updates its row rather than gaining a second one.
func TestAddRouteKeepsANameTheSameProjectHolds(t *testing.T) {
	s := liveStore(t)
	mine := t.TempDir()
	held(t, s, "analytics.localhost", mine)

	route := &Route{Hostname: "analytics.localhost", ProjectDir: mine, Port: "9999", Discriminator: "a1b2c3"}
	require.NoError(t, s.AddRoute(route))
	assert.Equal(t, "analytics.localhost", route.Hostname)

	routes, err := s.ReadRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1, "re-registering must update the row, not add one")
	assert.Equal(t, "9999", routes[0].Port)
}

// The same directory reached by a symlink is the same project. Comparing the
// paths as given would tell it its own name belongs to somebody else, and
// hand it a second route for one project.
//
// A symlink specifically: filepath.Join cleans a "sub/.." away before any
// comparison sees it, so a fixture built that way compares a path against
// itself and passes whether or not anything canonicalizes.
func TestAddRouteComparesProjectDirsCanonically(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs privileges on Windows")
	}
	s := liveStore(t)
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(target, link))

	held(t, s, "analytics.localhost", link)

	route := &Route{Hostname: "analytics.localhost", ProjectDir: target, Port: "8081", Discriminator: "a1b2c3"}
	require.NoError(t, s.AddRoute(route))
	assert.Equal(t, "analytics.localhost", route.Hostname, "one directory reached two ways is one project")

	routes, err := s.ReadRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1, "and it gets one route, not two")
}

// The qualified name can be taken too — by a directory literally called
// analytics-a1b2c3, or by a stale route that will not prune. Returning it
// anyway would put the project back where it started, with no route.
func TestAddRouteKeepsLookingWhenTheQualifiedNameIsTakenToo(t *testing.T) {
	s := liveStore(t)
	require.NoError(t, s.WriteRoutes([]Route{
		{Hostname: "analytics.localhost", ProjectDir: t.TempDir(), Port: "8080"},
		{Hostname: "analytics-a1b2c3.localhost", ProjectDir: t.TempDir(), Port: "8081"},
	}))

	mine := t.TempDir()
	route := &Route{Hostname: "analytics.localhost", ProjectDir: mine, Port: "8082", Discriminator: "a1b2c3"}
	require.NoError(t, s.AddRoute(route))

	assert.NotEqual(t, "analytics.localhost", route.Hostname)
	assert.NotEqual(t, "analytics-a1b2c3.localhost", route.Hostname)
	assert.True(t, strings.HasPrefix(route.Hostname, "analytics-a1b2c3-"), "%q", route.Hostname)

	stored, err := s.GetRoute(route.Hostname)
	require.NoError(t, err)
	require.NotNil(t, stored, "the name it reports must be the name it registered")
	assert.Equal(t, mine, stored.ProjectDir)
}

// A worktree's hostname carries the repo it belongs to, and qualifying it
// must not throw that away: the repo label is what tells two worktrees of
// different repos apart in the first place.
func TestAddRouteQualifyingAWorktreeKeepsTheRepo(t *testing.T) {
	s := liveStore(t)
	held(t, s, "feature-x.astro-cli.localhost", t.TempDir())

	route := &Route{Hostname: "feature-x.astro-cli.localhost", ProjectDir: t.TempDir(), Port: "8081", Discriminator: "a1b2c3"}
	require.NoError(t, s.AddRoute(route))
	assert.Equal(t, "feature-x-a1b2c3.astro-cli.localhost", route.Hostname)
}

// No discriminator, no way to tell this project apart from the holder, so the
// duplicate is refused exactly as it always was. Callers that have not opted
// in keep the behavior they had.
func TestAddRouteWithoutADiscriminatorStillRefuses(t *testing.T) {
	s := liveStore(t)
	theirs := t.TempDir()
	held(t, s, "analytics.localhost", theirs)

	err := s.AddRoute(&Route{Hostname: "analytics.localhost", ProjectDir: t.TempDir(), Port: "8081"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
	assert.Contains(t, err.Error(), theirs, "the refusal should say who has it")
}

// Two projects of the same name, registered in turn, end up at two names.
func TestAddRouteSeparatesTwoProjectsOfTheSameName(t *testing.T) {
	s := liveStore(t)

	first := &Route{Hostname: "analytics.localhost", ProjectDir: t.TempDir(), Port: "8081", Discriminator: "a1b2c3"}
	second := &Route{Hostname: "analytics.localhost", ProjectDir: t.TempDir(), Port: "8082", Discriminator: "d4e5f6"}
	require.NoError(t, s.AddRoute(first))
	require.NoError(t, s.AddRoute(second))

	assert.Equal(t, "analytics.localhost", first.Hostname, "the one that got there first keeps the plain name")
	assert.NotEqual(t, first.Hostname, second.Hostname)

	routes, err := s.ReadRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 2)
}

// The discriminator is an input, not part of the record: what routes.json
// needs is the name that was chosen.
func TestAddRouteDoesNotPersistTheDiscriminator(t *testing.T) {
	s := liveStore(t)
	require.NoError(t, s.AddRoute(&Route{
		Hostname: "analytics.localhost", ProjectDir: t.TempDir(), Port: "8081", Discriminator: "a1b2c3",
	}))

	raw, err := os.ReadFile(filepath.Join(s.Dir(), "routes.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "a1b2c3")
}

// The same directory reached with different capitalization is the same
// project, which matters because rt.CanonicalPath decided the same thing and
// these two have to agree.
//
// When they disagreed, a re-registration from the other spelling read as a
// stranger: the project was pushed onto a qualified hostname and the original
// row stayed behind pointing at a port nothing was listening on.
func TestAddRouteTreatsACaseVariantAsTheSameProject(t *testing.T) {
	base := t.TempDir()
	actual := filepath.Join(base, "Analytics")
	require.NoError(t, os.Mkdir(actual, 0o755))
	variant := filepath.Join(base, "analytics")
	if _, err := os.Stat(variant); err != nil {
		t.Skip("case-sensitive filesystem; the two spellings are two directories here")
	}

	s := liveStore(t)
	held(t, s, "analytics.localhost", actual)

	route := &Route{Hostname: "analytics.localhost", ProjectDir: variant, Port: "8081", Discriminator: "a1b2c3"}
	require.NoError(t, s.AddRoute(route))

	assert.Equal(t, "analytics.localhost", route.Hostname,
		"one directory reached two ways must keep its own name")

	routes, err := s.ReadRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1, "and must not gain a second row")
}
