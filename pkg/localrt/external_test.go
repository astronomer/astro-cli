//go:build !windows

// Same constraint as runtime_test.go: proving a claim is visible means proving
// its liveness probe passes, and that probe is kill(-pgid, 0). See that file's
// header for why the build tag has to be here rather than skipped at runtime.

package localrt

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// claimedRuntime is a Runtime with a fixed clock, so a claim's StartedAt is an
// asserted value rather than whatever the wall clock said.
func claimedRuntime(t *testing.T, at time.Time) *Runtime {
	t.Helper()
	r := realRuntime(t)
	r.now = func() time.Time { return at }
	return r
}

// claim runs the whole documented sequence — reserve, claim, close — and returns
// the first error from either step.
//
// It does NOT assert that Reserve succeeded, deliberately. Reserve is where the
// live-runtime and foreign-mode refusals happen, so a helper that required it to
// succeed would hide exactly the errors most of these tests are about.
func claim(t *testing.T, r *Runtime, ext ExternalRuntime) error {
	t.Helper()
	res, err := r.Reserve(ext.ProjectPath)
	if err != nil {
		return err
	}
	defer res.Close()
	return res.Claim(ext)
}

// liveGroupWithMember starts a group leader and a second process joined to that
// same group, so PID and Pgid are two different live numbers.
//
// Every claim used to pass PID and Pgid the same value, which meant nothing
// pinned which field was which: writing the record's PID from ext.Pgid, or
// comparing Release's ownership against Pgid instead of PID, both stayed green.
// The record's PID is what Record.Status publishes as the runtime's pid and what
// Release's whole ownership guarantee rests on, so the two have to be told apart.
func liveGroupWithMember(t *testing.T) (pgid, memberPID int) {
	t.Helper()
	pgid, _ = liveGroup(t)
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	require.NotEqual(t, pgid, cmd.Process.Pid, "the member must not be the leader")
	return pgid, cmd.Process.Pid
}

// liveNonLeader starts a real process that is NOT a group leader: no Setpgid, so
// it inherits the test binary's group and its own pid names no group.
//
// This is the case ExternalRuntime.Pgid calls the "common case" default —
// omitting Pgid and letting it fall back to PID — going wrong. kill(pid, 0)
// succeeds, so the process plainly exists, while kill(-pid, 0) fails, so every
// liveness probe in the toolchain reads the record as stopped.
func liveNonLeader(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	require.NoError(t, syscall.Kill(cmd.Process.Pid, 0), "the process must exist")
	require.Error(t, syscall.Kill(-cmd.Process.Pid, 0), "and must not lead its own group")
	return cmd.Process.Pid
}

// The whole point of the type: after a claim, the record describes the runtime
// and every reader that goes through the record can see it.
func TestClaimPublishesARuntimeTheRestOfTheToolchainCanSee(t *testing.T) {
	claimedAt := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	r := claimedRuntime(t, claimedAt)
	project := t.TempDir()
	pgid, member := liveGroupWithMember(t)

	require.NoError(t, claim(t, r, ExternalRuntime{
		ProjectPath:  project,
		PID:          member,
		Pgid:         pgid,
		Port:         8081,
		Hostname:     "analytics.localhost",
		AirflowMajor: "3",
	}))

	recorded, err := RecordedStatus(project)
	require.NoError(t, err)
	assert.Equal(t, ModeStandalone, recorded.Mode, "an unset mode must default to standalone")
	assert.Equal(t, 8081, recorded.Port)
	assert.Equal(t, "analytics.localhost", recorded.Hostname)
	assert.Equal(t, "3", recorded.AirflowMajor)
	assert.Equal(t, claimedAt, recorded.StartedAt.UTC())
	assert.False(t, recorded.StopWithSession, "a claim's lifetime is the consumer's, not a command's")

	// PID and Pgid are distinct here, so this pins which field carries which.
	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, member, rec.PID, "the record's pid is the supervised process")
	assert.Equal(t, pgid, rec.Pgid, "the record's group is the group it belongs to")

	// The probing reader — what `astro local status` answers with — calls it
	// running, which is the fact the whole wave turns on.
	live, err := r.ReadStatus(project)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, live.State)

	// And it shows up in the machine-wide listing, which is `astro local list`.
	all, err := r.List()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 8081, all[0].Port)
}

