package localdocker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// ErrComposeMissing reports that the engine has no Compose v2 plugin, which
// surfaces from the raw engine as an opaque "exit status 125" / "unknown flag:
// --file". The message points at the fix instead.
var ErrComposeMissing = errors.New("Docker Compose v2 is required but was not found; install the Compose plugin (it ships with Docker Desktop and OrbStack): https://docs.docker.com/compose/install/")

// ensureEngineUp brings the resolved container engine up before the start
// touches it: v1 auto-started a stopped Docker daemon (open -a docker on Mac),
// and v2 resolved the engine by binary presence alone, so `start --docker` with
// the daemon down failed immediately. This restores the Docker half of the v1
// path — a bounded wait with a clear message, failing only when the engine
// cannot come up. Podman machines are the user's to run: for podman this only
// checks one is up, and names `podman machine start` when it is not. Progress
// flows through the callbacks as "engine" lines; the container package itself
// never prints.
func ensureEngineUp(cb rt.Callbacks, now func() time.Time, binary string) error {
	engine, err := container.GetContainerRuntime(container.Config{Binary: binary}, callbackFeedback{cb: cb, now: now})
	if err != nil {
		return err
	}
	if err := engine.Initialize(); err != nil {
		return fmt.Errorf("could not start the container engine: %w", err)
	}
	return nil
}

// composeInfo is what probeCompose learns about the engine's compose beyond
// its being there.
type composeInfo struct {
	// envSecrets reports whether it can create a secret from an environment
	// variable, which decides how a start delivers SecretEnv (planHandoff).
	envSecrets bool
}

// probeCompose probes the engine's Compose v2 plugin with `compose version`, a
// client-only check that does not touch the daemon, so a missing plugin is
// reported as ErrComposeMissing before any compose command runs.
//
// The same output says which compose answered. Docker Compose has created
// secrets from the environment since v2.6.1. `podman compose` runs whichever
// provider it finds, Docker Compose first; the other, podman-compose, refuses
// an environment-sourced secret at runtime ("not supported for runtime
// secrets in podman-compose", as of 1.6.0), and so does anything that does not
// say it is Docker Compose. One call answers both, so a start pays for one
// (`podman compose` takes a few hundred milliseconds to find its provider).
func (e *Engine) probeCompose(ctx context.Context, conn engineConn) (composeInfo, error) {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "compose", "version")
	if err != nil {
		return composeInfo{}, ErrComposeMissing
	}
	return composeInfo{envSecrets: composeTakesEnvSecrets(string(out))}, nil
}

// composeVersionRe finds Docker Compose's own version line, which `podman
// compose` prints after a banner naming the provider it runs.
var composeVersionRe = regexp.MustCompile(`Docker Compose version v?(\d+)\.(\d+)\.(\d+)`)

// composeTakesEnvSecrets reads `compose version` output: Docker Compose at
// v2.6.1 or later.
//
// It matches the "Docker Compose version" line rather than asking for
// `--short` or `--format json`, because those give a number and nothing that
// says whose: podman-compose answers them too, and a podman-compose that
// reached 2.6.1 would pass a check on the number alone and fail every start.
// A line it does not recognize reads as no, which costs the handoff, not the
// start.
func composeTakesEnvSecrets(versionOutput string) bool {
	m := composeVersionRe.FindStringSubmatch(versionOutput)
	if m == nil {
		return false
	}
	v := [3]int{}
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1]) //nolint:errcheck // the regexp matched digits only
	}
	minimum := [3]int{2, 6, 1}
	for i := range v {
		if v[i] != minimum[i] {
			return v[i] > minimum[i]
		}
	}
	return true
}

// callbackFeedback adapts container.Feedback onto rt.Callbacks so the
// engine auto-start's progress reaches the frontend without the container
// package printing anything.
type callbackFeedback struct {
	cb  rt.Callbacks
	now func() time.Time
}

func (f callbackFeedback) emit(message string) {
	if f.cb.OnLine != nil && message != "" {
		f.cb.OnLine(rt.LogLine{Component: "engine", Time: f.now(), Text: message})
	}
}

func (f callbackFeedback) Start(message string)   { f.emit(message) }
func (f callbackFeedback) Update(message string)  { f.emit(message) }
func (f callbackFeedback) Success(message string) { f.emit(message) }
func (f callbackFeedback) Stop()                  {}
