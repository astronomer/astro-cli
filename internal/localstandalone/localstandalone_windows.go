//go:build windows

// Windows stub: the MVP runs local Airflow on Windows in docker mode only
// (docs/v2-architecture.md, "Defaults" — decision 12). The API matches the
// Unix implementation so callers compile without build tags; anything that
// would need a standalone process returns ErrWindowsUnsupported.
package localstandalone

import (
	"context"
	"errors"

	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// ErrWindowsUnsupported reports that standalone mode does not run on
// Windows; docker mode does.
var ErrWindowsUnsupported = errors.New("standalone mode is not supported on Windows; run local Airflow in containers instead: astro local start --mode docker")

// ErrNotStandaloneMode reports a record this engine does not own.
var ErrNotStandaloneMode = errors.New("this project's local Airflow is not running in standalone mode")

// Engine is the Windows stub.
type Engine struct{}

// New matches the Unix constructor.
func New(_ string) *Engine { return &Engine{} }

func (e *Engine) Start(_ context.Context, _ localrt.Plan, _ localrt.Callbacks) (localrt.Airflow, error) {
	return nil, ErrWindowsUnsupported
}

func (e *Engine) Attach(_ string) (localrt.Airflow, error) {
	return nil, ErrWindowsUnsupported
}

func (e *Engine) ReadStatus(_ string) (localrt.Status, error) {
	return localrt.Status{}, ErrWindowsUnsupported
}

// StatusOf reports a standalone record seen from Windows as not running:
// the process lives on another OS's machine only in theory, never here.
func (e *Engine) StatusOf(rec localstate.Record) localrt.Status {
	return rec.Status(false)
}