// The hazard this wave exists to close, asserted through the gate that closes
// it: refuseLiveStart is the first thing Runtime.Start calls, so a claimed
// project makes `astro local start` refuse instead of standing up a second
// Airflow against the same directory.
func TestAClaimedProjectMakesAStartRefuseRatherThanDouble(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	pgid, reap := liveGroup(t)

	// Before the claim, nothing stops a start. This is the bug.
	require.NoError(t, r.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeStandalone}))

	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: pgid, Pgid: pgid, Port: 8081}))

	// After it, a start in either mode is refused, and the cross-mode message
	// names what is actually live.
	require.ErrorContains(t, r.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeStandalone}), "already running")
	require.ErrorContains(t, r.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeDocker}), "standalone mode")

	// Once the supervised runtime dies the stale claim stops blocking, so a
	// crashed consumer cannot wedge the CLI out of the project.
	reap()
	require.NoError(t, r.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeStandalone}))
}

// The refusal has to happen at RESERVE, before the consumer provisions anything.
//
// An earlier version of this API checked only at Claim, which runs after
// provisioning: a project the CLI was already running would let a consumer sync
// a venv and spawn its own Airflow against the same project directory, and find
// out it was the second one at the very end. Two live Airflows, the newer one
// unrecorded and so unstoppable by `astro local stop` — the exact failure this
// API exists to prevent, reintroduced one layer along.
//
// So these assert on Reserve itself, not on the reserve-then-claim sequence.
func TestReserveRefusesBeforeTheConsumerProvisionsAnything(t *testing.T) {
	r := claimedRuntime(t, time.Now())

	t.Run("a live runtime", func(t *testing.T) {
		project := t.TempDir()
		pgid, _ := liveGroup(t)
		require.NoError(t, localstate.Save(localstate.Record{
			ProjectPath: project, Mode: ModeStandalone, PID: pgid, Pgid: pgid, Port: 8080,
		}))
		_, err := r.Reserve(project)
		require.ErrorIs(t, err, ErrAlreadyRunning)
	})

	t.Run("a docker record", func(t *testing.T) {
		project := t.TempDir()
		require.NoError(t, localstate.Save(localstate.Record{
			ProjectPath: project, Mode: ModeDocker, ComposeProject: "astro-x", Port: 8080,
		}))
		_, err := r.Reserve(project)
		require.ErrorIs(t, err, ErrForeignMode)
	})

	t.Run("and a refused reserve leaves the lock free", func(t *testing.T) {
		project := t.TempDir()
		pgid, reap := liveGroup(t)
		require.NoError(t, localstate.Save(localstate.Record{
			ProjectPath: project, Mode: ModeStandalone, PID: pgid, Pgid: pgid, Port: 8080,
		}))
		_, err := r.Reserve(project)
		require.ErrorIs(t, err, ErrAlreadyRunning)
		// The runtime dies; the next reserve must succeed, which it cannot if
		// the refused one leaked its lock.
		reap()
		res, err := r.Reserve(project)
		require.NoError(t, err)
		res.Close()
	})
}

// A claim is the "nothing is running here" assertion, so it refuses rather than
// clobbers. The loser of that race would otherwise overwrite the winner's pid
// and orphan a running Airflow.
func TestClaimRefusesToOverwriteALiveRuntime(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	pgid, _ := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project, Mode: ModeStandalone, PID: pgid, Pgid: pgid, Port: 8080,
	}))

	err := claim(t, r, ExternalRuntime{ProjectPath: project, PID: pgid, Pgid: pgid, Port: 9999})
	require.ErrorIs(t, err, ErrAlreadyRunning)

	// A refused claim must not have half-written its port over the live one.
	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, 8080, rec.Port)
}

