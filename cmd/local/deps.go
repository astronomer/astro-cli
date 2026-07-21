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

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

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
		Runtime:    localrtRuntime{},
		WorkingDir: os.Getwd,
		OpenURL:    browser.OpenURL,
	}
}

// localrtRuntime is the production Runtime: straight delegation to
// pkg/localrt.
type localrtRuntime struct{}

func (localrtRuntime) Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error) {
	return localrt.Start(ctx, p, cb)
}

func (localrtRuntime) Attach(projectPath string) (localrt.Airflow, error) {
	return localrt.Attach(projectPath)
}

func (localrtRuntime) ReadStatus(projectPath string) (localrt.Status, error) {
	return localrt.ReadStatus(projectPath)
}

func (localrtRuntime) List() ([]localrt.Status, error) {
	return localrt.List()
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
