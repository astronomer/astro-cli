package localrt

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// allGone confirms every path asked about is gone, which is what a test that
// is not about per-project differences wants to say. The opposite verdict
// needs no helper: an absent path is not confirmed gone.
func allGone(paths []string) map[string]bool {
	answer := make(map[string]bool, len(paths))
	for _, p := range paths {
		answer[p] = true
	}
	return answer
}

// stubDockerStatuses makes the docker sweep answer from the records it is
// given instead of from the host.
//
// Not a convenience: without it these cases reach the real docker and podman
// on the developer's machine, which localdocker's Commander contract forbids
// ("tests substitute a fake and never touch a real daemon"). Measured at
// 0.7s each with both binaries on PATH, and on the machine this whole change
// is about — podman installed, its machine wedged — a unit test would block
// on `podman machine inspect` with no deadline of its own.
//
// running names the projects to report live; everything else is stopped.
func stubDockerStatuses(r *Runtime, running ...string) *int {
	calls := 0
	live := map[string]bool{}
	for _, p := range running {
		live[p] = true
	}
	r.dockerStatuses = func(_ context.Context, recs []localstate.Record) ([]Status, error) {
		calls++
		statuses := make([]Status, len(recs))
		for i := range recs {
			statuses[i] = recs[i].Status(live[recs[i].ProjectPath])
		}
		return statuses, nil
	}
	return &calls
}

// The docker records are lifted out of the list, answered in one sweep, and
// put back where they came from. That bookkeeping is the only new way this
// can go wrong, and getting it wrong would report one project's liveness
// under another project's name — so it is pinned by index, not by set.
//
// Interleaved on purpose: docker and standalone records alternate, so a
// version that appended the batch answers rather than returning them to their
// own slots fails here instead of passing on a list that happens to be sorted.
func TestListAnswersEveryRecordInItsOwnSlot(t *testing.T) {
	rtime := isolatedRuntime(t)
	// The third docker record is the live one, so a mix-up cannot be hidden
	// by every answer being the same.
	calls := stubDockerStatuses(rtime, "/p/project-4")

	var recs []localstate.Record
	for i := 0; i < 8; i++ {
		mode := ModeStandalone
		if i%2 == 0 {
			mode = ModeDocker
		}
		recs = append(recs, localstate.Record{
			ProjectPath:    fmt.Sprintf("/p/project-%d", i),
			Mode:           mode,
			ComposeProject: fmt.Sprintf("astro-project-%d", i),
		})
	}

	statuses, err := rtime.statusOfAll(recs)

	require.NoError(t, err)
	require.Len(t, statuses, len(recs))
	for i := range recs {
		assert.Equal(t, recs[i].ProjectPath, statuses[i].ProjectPath,
			"the answer at %d belongs to another record", i)
		assert.Equal(t, recs[i].Mode, statuses[i].Mode)
	}
	assert.Equal(t, StateRunning, statuses[4].State, "the live docker project kept its own slot")
	assert.Equal(t, 1, *calls, "the docker records are answered in one sweep")
}

// A list of nothing asks the engines nothing. Worth its own case because the
// sweep is the expensive call: a machine with no records at all should not
// shell out to a container engine to be told so.
func TestListOfNoRecordsIsEmpty(t *testing.T) {
	rtime := isolatedRuntime(t)
	calls := stubDockerStatuses(rtime)

	statuses, err := rtime.statusOfAll(nil)

	require.NoError(t, err)
	assert.Empty(t, statuses)
	assert.Zero(t, *calls)
}

// A list with no docker records in it does not ask a container engine either.
// The sweep is one call rather than N now, but one call against a wedged
// podman is still the hang this change exists to remove.
func TestAListOfStandaloneRecordsAsksNoEngine(t *testing.T) {
	rtime := isolatedRuntime(t)
	calls := stubDockerStatuses(rtime)

	_, err := rtime.statusOfAll([]localstate.Record{
		{ProjectPath: "/p/one", Mode: ModeStandalone},
	})

	require.NoError(t, err)
	assert.Zero(t, *calls, "no docker records, so nothing to ask an engine about")
}

// Records nothing claims are stopped — the property that makes them removable
// again, since --clean skips records that report running.
func TestRecordsNothingClaimsAreStopped(t *testing.T) {
	rtime := isolatedRuntime(t)
	stubDockerStatuses(rtime)

	statuses, err := rtime.statusOfAll([]localstate.Record{
		{ProjectPath: "/p/nowhere", Mode: ModeDocker, ComposeProject: "astro-nowhere"},
		// The shape the suite leaked: no compose project name at all.
		{ProjectPath: "/p/nameless", Mode: ModeDocker},
	})

	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.Equal(t, StateStopped, statuses[0].State)
	assert.Equal(t, StateStopped, statuses[1].State)
}

// The --clean sweep asks about every stale docker record at once.
//
// This is the half of the probe storm that survived the first pass: List was
// batched and pruneAll was not, so `astro local list --clean` on the 26
// records that prompted the change still took 18 seconds — each record paying
// its own two shellouts, in the command that runs immediately after the
// listing that had just stopped doing exactly that.
func TestCleanAsksAboutEveryStaleRecordAtOnce(t *testing.T) {
	rtime := isolatedRuntime(t)
	asked := 0
	var sawPaths []string
	rtime.containersGone = func(_ context.Context, paths []string) (map[string]bool, error) {
		asked++
		sawPaths = paths
		return allGone(paths), nil
	}

	var statuses []Status
	for i := 0; i < 26; i++ {
		statuses = append(statuses, Status{
			ProjectPath: fmt.Sprintf("/p/stale-%d", i),
			Mode:        ModeDocker,
			State:       StateStopped,
		})
	}

	removed, err := rtime.pruneAll(statuses)

	require.NoError(t, err)
	assert.Len(t, removed, 26)
	assert.Equal(t, 1, asked, "26 records, one question")
	assert.Len(t, sawPaths, 26, "and every one of them was in it")
}

// A running docker project is not a removal candidate, so the engine is not
// asked about it. Asking would cost a probe to learn what the listing already
// said.
func TestCleanDoesNotAskAboutRunningProjects(t *testing.T) {
	rtime := isolatedRuntime(t)
	var sawPaths []string
	rtime.containersGone = func(_ context.Context, paths []string) (map[string]bool, error) {
		sawPaths = paths
		return allGone(paths), nil
	}

	_, err := rtime.pruneAll([]Status{
		{ProjectPath: "/p/live", Mode: ModeDocker, State: StateRunning},
		{ProjectPath: "/p/stale", Mode: ModeDocker, State: StateStopped},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"/p/stale"}, sawPaths)
}
