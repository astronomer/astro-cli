// Package local is the v2 command tree: `astro local *`, `astro init`, the
// root aliases `astro start/stop/logs`, and the `astro dev` removal stub.
// It follows the cmd/ layer rules in docs/v2-architecture.md: parse flags,
// call one function, render output. All process state (stdio, the runtime,
// the working directory) arrives through Deps, built once in main; nothing
// here reads config at import time or holds mutable package state.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/localdocker"
	"github.com/astronomer/astro-cli/internal/localstandalone"
	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// Deps is everything the v2 commands need from the process. The composition
// root (cmd/astro/main.go today; the v1 root's main once final wiring lands)
// builds it once and hands it down.
type Deps struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Runtime is the pkg/localrt surface the commands call.
	Runtime Runtime

	// WorkingDir resolves the project path. Commands never call os.Getwd
	// themselves so tests can pin it.
	WorkingDir func() (string, error)

	// OpenURL opens a URL in the user's browser (`astro local open`).
	OpenURL func(url string) error
}

// Runtime mirrors the package-level functions of pkg/localrt as an
// interface, so commands can be tested without a real runtime.
type Runtime interface {
	Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error)
	Attach(projectPath string) (localrt.Airflow, error)
	ReadStatus(projectPath string) (localrt.Status, error)
	List() ([]localrt.Status, error)
}

// NewDeps builds the production Deps. Call it once, from main.
func NewDeps() Deps {
	return Deps{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Runtime:    newModeRuntime(),
		WorkingDir: os.Getwd,
		OpenURL:    browser.OpenURL,
	}
}

// modeRuntime is the production Runtime: it dispatches on localrt.Mode
// between the two engines, standalone (internal/localstandalone, an earlier fix)
// and docker (internal/localdocker, an earlier fix). Read paths dispatch on the
// mode the state record captured at start, so any tool stops what another
// started.
type modeRuntime struct {
	docker     *localdocker.Engine
	standalone *localstandalone.Engine
}

func newModeRuntime() modeRuntime {
	dir := routesDir()
	return modeRuntime{docker: localdocker.New(dir), standalone: localstandalone.New(dir)}
}

// routesDir is where pkg/proxy keeps routes.json: <astro home>/proxy, the
// same location v1 uses, honoring the same ASTRO_HOME override — v1 and v2
// must see each other's routes.
func routesDir() string {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".astro", "proxy")
}

func (r modeRuntime) Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error) {
	if p.Mode == "" {
		// Plan building (manifest + user state) lands in a later issue;
		// until then no --mode means the standalone default.
		p.Mode = localrt.ModeStandalone
	}
	if p.Mode == localrt.ModeDocker {
		return r.docker.Start(ctx, p, cb)
	}
	return r.standalone.Start(ctx, p, cb)
}

func (r modeRuntime) Attach(projectPath string) (localrt.Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err != nil {
		return nil, err
	}
	if rec.Mode == localrt.ModeDocker {
		return r.docker.Attach(projectPath)
	}
	return r.standalone.Attach(projectPath)
}

func (r modeRuntime) ReadStatus(projectPath string) (localrt.Status, error) {
	rec, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return localrt.Status{ProjectPath: projectPath, State: localrt.StateStopped}, nil
	}
	if err != nil {
		return localrt.Status{}, err
	}
	if rec.Mode == localrt.ModeDocker {
		return r.docker.ReadStatus(projectPath)
	}
	return r.standalone.ReadStatus(projectPath)
}

func (r modeRuntime) List() ([]localrt.Status, error) {
	recs, err := localstate.List()
	if err != nil {
		return nil, err
	}
	statuses := make([]localrt.Status, 0, len(recs))
	for _, rec := range recs {
		if rec.Mode == localrt.ModeDocker {
			statuses = append(statuses, r.docker.StatusOf(rec))
			continue
		}
		statuses = append(statuses, r.standalone.StatusOf(rec))
	}
	return statuses, nil
}

// skipPreRunAnnotation mirrors internal/telemetry.SkipPreRunAnnotation. It
// is spelled out here because v2 packages never import config/, which
// internal/telemetry pulls in. The v1 root's PersistentPreRunE checks this
// annotation on the invoked command, so `astro local` stays offline: no
// network call runs before the command does.
const skipPreRunAnnotation = "skipPreRun"

// markSkipPreRun annotates cmd and every descendant. Cobra annotations do
// not inherit, and the v1 root reads the annotation off the leaf command.
func markSkipPreRun(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipPreRunAnnotation] = "true"
	for _, sub := range cmd.Commands() {
		markSkipPreRun(sub)
	}
}

// notBuilt marks a surface whose engine or scaffold work has not landed yet
// (an earlier fix/42/43). It wraps localrt.ErrNotImplemented so callers and tests
// can detect the condition with errors.Is.
func notBuilt(what string) error {
	return fmt.Errorf("%s is %w in this build", what, localrt.ErrNotImplemented)
}

// errAborted reports a confirmation answered "no".
var errAborted = errors.New("aborted")
