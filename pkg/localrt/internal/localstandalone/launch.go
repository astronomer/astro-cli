//go:build !windows

package localstandalone

import (
	"context"
	"os/exec"
	"syscall"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Commander runs foreground commands in the project environment (Run and
// Shell). It is a seam so tests never execute anything real.
type Commander interface {
	// Run runs the command with the given working dir and full environment,
	// wired to the given stdio. Nil readers/writers mean "none"/discard.
	Run(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error
}

// execCommander is the production Commander, backed by os/exec.
type execCommander struct{}

func (execCommander) Run(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // `astro local run` executes the user's own command in their own environment, the same trust as their shell
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = s.In
	cmd.Stdout = s.Out
	cmd.Stderr = s.Err
	return cmd.Run()
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
		return 0, err
	}
	// Reap the child if it exits while the CLI is still alive; when the
	// CLI exits first (the normal detached case), init adopts the child.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // reaper goroutine; the child's exit status is not our concern
	return cmd.Process.Pid, nil
}
