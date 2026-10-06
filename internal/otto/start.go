package otto

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/astronomer/astro-cli/pkg/connwarehouse"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// ErrNotLoggedIn signals that the user invoked `astro otto` without an auth
// context. ottoRun intercepts this and exits silently so cobra doesn't print
// "Error: ..." on top of the guidance we already wrote to stderr.
var ErrNotLoggedIn = errors.New("not logged in")

// isHelpOrVersion reports whether args is a help- or version-only invocation.
// Otto's --help / --version exit before bootstrap() runs (see otto cli.ts),
// so they don't need an auth context — let them through even when logged out.
func isHelpOrVersion(args []string) bool {
	for _, a := range args {
		switch a {
		case "--help", "-h", "--version":
			return true
		}
	}
	return false
}

// Start spawns Otto with the given arguments and environment from the current context.
// It blocks until Otto exits, forwarding signals for clean shutdown.
func Start(args []string) error {
	cfg := NewConfigFromContext()
	if cfg.Token == "" && !isHelpOrVersion(args) {
		fmt.Fprintln(os.Stderr, "You're not logged in to Astro. Otto is an AI assistant for Airflow — sign in or start a trial to use it.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  • Sign in:        astro login")
		fmt.Fprintln(os.Stderr, "  • Free trial:     https://www.astronomer.io/try-astro")
		fmt.Fprintln(os.Stderr, "  • What Otto does: astro otto --help")
		return ErrNotLoggedIn
	}

	if err := EnsureBinary(); err != nil {
		return fmt.Errorf("setting up otto: %w", err)
	}

	// Refresh the cached latest version before autoUpdate and hintUpdateAvailable
	// read it, so a launch on release day sees today's version. Bounded by
	// LatestVersion's 5s timeout; offline launches keep the previous cache.
	refreshUpdateCacheIfStale()

	// Apply auto-update or fall back to the hint. Both must run before
	// redirectCLILogs — post-redirect, writes would land in a log file the
	// user never opens. autoUpdate downloads synchronously when the cache
	// knows of a newer version; failures soft-fail to the installed binary.
	if autoUpdateEnabled() {
		autoUpdate(os.Stderr, downloadAndInstall)
	} else {
		hintUpdateAvailable(os.Stderr)
	}

	// Otto is a TUI — CLI-side writes to stderr corrupt its rendering. Route
	// the CLI's own logger to a file for the lifetime of the session.
	if closer, err := redirectCLILogs(); err != nil {
		// Logging redirection is best-effort. If it fails (permissions, disk
		// full), silence the logger entirely — a broken TUI is worse than
		// dropped log lines.
		logger.SetOutput(io.Discard)
	} else {
		defer closer.Close()
	}

	// Deferred from NewConfigFromContext: detection health-probes local
	// ports, so it runs only once the launch is definitely spawning Otto.
	cfg.AirflowURL, cfg.ProjectAirflow = DetectAirflow()

	// Help and version exit before Otto reads anything, so they neither need
	// the warehouses nor should open the keychain for them. The warehouses'
	// secrets travel in Otto's environment, which its analyzing-data kernel
	// inherits, rather than in a file.
	var warehouses []connwarehouse.Materialized
	if !isHelpOrVersion(args) {
		if cwd, err := os.Getwd(); err == nil {
			warehouses = writeWarehouses(cwd)
		}
	}

	return spawnOtto(BinaryPath(), args, connwarehouse.AppendEnv(cfg.BuildEnv(), warehouses))
}

// spawnOtto runs the Otto binary in the foreground and waits for it. A var so
// a test can take the launch's arguments and environment without running Otto.
var spawnOtto = func(bin string, args, env []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	forwardSignals(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting otto: %w", err)
	}

	return cmd.Wait()
}
