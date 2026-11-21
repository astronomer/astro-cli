//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localshared"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Commander runs foreground commands in the project environment (Run and
// Shell). It is a seam so tests never execute anything real.
type Commander interface {
	// Run runs the command with the given working dir and full environment,
	// wired to the given stdio. Nil readers/writers mean "none"/discard.
	Run(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error
}

// cancelGrace is how long a command gets to exit after a canceled run asks it
// to stop, before it is killed.
const cancelGrace = 5 * time.Second

// execCommander is the production Commander, backed by os/exec.
type execCommander struct{}

func (execCommander) Run(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // `astro local run` executes the user's own command in their own environment, the same trust as their shell
	cmd.Dir = dir
	cmd.Env = env
	// A canceled run asks the command to stop with SIGTERM instead of killing
	// it at once, so it can clean up (flush, release locks, stop its own
	// children). WaitDelay is the backstop: a command still running after it
	// is killed. The command stays in the CLI's process group, so a terminal
	// Ctrl-C reaches it directly and an interactive shell keeps job control; a
	// SIGTERM or SIGHUP sent to the CLI alone reaches it through this Cancel.
	cmd.WaitDelay = cancelGrace
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.Stdin = s.In
	cmd.Stdout = s.Out
	cmd.Stderr = s.Err
	// The environment is the whole of the project's, not an addition to the
	// CLI's, so its names would say nothing a debug reader is after.
	lg := localshared.StartCommand(ctx, nil, name, args...)
	err := cmd.Run()
	// ErrWaitDelay means the command itself exited 0 but a background child
	// kept its output open past the delay. On a run nobody canceled that is
	// success. The child is cut off from the CLI's stdout and stderr after
	// WaitDelay (5s) once the command has exited: whatever it prints later is
	// lost, though the child itself keeps running.
	if errors.Is(err, exec.ErrWaitDelay) && ctx.Err() == nil {
		err = nil
	}
	lg.End(err, nil)
	return err
}

// launchDetached is the production launchFunc. Setpgid puts the child (the
// supervisor) in its own process group: it survives the CLI's exit —
// detached is the default ownership — and stop can signal the whole group
// at once. Its stdio is deliberately empty: the supervisor owns the log
// file, and any pipe from this process would close when the CLI exits.
func launchDetached(dir string, env []string, name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		slog.Debug("launching detached failed", "cmd", localshared.CommandLine(name, args), "dir", dir, "err", err)
		return 0, err
	}
	slog.Debug("launched detached", "cmd", localshared.CommandLine(name, args), "dir", dir, "pid", cmd.Process.Pid)
	// Reap the child if it exits while the CLI is still alive; when the
	// CLI exits first (the normal detached case), init adopts the child.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // reaper goroutine; the child's exit status is not our concern
	return cmd.Process.Pid, nil
}
