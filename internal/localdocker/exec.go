package localdocker

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/astronomer/astro-cli/pkg/localrt"
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
	Run(ctx context.Context, extraEnv []string, s localrt.Stdio, name string, args ...string) error
}

// execCommander is the production Commander, backed by os/exec.
type execCommander struct{}

func (execCommander) Output(ctx context.Context, extraEnv []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return out.Bytes(), err
}

func (execCommander) Run(ctx context.Context, extraEnv []string, s localrt.Stdio, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = s.In
	cmd.Stdout = s.Out
	cmd.Stderr = s.Err
	return cmd.Run()
}
