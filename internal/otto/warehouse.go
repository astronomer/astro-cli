package otto

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/connwarehouse"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// reachingConnections is the vault's connections that reach the checkout at
// projectDir, by the link state Astro Desktop applies
// to the same vault. A var so a test can count what the launch asked for.
var reachingConnections = func(projectDir string) []connmodel.Connection {
	return vaultenv.Load(projectDir).ReachingConnections()
}

// warehouseReadTimeout bounds the workspace read a launch makes for the
// warehouses, the bound Astro Desktop's warehouse read uses, so a slow API
// cannot stall an Otto spawn for long.
const warehouseReadTimeout = 8 * time.Second

// workspaceClients builds the client the workspace read uses. A var so a test
// stands in a fake Environment Manager.
var workspaceClients emenv.ClientFactory = emenv.Clients

// workspaceConnections is the native connections the v2 project at projectDir
// gets from its linked workspace, read with secret values, since a warehouse
// without credentials cannot be queried. Nil, with no read, when the manifest
// links no workspace.
func workspaceConnections(projectDir string) ([]connmodel.Connection, error) {
	m, err := manifest.Load(filepath.Join(projectDir, project.Marker))
	if err != nil || m.Astro.Workspace == "" {
		// Intended: the warehouses are best effort, and a manifest that does
		// not load links no workspace to read. A start reports the manifest
		// error itself; the launch should not fail over it here.
		return nil, nil //nolint:nilerr // no readable manifest links no workspace
	}
	return emenv.WorkspaceConnections(m.Astro.Workspace, m.Astro.WorkspaceDomain(), workspaceClients, warehouseReadTimeout)
}

// warehouseConnections is every connection Otto's warehouses come from: the
// vault's that reach the checkout, then, at the lowest precedence, the linked
// workspace's, as a start layers them and as Astro Desktop feeds its
// warehouses. A connection id the vault holds keeps the vault's. v2 is whether
// projectDir is a v2 project, which is what can link a workspace.
//
// The workspace read is bounded by warehouseReadTimeout, and
// a failure is logged and leaves those warehouses out: a launch never waits on
// or fails over the platform.
func warehouseConnections(projectDir string, v2 bool) []connmodel.Connection {
	out := reachingConnections(projectDir)
	if !v2 {
		return out
	}
	cloud, err := workspaceConnections(projectDir)
	if err != nil {
		logger.Warnf("otto: workspace connections left out of the warehouse config: %v", err)
		return out
	}
	have := make(map[string]bool, len(out))
	for i := range out {
		have[out[i].ConnID] = true
	}
	for i := range cloud {
		if !have[cloud[i].ConnID] {
			out = append(out, cloud[i])
		}
	}
	return out
}

// warehouseDir is where the analyzing-data skill reads its config. A var so a
// test writes under a temp directory rather than the real ~/.astro/agents.
var warehouseDir = connwarehouse.ConfigDir

// writeWarehouses rewrites ~/.astro/agents/warehouse.yml and .env from the
// connections that reach the checkout at cwd, so Otto's analyzing-data skill
// can query the same warehouses whichever tool launched it. The files are
// global, so they are regenerated on every launch to match where Otto runs.
//
// Best effort: a failure is logged and the launch goes on without warehouses,
// as the desktop's does.
//
// The checkout is the enclosing v2 project, or cwd itself otherwise: a v1
// project's links and scoped entries name its own directory, and a directory
// that is no project matches only the globals with no link row.
func writeWarehouses(cwd string) {
	projectDir := cwd
	v2 := false
	if proj, err := project.Discover(cwd); err == nil {
		projectDir = proj.Dir
		v2 = true
	} else {
		var notFound *project.NotFoundError
		if !errors.As(err, &notFound) {
			logger.Debugf("otto: discovering v2 project for warehouses: %v", err)
		}
	}
	dir, err := warehouseDir()
	if err != nil {
		logger.Warnf("otto: warehouse config dir: %v", err)
		return
	}
	live, skipped := connwarehouse.MaterializeAll(warehouseConnections(projectDir, v2))
	if err := connwarehouse.Write(dir, live); err != nil {
		logger.Warnf("otto: writing warehouse config: %v", err)
		return
	}
	logger.Infof("otto: warehouse config written: %d queryable, %d skipped", len(live), len(skipped))
}