// A docker record is refused whether or not its containers probe as live, which
// is stricter than Start's own guard and deliberately so.
//
// With Docker Desktop stopped, a docker record probes as stopped. Overwriting it
// with a standalone record would drop ComposeProject, and nothing could then find
// or tear that compose stack down — while, if the probe failure was transient,
// the containers are still bound to the port and a second Airflow comes up
// alongside them.
//
// refuseClaim's other reason for checking mode first — never shelling out to
// `docker compose ls` under the project lock — is argued in its doc comment but
// NOT pinned here: the two orderings differ only when a docker runtime is
// genuinely live, which needs a real daemon. Reordering would keep this green.
func TestClaimRefusesADockerRecordEvenWhenItProbesDead(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath:    project,
		Mode:           ModeDocker,
		ComposeProject: "astro-analytics",
		Port:           8080,
	}))
	pgid, _ := liveGroup(t)

	err := claim(t, r, ExternalRuntime{ProjectPath: project, PID: pgid, Pgid: pgid, Port: 9999})
	require.ErrorIs(t, err, ErrForeignMode)
	require.ErrorContains(t, err, "docker")

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, ModeDocker, rec.Mode)
	assert.Equal(t, "astro-analytics", rec.ComposeProject, "the compose project name is the only handle on that stack")
	assert.Equal(t, 8080, rec.Port)
}

// Reserve clears a stale record instead of leaving it for Claim, and the reason
// is a bug that only shows up through the prune predicate.
//
// RouteAlive resolves a standalone route to its record and asks THAT for
// liveness rather than the route's own pid. So while a dead record sits on disk,
// a consumer's start-time route reservation — carrying the consumer's own live
// pid, holding the hostname and port across provisioning — is reported dead and
// evicted by the next prune, handing the port to whatever starts next.
//
// Asserted through the real predicate and a real store, because that is the only
// place the two halves meet.
func TestReserveClearsAStaleRecordSoAReservationRouteSurvives(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()

	// A project that was running and is not any more: the record outlives it.
	deadPgid, reap := liveGroup(t)
	res, err := r.Reserve(project)
	require.NoError(t, err)
	require.NoError(t, res.Claim(ExternalRuntime{
		ProjectPath: project, PID: deadPgid, Pgid: deadPgid, Port: 8080,
	}))
	res.Close()
	reap()

	// The next start reserves the project again.
	res, err = r.Reserve(project)
	require.NoError(t, err, "a stale record must not block a fresh start")
	defer res.Close()

	// The stale record is gone, so nothing can answer "dead" on the
	// reservation's behalf.
	_, err = RecordedStatus(project)
	require.True(t, IsNotRunning(err), "Reserve must clear the stale record, got %v", err)

	// And the reservation route survives a prune, which is the behaviour that
	// was broken: the store is built exactly as a consumer builds it.
	routesDir := t.TempDir()
	store := proxy.NewStore(routesDir, proxy.WithRouteLiveness(RouteAlive))
	require.NoError(t, store.AddRoute(&proxy.Route{
		Hostname:   "analytics.localhost",
		Port:       "10123",
		ProjectDir: project,
		PID:        os.Getpid(), // the consumer itself, indisputably alive
		Mode:       proxy.RouteModeStandalone,
	}))
	kept, err := store.ListRoutes()
	require.NoError(t, err)
	require.Len(t, kept, 1, "the reservation route was pruned while its owner was alive")
	assert.Equal(t, "10123", kept[0].Port)
}

// A same-mode record whose runtime is gone is stale state from a crash, not a
// reason to refuse: a consumer that could never reclaim its own project would be
// stuck until something swept it.
func TestClaimTakesOverAStaleSameModeRecord(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	deadPgid, reap := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project, Mode: ModeStandalone, PID: deadPgid, Pgid: deadPgid, Port: 8080,
	}))
	reap()

	livePgid, _ := liveGroup(t)
	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: livePgid, Pgid: livePgid, Port: 9999}))

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, 9999, rec.Port)
	assert.Equal(t, livePgid, rec.Pgid)
}

