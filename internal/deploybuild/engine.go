package deploybuild

import (
	"context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// errNoDocker is the plain, actionable message for the no-Docker user
// (docs/deploy.md, section 5). An image deploy needs a container builder;
// dags-only does not.
var errNoDocker = errors.New("an image deploy needs Docker, but no running container engine was found. Start Docker and try again, or run 'astro deploy --dags' to deploy just your DAGs (no Docker needed). Server-side builds are coming")

// errNoResolver is a caller's mistake: EnsureEngine was given no way to pick
// the container engine.
var errNoResolver = errors.New("an image deploy could not pick a container engine: no engine resolver was given")

// EnsureEngine resolves the container CLI through resolve and confirms its
// daemon is reachable through cmd, so an image deploy fails with a plain
// message before any transport work when Docker is absent or down — or, for
// podman with no machine up, with the podman command that brings one up, which
// "Start Docker" would not be.
//
// resolve is required: picking the engine reads the CLI's config, which this
// package does not, so the caller supplies it. A nil resolve is refused with
// errNoResolver rather than guessed at.
func EnsureEngine(ctx context.Context, cmd imagebuild.Commander, resolve func() (string, []string, error)) (bin string, env []string, err error) {
	if resolve == nil {
		return "", nil, errNoResolver
	}
	bin, env, err = resolve()
	if errors.Is(err, container.ErrMachineNotRunning) {
		return "", nil, fmt.Errorf("an image deploy needs a running container engine, but %w. Or run 'astro deploy --dags' to deploy just your DAGs (no container engine needed)", err)
	}
	if err != nil {
		return "", nil, errNoDocker
	}
	// `info` needs the daemon, so it fails fast when the engine is installed but
	// not running.
	if err := cmd.Run(ctx, env, localrt.Stdio{}, bin, "info"); err != nil {
		return "", nil, errNoDocker
	}
	return bin, env, nil
}
