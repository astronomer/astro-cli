//go:build windows

// Windows stub: the MVP runs local Airflow on Windows in docker mode only
// (docs/v2-architecture.md, "Defaults" — decision 12). The API matches the
// Unix implementation so callers compile without build tags; anything that
// would need a standalone process returns ErrWindowsUnsupported.
package localstandalone

import (
	"context"
	"errors"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// ErrWindowsUnsupported reports that standalone mode does not run on
// Windows; docker mode does.
var ErrWindowsUnsupported = errors.New("standalone mode is not supported on Windows; run local Airflow in containers instead: astro local start --docker")

// ErrNotStandaloneMode reports a record this engine does not own.
var ErrNotStandaloneMode = errors.New("this project's local Airflow is not running in standalone mode")

// Engine is the Windows stub.
type Engine struct{}

// New matches the Unix constructor.
func New(_ string, _ rt.ProxyDaemon) *Engine { return &Engine{} }

func (e *Engine) Start(_ context.Context, _ rt.Plan, _ rt.Callbacks) (rt.Airflow, error) {
	return nil, ErrWindowsUnsupported
}

func (e *Engine) Attach(_ string) (rt.Airflow, error) {
	return nil, ErrWindowsUnsupported
}

// LogHandle matches the Unix signature; standalone logs never exist on
// Windows, where local Airflow runs in docker mode.
func (e *Engine) LogHandle(_ string) (rt.Airflow, error) {
	return nil, ErrWindowsUnsupported
}

func (e *Engine) ReadStatus(_ string) (rt.Status, error) {
	return rt.Status{}, ErrWindowsUnsupported
}

// StatusOf reports a standalone record seen from Windows as not running:
// the process lives on another OS's machine only in theory, never here.
func (e *Engine) StatusOf(rec localstate.Record) rt.Status {
	return rec.Status(false)
}

// Clean succeeds having done nothing, rather than returning
// ErrWindowsUnsupported like the rest of this stub.
//
// The others are asked to DO something standalone, and refusing is the answer.
// This one is asked whether any standalone leftovers need removing, and on
// Windows the answer is no — standalone never ran, so it left nothing. Failing
// here would break `astro local reset` on Windows for a docker-mode project,
// which is the only kind Windows has.
func (e *Engine) Clean(_ string) error { return nil }