// A record with no mode is standalone everywhere else in the package —
// localprune says so by name, and statusOf routes it to the standalone engine —
// so the claim guard must agree.
//
// Treating it as foreign made such a project permanently unclaimable, and said
// so with "a runtime record from a different mode (standalone)" against a
// standalone claim: a message naming the claim's own mode as the alien one.
func TestClaimTreatsARecordWithNoModeAsStandalone(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	deadPgid, reap := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project, PID: deadPgid, Pgid: deadPgid, Port: 8080,
	}))
	reap()

	livePgid, _ := liveGroup(t)
	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: livePgid, Pgid: livePgid, Port: 9999}))

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, ModeStandalone, rec.Mode)
	assert.Equal(t, 9999, rec.Port)
}

// The silent failure ExternalRuntime.Pgid spends its whole doc comment on. Each
// of these would have published a record that reads as stopped from the moment
// it landed, dropping the project out of `astro local list` and letting a start
// stop refusing — so each is refused instead.
func TestClaimRefusesAGroupThatIsNotAlive(t *testing.T) {
	r := claimedRuntime(t, time.Now())

	t.Run("a group that has exited", func(t *testing.T) {
		pgid, reap := liveGroup(t)
		reap()
		err := claim(t, r, ExternalRuntime{ProjectPath: t.TempDir(), PID: pgid, Pgid: pgid, Port: 8081})
		require.ErrorIs(t, err, ErrGroupNotAlive)
	})

	t.Run("an already-negated pgid", func(t *testing.T) {
		pgid, _ := liveGroup(t)
		// A consumer that negates for kill(-pgid, 0) itself. GroupID declines to
		// salvage it by falling back to PID, because that would name a different
		// group than the writer meant.
		err := claim(t, r, ExternalRuntime{ProjectPath: t.TempDir(), PID: pgid, Pgid: -pgid, Port: 8081})
		require.ErrorIs(t, err, ErrGroupNotAlive)
	})

	t.Run("a live process that does not lead its own group", func(t *testing.T) {
		pid := liveNonLeader(t)
		// Pgid omitted, which the doc calls the common case: it falls back to
		// PID, and PID names no group here.
		err := claim(t, r, ExternalRuntime{ProjectPath: t.TempDir(), PID: pid, Port: 8081})
		require.ErrorIs(t, err, ErrGroupNotAlive)
	})
}

// Both of these fail silently if they reach disk: a zero pid reads as a dead
// runtime, and a zero port sends anything that trusts the record to a port
// nothing is listening on.
func TestClaimRefusesAnIncompleteClaim(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()

	require.ErrorContains(t, claim(t, r, ExternalRuntime{ProjectPath: project, Port: 8080}), "pid")
	require.ErrorContains(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: 4321}), "port")
	require.ErrorContains(t, claim(t, r, ExternalRuntime{ProjectPath: project, Mode: "kubernetes", PID: 1, Port: 2}), "unknown runtime mode")
	require.ErrorContains(t, claim(t, r, ExternalRuntime{ProjectPath: project, Mode: ModeDocker, PID: 1, Port: 2}), "own engine")

	// None of the refusals wrote anything.
	_, err := localstate.Load(project)
	require.ErrorIs(t, err, localstate.ErrNotRunning)
}

// Zero Pgid means "PID leads its own group". The record is written with the
// resolved value rather than the zero, so no reader has to repeat the inference.
func TestClaimResolvesThePgidOnDiskRatherThanLeavingItZero(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	pgid, _ := liveGroup(t)

	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: pgid, Port: 8081}))

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, pgid, rec.Pgid, "an omitted pgid must be written out as the pid, not left at zero")
}

