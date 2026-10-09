package proxy

import (
	"os"

	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/version"
)

// daemon is this CLI's proxy daemon: its own binary, re-executed into
// ServeSubcommand, serving the routes under ~/.astro/proxy. The lifecycle is
// pkg/proxy's, shared with every other tool that serves those routes; what is
// this CLI's is the executable, its version, and the astro 1.x takeover that
// runs before a start.
//
// Built per call, like Routes, so a test that points config.HomeConfigPath at
// a temp dir or sets version.CurrVersion is seen by the next call.
func daemon() *pkgproxy.Daemon {
	exe, err := os.Executable()
	if err != nil {
		// Start reports the missing executable when it is needed; a caller
		// that only reads the record has no use for one.
		logger.Debugf("cannot find the CLI executable for the proxy daemon: %s", err)
	}
	return &pkgproxy.Daemon{
		Store:       Routes(),
		Exe:         exe,
		ServeArgs:   []string{ServeSubcommand},
		Version:     version.CurrVersion,
		BeforeStart: beforeStart,
		Logf:        logger.Debugf,
	}
}

// EnsureRunning starts the proxy daemon if it is not already running, and
// returns the port it is listening on. See pkg/proxy's Daemon.EnsureRunning.
func EnsureRunning(port string) (string, error) {
	return daemon().EnsureRunning(port)
}

// StopIfEmpty stops the proxy daemon when no route needs it any more.
func StopIfEmpty() {
	daemon().StopIfEmpty()
}

// BoundPort returns the port the running daemon reported at bind time, or ""
// when unknown.
func BoundPort() string {
	return daemon().BoundPort()
}

// Serve runs the proxy server in the foreground. It is the body of the hidden
// ServeSubcommand the daemon runs as.
func Serve(port string) error {
	return daemon().Serve(port)
}
