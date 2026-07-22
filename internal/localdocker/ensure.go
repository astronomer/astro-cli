package localdocker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// ErrComposeMissing reports that the engine has no Compose v2 plugin, which
// surfaces from the raw engine as an opaque "exit status 125" / "unknown flag:
// --file". The message points at the fix instead.
var ErrComposeMissing = errors.New("Docker Compose v2 is required but was not found; install the Compose plugin (it ships with Docker Desktop and OrbStack): https://docs.docker.com/compose/install/")

// ensureEngineUp brings the resolved container engine up before the start
// touches it: v1 auto-started a stopped Docker daemon (open -a docker on Mac)
// or the podman machine, and v2 resolved the engine by binary presence alone,
// so `start --docker` with the daemon down failed immediately. This restores
// the v1 path — a bounded wait with a clear message, failing only when the
// engine cannot come up. Progress flows through the callbacks as "engine"
// lines; the container package itself never prints.
func ensureEngineUp(cb localrt.Callbacks, now func() time.Time) error {
	rt, err := container.GetContainerRuntime(container.Config{}, callbackFeedback{cb: cb, now: now})
	if err != nil {
		return err
	}
	if err := rt.Initialize(); err != nil {
		return fmt.Errorf("could not start the container engine: %w", err)
	}
	return nil
}

// composeAvailable probes the engine's Compose v2 plugin with `compose
// version`, a client-only check that does not touch the daemon, so a missing
// plugin is reported as ErrComposeMissing before any compose command runs.
func (e *Engine) probeCompose(ctx context.Context, conn engineConn) error {
	if _, err := e.cmd.Output(ctx, conn.env, conn.bin, "compose", "version"); err != nil {
		return ErrComposeMissing
	}
	return nil
}

// callbackFeedback adapts container.Feedback onto localrt.Callbacks so the
// engine auto-start's progress reaches the frontend without the container
// package printing anything.
type callbackFeedback struct {
	cb  localrt.Callbacks
	now func() time.Time
}

func (f callbackFeedback) emit(message string) {
	if f.cb.OnLine != nil && message != "" {
		f.cb.OnLine(localrt.LogLine{Component: "engine", Time: f.now(), Text: message})
	}
}

func (f callbackFeedback) Start(message string)   { f.emit(message) }
func (f callbackFeedback) Update(message string)  { f.emit(message) }
func (f callbackFeedback) Success(message string) { f.emit(message) }
func (f callbackFeedback) Stop()                  {}