// The consumer is the only party that knows when its Airflow really started, so
// a re-claim of a long-running runtime must be able to say so rather than
// reporting an age of zero.
func TestClaimHonoursASuppliedStartTime(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	r := claimedRuntime(t, now)
	project := t.TempDir()
	pgid, _ := liveGroup(t)
	sixHoursAgo := now.Add(-6 * time.Hour)

	require.NoError(t, claim(t, r, ExternalRuntime{
		ProjectPath: project, PID: pgid, Pgid: pgid, Port: 8081, StartedAt: sixHoursAgo,
	}))

	recorded, err := RecordedStatus(project)
	require.NoError(t, err)
	assert.Equal(t, sixHoursAgo, recorded.StartedAt.UTC())
}

// A claim stores the path the engines would store — absolute, symlinks intact —
// because Runtime.List joins records to routes.json by plain string comparison.
// Storing the resolved spelling would miss that join for any project under a
// symlink, so List would report an empty hostname and PruneStale would then skip
// removing the route and leak it.
func TestClaimStoresThePathSpellingTheEnginesWouldStore(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	require.NoError(t, os.Symlink(target, link))
	pgid, _ := liveGroup(t)

	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: link, PID: pgid, Pgid: pgid, Port: 8081}))

	// Keyed by the resolved path, so it is findable either way...
	rec, err := localstate.Load(target)
	require.NoError(t, err)
	// ...but the stored spelling is the one that was passed in, not the target.
	assert.Equal(t, link, rec.ProjectPath)
	assert.NotEqual(t, target, rec.ProjectPath, "resolving here would break List's routes.json join")
}

// The reservation is what covers the provisioning window, so a second one has to
// be refused — and refused with an error a consumer can classify, since the right
// response (wait and retry) differs from every other refusal here.
func TestReserveKeepsASecondStartOut(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()

	first, err := r.Reserve(project)
	require.NoError(t, err)

	_, err = r.Reserve(project)
	require.ErrorIs(t, err, ErrStartInProgress)

	// Closing frees it, and closing twice is harmless so `defer` composes with
	// an explicit close on the success path.
	first.Close()
	first.Close()
	second, err := r.Reserve(project)
	require.NoError(t, err)
	second.Close()
}

// Claim lives on the reservation so the check-and-write cannot happen without
// the lock. These are the two ways that could be subverted.
func TestClaimRequiresItsOwnOpenReservation(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	pgid, _ := liveGroup(t)

	res, err := r.Reserve(project)
	require.NoError(t, err)
	res.Close()
	err = res.Claim(ExternalRuntime{ProjectPath: project, PID: pgid, Pgid: pgid, Port: 8081})
	require.ErrorContains(t, err, "closed")

	other, err := r.Reserve(t.TempDir())
	require.NoError(t, err)
	defer other.Close()
	err = other.Claim(ExternalRuntime{ProjectPath: project, PID: pgid, Pgid: pgid, Port: 8081})
	require.ErrorContains(t, err, "this reservation is for")
}

// Two spellings that hash to the same state dir are the same project, so a
// reservation taken under one must cover a claim under the other. Comparing raw
// strings failed a claim the lock provably covered, after provisioning had
// already finished.
func TestAReservationCoversAnEquivalentSpellingOfItsProject(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	require.NoError(t, os.Symlink(target, link))
	pgid, _ := liveGroup(t)

	res, err := r.Reserve(target)
	require.NoError(t, err)
	defer res.Close()

	require.NoError(t, res.Claim(ExternalRuntime{ProjectPath: link, PID: pgid, Pgid: pgid, Port: 8081}))
}

// The reservation's intended holder is a long-lived supervisor whose timers and
// provisioning goroutine can reach it at once. Run under -race this catches an
// unsynchronized unlock field; without -race it still catches a double release.
func TestAReservationSurvivesConcurrentCloses(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	res, err := r.Reserve(t.TempDir())
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res.Close()
		}()
	}
	wg.Wait()
}

