//go:build !windows

package local

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// These use stoppedProject, which lifecycle_test.go builds for every platform
// but Windows.

// A restart the watch asks for after Airflow went down is a fresh start, and
// still the watch's: tied to the session, on the watch's port, and in its
// mode when no record survives to name one.
func TestWatchRestartOfAStoppedProjectKeepsTheWatchsStart(t *testing.T) {
	for _, mode := range []localrt.Mode{localrt.ModeStandalone, localrt.ModeDocker} {
		d, _, calls := stoppedProject(t, "")
		c := &cli{d: d, output: cliout.FormatText}
		_, err := c.restartProject(context.Background(), restartOptions{stopWithSession: true, port: 8123, mode: mode})
		require.ErrorIs(t, err, localrt.ErrNotImplemented, "the fake runtime refuses the start it was asked for")
		require.True(t, calls.started.StopWithSession, "%s: the start is tied to the watch", mode)
		require.Equal(t, 8123, calls.started.RequestedPort, mode)
		require.Equal(t, mode, calls.started.Mode)
	}
}

// --watch with nothing to watch with is refused before Airflow starts, not
// after a start it would leave behind.
func TestStartWatchWithoutAWatcherRefusesBeforeStarting(t *testing.T) {
	d, _, calls := stoppedProject(t, "")
	err := execute(t, d, "local", "start", "--watch")
	require.ErrorIs(t, err, errWatchUnavailable)
	require.Empty(t, calls.started.ProjectPath, "nothing was started")
}
