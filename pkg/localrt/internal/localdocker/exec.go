package localdocker

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Commander runs external commands. It is the engine's only path to a
// container runtime, so tests substitute a fake and never touch a real
// daemon. extraEnv is appended to the process environment (e.g. the
// DOCKER_HOST/CONTAINER_HOST pair that reaches a podman machine).
type Commander interface {
	// Output runs the command and returns its stdout.
	Output(ctx context.Context, extraEnv []string, name string, args ...string) ([]byte, error)
	// Run runs the command wired to the given stdio. Nil readers/writers
	// are allowed and mean "none"/discard.
	Run(ctx context.Context, extraEnv []string, s rt.Stdio, name string, args ...string) error
}

// execCommander is the production Commander, backed by os/exec.
type execCommander struct{}

// waitDelay bounds the wait AFTER a context has already done its work: a child
// that ignores the kill, and — the one that matters here — a child that exits
// but leaves its I/O pipes open in a descendant.
//
// Without it a deadline is advisory. Neither call below hands the child an
// *os.File, so os/exec makes pipes and copying goroutines, and Wait does not
// return until every writer of those pipes is closed. A killed engine CLI that
// forked a helper holding the inherited fd leaves Run blocked for good, past
// the deadline that was supposed to be the whole guarantee.
const waitDelay = 5 * time.Second

func (execCommander) Output(ctx context.Context, extraEnv []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.WaitDelay = waitDelay
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return out.Bytes(), err
}

func (execCommander) Run(ctx context.Context, extraEnv []string, s rt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	// Harmless to a healthy long-running call: the clock starts only once the
	// context is done or the process has exited, so a `logs --follow` streams
	// for as long as its caller wants and only its teardown is bounded.
	cmd.WaitDelay = waitDelay
	// SIGTERM rather than the default SIGKILL. `docker compose` and `podman
	// compose` run the compose plugin or provider as a child, and a killed
	// CLI cannot pass anything on, so the child kept driving the up while
	// the rollback took the project down. A terminated one forwards the
	// signal and waits for the child. WaitDelay is the backstop: a CLI that
	// has not exited by then is killed. Windows has no SIGTERM to send, so
	// there Signal fails and this kills, as before.
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.Stdin = s.In
	cmd.Stdout = s.Out
	cmd.Stderr = s.Err
	return cmd.Run()
}
