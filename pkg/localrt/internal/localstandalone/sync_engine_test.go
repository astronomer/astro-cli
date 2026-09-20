//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// Sync is Start's first step alone, so what it forwards to uv is the whole
// behavior. The faked syncer is why this is cheap; without it the only Sync
// tests were the façade's refusals, every one of which a Sync that provisioned
// nothing would also satisfy.
func TestEngineSyncProvisionsWithThePlansInterpreter(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()

	var gotProject, gotPython string
	var called bool
	e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
		return syncerFunc(func(_ context.Context, p, python string, _ uv.Stdio) error {
			called, gotProject, gotPython = true, p, python
			return nil
		}), nil
	}

	require.NoError(t, e.Sync(context.Background(), rt.Plan{
		ProjectPath:   project,
		Mode:          rt.ModeStandalone,
		PythonVersion: "3.12",
	}, rt.Callbacks{}))

	require.True(t, called, "Sync provisioned nothing")
	assert.Equal(t, project, gotProject)
	assert.Equal(t, "3.12", gotPython, "the interpreter the plan asked for did not reach uv")
}

// A consumer wiring one set of Callbacks for both Start and Sync must not be
// left reading silence as either outcome — the reason Start emits these.
func TestEngineSyncReportsItsOutcome(t *testing.T) {
	for _, tc := range []struct {
		name    string
		syncErr error
		want    []rt.State
	}{
		{"a provision that works", nil, []rt.State{rt.StateStarting}},
		{"one that fails", errors.New("no solution found"), []rt.State{rt.StateStarting, rt.StateError}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
				return syncerFunc(func(context.Context, string, string, uv.Stdio) error {
					return tc.syncErr
				}), nil
			}

			var states []rt.State
			err := e.Sync(context.Background(), rt.Plan{ProjectPath: t.TempDir(), Mode: rt.ModeStandalone},
				rt.Callbacks{OnState: func(s rt.State, _ error) { states = append(states, s) }})

			if tc.syncErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, tc.want, states)
		})
	}
}

// An empty Mode is what a plan built for a command that asked for no
// particular runtime carries, and Start treats it as standalone. A docker
// plan arriving here is a dispatch bug and says so.
func TestEngineSyncOnModes(t *testing.T) {
	e, _, _ := testEngine(t)
	var called bool
	e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
		return syncerFunc(func(context.Context, string, string, uv.Stdio) error {
			called = true
			return nil
		}), nil
	}

	require.NoError(t, e.Sync(context.Background(), rt.Plan{ProjectPath: t.TempDir()}, rt.Callbacks{}))
	assert.True(t, called, "an empty Mode was refused; Start defaults it to standalone")

	err := e.Sync(context.Background(), rt.Plan{ProjectPath: t.TempDir(), Mode: rt.ModeDocker}, rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker")
}
