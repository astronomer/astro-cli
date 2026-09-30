package otto

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/connwarehouse"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// reachingConnections is the vault's connections that reach the checkout at
// projectDir, by the link state Astro Desktop applies
// to the same vault. A var so a test can count what the launch asked for.
var reachingConnections = func(projectDir string) []connmodel.Connection {
	return vaultenv.Load(projectDir).ReachingConnections()
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
	if proj, err := project.Discover(cwd); err == nil {
		projectDir = proj.Dir
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
	live, skipped := connwarehouse.MaterializeAll(reachingConnections(projectDir))
	if err := connwarehouse.Write(dir, live); err != nil {
		logger.Warnf("otto: writing warehouse config: %v", err)
		return
	}
	logger.Infof("otto: warehouse config written: %d queryable, %d skipped", len(live), len(skipped))
}
