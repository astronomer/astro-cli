package airflow

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	proxy "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// routesInTempHome points the shared routes store at a directory of this
// test's own, the way airflow/proxy's own tests do.
func routesInTempHome(t *testing.T) *pkgproxy.Store {
	t.Helper()
	orig := config.HomeConfigPath
	config.HomeConfigPath = t.TempDir()
	t.Cleanup(func() { config.HomeConfigPath = orig })
	return proxy.Routes()
}

// removeProxyRoute must remove this project's route and nobody else's.
//
// It used to re-derive the hostname from the project directory, and a
// hostname is the directory's base name — so `astro dev stop` in
// ~/personal/analytics derived analytics.localhost and deleted whatever held
// it, which could be a different project, still running, that had registered
// first. The route knows which project it belongs to; this asks it.
func TestRemoveProxyRouteLeavesAnotherProjectsRouteAlone(t *testing.T) {
	store := routesInTempHome(t)

	theirs := filepath.Join(t.TempDir(), "analytics")
	mine := filepath.Join(t.TempDir(), "analytics")

	// The other project got there first and holds the plain name; this one
	// was qualified out of the way, exactly as AddRoute now arranges.
	// Docker-mode, which is what this path registers — and which survives
	// pruning without a live PID, so the fixture is still there when the
	// removal runs. A standalone fixture with no PID prunes away, and the
	// case then passes for having deleted nothing at all.
	require.NoError(t, store.WriteRoutes([]pkgproxy.Route{
		{Hostname: "analytics.localhost", ProjectDir: theirs, Port: "8080", Mode: pkgproxy.RouteModeDocker},
		{Hostname: "analytics-a1b2c3.localhost", ProjectDir: mine, Port: "8081", Mode: pkgproxy.RouteModeDocker},
	}))

	d := &DockerCompose{airflowHome: mine}
	d.removeProxyRoute()

	gone, err := store.GetRouteByProject(mine)
	require.NoError(t, err)
	assert.Nil(t, gone, "the project's own route should be removed")

	kept, err := store.GetRoute("analytics.localhost")
	require.NoError(t, err)
	require.NotNil(t, kept, "another project's route must survive a stop that is not its own")
	assert.Equal(t, theirs, kept.ProjectDir)
}

// And a project that never registered anything removes nothing.
func TestRemoveProxyRouteWithNoRouteOfItsOwnRemovesNothing(t *testing.T) {
	store := routesInTempHome(t)

	theirs := filepath.Join(t.TempDir(), "analytics")
	require.NoError(t, store.WriteRoutes([]pkgproxy.Route{
		{Hostname: "analytics.localhost", ProjectDir: theirs, Port: "8080", Mode: pkgproxy.RouteModeDocker},
	}))

	d := &DockerCompose{airflowHome: filepath.Join(t.TempDir(), "analytics")}
	d.removeProxyRoute()

	kept, err := store.GetRoute("analytics.localhost")
	require.NoError(t, err)
	require.NotNil(t, kept, "a project with no route of its own must not remove somebody else's")
	assert.Equal(t, theirs, kept.ProjectDir)
}