// Release is a teardown path, so removing a record that is already gone is not
// an error — a consumer's stop must stay idempotent across retries and crashes.
func TestReleaseRemovesOurRecordAndIsIdempotent(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	pgid, member := liveGroupWithMember(t)

	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: member, Pgid: pgid, Port: 8081}))

	// Released by the claim's PID. Passing the GROUP must not work, which is
	// what pins that the ownership check reads the field it says it does.
	require.ErrorIs(t, r.Release(project, pgid), ErrNotOurs)

	require.NoError(t, r.Release(project, member))
	_, err := RecordedStatus(project)
	require.ErrorIs(t, err, localstate.ErrNotRunning)
	assert.True(t, IsNotRunning(err))

	require.NoError(t, r.Release(project, member), "releasing twice must not fail")
}

// A teardown for a project with no runtime state must not create any. Taking the
// project lock does MkdirAll plus O_CREATE on start.lock, so releasing an
// unknown project used to materialize a state directory that localstate.List
// then walks on every `astro local list`, forever.
func TestReleaseLeavesNoStateBehindForAProjectItNeverClaimed(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()

	require.NoError(t, r.Release(project, 4321))

	dir, err := StateDir(project)
	require.NoError(t, err)
	_, statErr := os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr), "release must not create %s", dir)
}

// A non-positive pid is not a wildcard. Docker records omit PID entirely, so
// Release(project, 0) matched every one of them and deleted it — losing the
// compose project name that is the only handle on that stack, which is the exact
// outcome ErrForeignMode prevents on the way in.
func TestReleaseRefusesToActWithoutAPid(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project, Mode: ModeDocker, ComposeProject: "astro-analytics", Port: 8080,
	}))

	require.ErrorContains(t, r.Release(project, 0), "pid")
	require.ErrorContains(t, r.Release(project, -1), "pid")

	// Even with a real pid, a docker record is not ours to remove.
	require.ErrorIs(t, r.Release(project, 4321), ErrNotOurs)

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, "astro-analytics", rec.ComposeProject)
}

// The docker record whose pid MATCHES, which is the only case that actually
// pins the mode check.
//
// A session-tied docker record carries the starter's pid, so a consumer passing
// its own pid can collide with it exactly. Mutation testing found the earlier
// version of this assertion was satisfied by the pid check instead — the record
// it used had no pid at all, so removing the mode check entirely kept the test
// green while `astro local stop` lost the only handle on that compose stack.
func TestReleaseWillNotRemoveADockerRecordThatSharesItsPid(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	const starter = 4321
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath:     project,
		Mode:            ModeDocker,
		ComposeProject:  "astro-analytics",
		PID:             starter,
		StopWithSession: true,
		Port:            8080,
	}))

	err := r.Release(project, starter)
	require.ErrorIs(t, err, ErrNotOurs)
	require.ErrorContains(t, err, "docker")

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, ModeDocker, rec.Mode)
	assert.Equal(t, "astro-analytics", rec.ComposeProject, "the compose stack's only handle must survive")
}

// The record a consumer's teardown must NOT delete: its own Airflow died, the
// user started one through the CLI, and the consumer's idle timer fires. Deleting
// there would orphan a live Airflow — invisible to `astro local list`, and
// unstoppable by `astro local stop`, which would have no record to attach to.
func TestReleaseLeavesAnotherRuntimesRecordAlone(t *testing.T) {
	r := claimedRuntime(t, time.Now())
	project := t.TempDir()
	ours, ourReap := liveGroup(t)
	require.NoError(t, claim(t, r, ExternalRuntime{ProjectPath: project, PID: ours, Pgid: ours, Port: 8081}))
	ourReap()

	// Something else takes the project over.
	theirs, _ := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project, Mode: ModeStandalone, PID: theirs, Pgid: theirs, Port: 8080,
	}))

	err := r.Release(project, ours)
	require.ErrorIs(t, err, ErrNotOurs)

	rec, err := localstate.Load(project)
	require.NoError(t, err)
	assert.Equal(t, theirs, rec.PID, "the other runtime's record must survive")
	assert.Equal(t, 8080, rec.Port)
}
