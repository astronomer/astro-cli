package otto

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

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

	// Refresh the cached latest version up-front so autoUpdate / hint paths
	// see today's release, not yesterday's. Previously this ran post-autoUpdate
	// in a goroutine, which meant the very first launch after a release missed
	// the new version (cache only caught up by the *next* invocation). Bounded
	// by LatestVersion's 5s timeout; offline launches soft-fall-through to the
	// previously cached value.
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

	// Install the `af` wrapper alongside Otto. Best-effort: if this fails
	// (disk full, permissions, etc.), Otto falls back to the full
	// `uvx --from ...` prefix at runtime.
	if err := EnsureAfWrapper(); err != nil {
		logger.Warnf("otto: failed to install af wrapper: %v (will fall back to uvx)", err)
	}

	env := cfg.BuildEnv()

	cmd := exec.Command(BinaryPath(), args...) //nolint:gosec // forwarding user args to the Otto binary is the whole point of this command
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
