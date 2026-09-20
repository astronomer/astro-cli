//go:build !windows

// Same constraint as runtime_test.go, and only for this one case: judging a
// standalone record alive means signaling a real process group. The rest of
// the reap's coverage is in reapafterclean_test.go, unconstrained, so the
// wiring is still checked on the Windows job.

package localrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// A live record keeps its route, and nothing asks the proxy to stand down.
//
// The sweep that removes nothing is the one with no fresh count to act on, and
// asking anyway is what used to reach the store with the wrong prune
// predicate. There is no route to remove here and so no question to put.
func TestCleanDoesNotPruneOrReapForALiveRecord(t *testing.T) {
	rtime, daemon := runtimeWatchingItsProxy(t)
	project := t.TempDir()
	pgid, _ := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
		Hostname:    "live.localhost",
	}))

	removed, err := rtime.PruneStale()
	require.NoError(t, err)
	assert.Empty(t, removed, "a record whose process group is alive is not stale")

	left, err := localstate.List()
	require.NoError(t, err)
	assert.Len(t, left, 1, "the live record must survive the sweep")
	assert.Zero(t, daemon.asks(), "nothing was removed, so nothing should have been asked")
}
