package agent

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/astronomer/astro-cli/pkg/logger"
)

// Start spawns Otto with the given arguments and environment from the current context.
// It blocks until Otto exits, forwarding signals for clean shutdown.
func Start(args []string) error {
	cfg := NewConfigFromContext()
	if cfg.Token == "" {
		return fmt.Errorf("not logged in. Run `astro login` first")
	}

	if err := EnsureBinary(); err != nil {
		return fmt.Errorf("setting up otto: %w", err)
	}

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
	// the CLI's own logger to a file for the lifetime of the agent.
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
		logger.Warnf("agent: failed to install af wrapper: %v (will fall back to uvx)", err)
	}

	// Refresh the cached latest-version in the background so the next launch
	// prints an up-to-date hint. Fire-and-forget; the goroutine lives for the
	// duration of the Otto session via cmd.Wait() below.
	go refreshUpdateCacheAsync()

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
