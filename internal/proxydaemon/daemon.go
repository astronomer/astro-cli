package proxydaemon

import (
	"os"

	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/version"
)

// daemon is this CLI's proxy daemon: its own binary, re-executed into
// ServeSubcommand, serving the routes under dir (Dir in production). The
// lifecycle is pkg/proxy's, shared with every other tool that serves those
// routes, and so is the astro 1.x takeover before a start; what is this CLI's
// is the executable and its version.
//
// Built per call, like Routes, so a test that sets version.CurrVersion is seen
// by the next call.
func daemon(dir string) *pkgproxy.Daemon {
	exe, err := os.Executable()
	if err != nil {
		// Start reports the missing executable when it is needed; a caller
		// that only reads the record has no use for one.
		logger.Debugf("cannot find the CLI executable for the proxy daemon: %s", err)
	}
	return &pkgproxy.Daemon{
		Store:     Routes(dir),
		Exe:       exe,
		ServeArgs: []string{ServeSubcommand},
		Version:   version.CurrVersion,
		Logf:      logger.Debugf,
	}
}

// EnsureRunning starts the proxy daemon serving the routes under dir if it is
// not already running, and returns the port it is listening on. See
// pkg/proxy's Daemon.EnsureRunning.
func EnsureRunning(dir, port string) (string, error) {
	return daemon(dir).EnsureRunning(port)
}

// StopIfEmpty stops the proxy daemon for dir when no route needs it any more.
func StopIfEmpty(dir string) {
	daemon(dir).StopIfEmpty()
}

// BoundPort returns the port the daemon for dir reported at bind time, or ""
// when unknown.
func BoundPort(dir string) string {
	return daemon(dir).BoundPort()
}

// Serve runs the proxy server for dir in the foreground. It is the body of the
// hidden ServeSubcommand the daemon runs as.
func Serve(dir, port string) error {
	return daemon(dir).Serve(port)
}
