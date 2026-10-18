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
var errNoDocker = errors.New("an image deploy needs Docker, but no running container engine was found. Start Docker and try again, or run 'astro deploy --dags' to deploy just your Dags (no Docker needed). Server-side builds are coming")

// errNoResolver is a caller's mistake: EnsureEngine was given no way to pick
// the container engine.
var errNoResolver = errors.New("an image deploy could not pick a container engine: no engine resolver was given")

// ErrEngineUnreachable reports that ReachEngine found no container engine to
// use: none is installed, or its daemon does not answer. What to tell the
// user is the caller's, so it carries no advice.
var ErrEngineUnreachable = errors.New("no running container engine was found")

// ReachEngine resolves the container CLI through resolve and confirms its
// daemon is reachable through cmd, returning the engine every later image
// action runs on. It is the engine check an image deploy (EnsureEngine) and
// `astro package` (internal/pack) share; each words its own failure.
//
// It fails with resolve's own error when that is container.ErrMachineNotRunning
// (podman with no machine up, whose message names the command that brings one
// up), and with ErrEngineUnreachable for any other way of finding no engine.
func ReachEngine(ctx context.Context, cmd imagebuild.Commander, resolve func() (string, []string, error)) (imagebuild.Engine, error) {
	bin, env, err := resolve()
	if errors.Is(err, container.ErrMachineNotRunning) {
		return imagebuild.Engine{}, err
	}
	if err != nil {
		return imagebuild.Engine{}, ErrEngineUnreachable
	}
	// `info` needs the daemon, so it fails fast when the engine is installed but
	// not running.
	if err := cmd.Run(ctx, env, localrt.Stdio{}, bin, "info"); err != nil {
		return imagebuild.Engine{}, ErrEngineUnreachable
	}
	return imagebuild.Engine{Cmd: cmd, Bin: bin, Env: env}, nil
}

// EnsureEngine resolves the container CLI through resolve and confirms its
// daemon is reachable through cmd (ReachEngine), so an image deploy fails with
// a plain message before any transport work when Docker is absent or down —
// or, for podman with no machine up, with the podman command that brings one
// up, which "Start Docker" would not be.
//
// resolve is required: picking the engine reads the CLI's config, which this
// package does not, so the caller supplies it. A nil resolve is refused with
// errNoResolver rather than guessed at.
func EnsureEngine(ctx context.Context, cmd imagebuild.Commander, resolve func() (string, []string, error)) (bin string, env []string, err error) {
	if resolve == nil {
		return "", nil, errNoResolver
	}
	engine, err := ReachEngine(ctx, cmd, resolve)
	if errors.Is(err, container.ErrMachineNotRunning) {
		return "", nil, fmt.Errorf("an image deploy needs a running container engine, but %w. Or run 'astro deploy --dags' to deploy just your Dags (no container engine needed)", err)
	}
	if err != nil {
		return "", nil, errNoDocker
	}
	return engine.Bin, engine.Env, nil
}
