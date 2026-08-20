package localdocker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstandalone/supervise"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
)

// Session-tied docker mode ties the compose project's lifetime to the process
// that started it: --stop-with-session means the containers must not outlive
// that session. Containers run in the engine daemon, not as a child of the
// starter, so there is nothing to reap when the starter dies — the record used
// to note the starter's PID and nothing acted on it. A detached watcher fixes
// that: it survives the starter (its own process group), waits for the
// starter's PID to exit, then stops the project. This mirrors standalone's
// supervisor parent-watch; the difference is the action on parent death —
// `compose down` rather than signaling a child group.
//
// The watcher is only meaningful while the starting process stays alive. A
// plain `astro local start --stop-with-session` returns and its process exits
// right away, so the watcher stops the project almost at once — the same shape
// standalone has. The flag earns its keep when the caller is long-lived (the
// desktop app), which is what it is for.

// SessionWatchSubcommand is the hidden CLI subcommand the watcher runs as.
// cmd/local registers it; the engine spawns it. Never typed by users.
const SessionWatchSubcommand = "__local-session-watch"

const (
	sessionParentFlagName  = "parent-pid"
	sessionProjectFlagName = "project"
)

// SessionParentPIDFlag and SessionProjectFlag are the flag tokens the engine
// passes and cmd/local parses. Derived from the names so the two cannot drift.
const (
	SessionParentPIDFlag = "--" + sessionParentFlagName
	SessionProjectFlag   = "--" + sessionProjectFlagName
)

// WatchAndStop blocks until parentPID exits, then stops the project's compose
// stack. It is what the hidden watcher subcommand runs. A project already gone
// (stopped by hand before the session ended) is a no-op, not an error.
func (e *Engine) WatchAndStop(ctx context.Context, projectPath string, parentPID int) error {
	supervise.WaitForParent(parentPID)
	af, err := e.Attach(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	return af.Stop(ctx, rt.StopOptions{})
}

// spawnSessionWatcher launches the watcher as a detached `astro
// __local-session-watch` process: its own process group (so a Ctrl-C on the
// starter's group does not take it down before it can clean up) and no stdio
// (the CLI it descends from exits). It inherits the environment, so an
// ASTRO_HOME/XDG sandbox carries through to the clean-up.
func spawnSessionWatcher(projectPath string, parentPID int) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving the astro binary for the session watcher: %w", err)
	}
	cmd := exec.Command(exe, SessionWatchSubcommand,
		SessionParentPIDFlag, strconv.Itoa(parentPID),
		SessionProjectFlag, projectPath)
	cmd.SysProcAttr = detachSysProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the watcher if it exits while the CLI is still alive; once the CLI
	// exits (the normal case) init adopts it.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // reaper goroutine; the watcher's exit status is not our concern
	return nil
}
