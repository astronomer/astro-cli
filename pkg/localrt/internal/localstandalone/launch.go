//go:build !windows

package localstandalone

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

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
//
// secretPayload, when set, goes to the child on a pipe it inherits as
// secretEnvFD, which superviseArgs told it to read. The pipe is the handoff
// shared with docker mode (internal/secretenv): the values reach Airflow's
// environment without passing through the supervisor's, and a pipe is read
// once and gone. The write finishes before this returns, so the child holds
// the values before the CLI can exit.
func launchDetached(dir string, env []string, secretPayload []byte, name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var w *os.File
	if secretPayload != nil {
		r, pw, err := os.Pipe()
		if err != nil {
			return 0, fmt.Errorf("creating the secret handoff pipe: %w", err)
		}
		// The parent's read end closes either way: the child has its own copy,
		// and holding ours would keep the pipe open past a child that died.
		defer func() { _ = r.Close() }()
		w = pw
		cmd.ExtraFiles = []*os.File{r} // fd 3 in the child: secretEnvFD
	}
	if err := cmd.Start(); err != nil {
		if w != nil {
			_ = w.Close()
		}
		return 0, err
	}
	if w != nil {
		if err := writeSecretPayload(w, secretPayload); err != nil {
			// A supervisor that never took its values would start Airflow
			// without them. End the whole group now: no record names it yet.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // best effort: the group may already be gone, and the write error is the one to report
			_ = cmd.Wait()                                      //nolint:errcheck // reaping the child just killed; its exit status is the kill
			return 0, err
		}
	}
	// Reap the child if it exits while the CLI is still alive; when the
	// CLI exits first (the normal detached case), init adopts the child.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // reaper goroutine; the child's exit status is not our concern
	return cmd.Process.Pid, nil
}

// secretWriteTimeout bounds the handoff write. The supervisor reads the pipe
// before anything else it does, so a write still blocked after this is one
// whose reader is not coming.
const secretWriteTimeout = 30 * time.Second

// writeSecretPayload writes the payload and closes the pipe, which is the
// child's end-of-file. A child that exited first fails the write with EPIPE
// rather than a signal: Go raises SIGPIPE only for stdout and stderr.
func writeSecretPayload(w *os.File, payload []byte) error {
	defer func() { _ = w.Close() }()
	_ = w.SetWriteDeadline(time.Now().Add(secretWriteTimeout)) //nolint:errcheck // a pipe that cannot take a deadline still takes the write; the bound is a backstop
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("handing secret values to the supervisor: %w", err)
	}
	return nil
}
