package localdocker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// dockerRec is a docker-mode record, the only kind the batch accepts.
func dockerRec(path, composeProject string) localstate.Record {
	return localstate.Record{ProjectPath: path, Mode: rt.ModeDocker, ComposeProject: composeProject}
}

// The emptiness guard in claims, from the outside.
//
// A record that reached disk without a compose project name carries "", and a
// probe that finds nothing answers "" — so the bare equality this replaced saw
// two silences and called them a match, reporting a dead project as a running
// Airflow. It could not be undone from the CLI either: a --clean sweep skips
// records that report running, so such a record survived every supported way
// of removing it and had to be deleted by hand.
//
// Real records grew this way: the suite wrote them into the developer's own
// cache before it was isolated, and 26 of them turned `astro use` into an
// 18-second command that listed 26 Airflows nobody was running.
func TestADockerRecordWithNoComposeNameIsNeverRunning(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)

	st := e.StatusOf(dockerRec("/p/orphan", ""))

	assert.Equal(t, rt.StateStopped, st.State,
		"nothing is running and the record names no project: that is not a live Airflow")
}

// Matching on an empty name is refused from the other direction too: an engine
// that answers about some other directory must not satisfy a nameless record.
func TestANamelessRecordIsNotSatisfiedByAnotherProject(t *testing.T) {
	cmd := &fakeCmd{output: func(string) ([]byte, error) {
		return []byte("/p/somebody-else\tastro-somebody-else\n"), nil
	}}
	e := testEngine(t, cmd)

	statuses, err := e.StatusOfAll(context.Background(), []localstate.Record{dockerRec("/p/orphan", "")})

	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, rt.StateStopped, statuses[0].State)
}

// A directory is not a key. Somebody's own `docker compose up` in the project
// directory, or an older astro stack still up under a previous name, puts two
// compose projects under one working_dir — and keeping a single winner per
// directory meant the record's own project could lose its slot and its live
// Airflow be reported stopped.
func TestALiveProjectIsFoundBesideAForeignOneInTheSameDirectory(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasPrefix(call, "docker ps") {
			// Ours first, so a last-write-wins map would drop it.
			return []byte("/p/shared\tastro-shared\n/p/shared\tsomebody-elses-stack\n"), nil
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)

	statuses, err := e.StatusOfAll(context.Background(), []localstate.Record{dockerRec("/p/shared", "astro-shared")})

	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, rt.StateRunning, statuses[0].State,
		"our project is running in that directory, whoever else is too")
}

// The call count is the whole point: one sweep for the list, not one probe per
// record. Asked per record, a machine with 26 docker records spent 18 seconds
// in `astro use`, because every record that was not running also paid for a
// fall-through probe of the other engine — 52 subprocesses to print a table.
func TestStatusOfAllAsksTheEnginesOnceForTheWholeList(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasPrefix(call, "docker ps") {
			// One compose project answers once per container, so the same
			// pair arrives more than once; the reader must not mind.
			return []byte("/p/live\tastro-live\n/p/live\tastro-live\n"), nil
		}
		return nil, errors.New("podman machine not running")
	}}
	e := testEngine(t, cmd)

	recs := []localstate.Record{dockerRec("/p/live", "astro-live")}
	for i := 0; i < 25; i++ {
		recs = append(recs, dockerRec(fmt.Sprintf("/p/stale-%d", i), fmt.Sprintf("astro-stale-%d", i)))
	}

	statuses, err := e.StatusOfAll(context.Background(), recs)

	require.NoError(t, err)
	require.Len(t, statuses, len(recs), "every record gets an answer, in its own slot")
	assert.Equal(t, rt.StateRunning, statuses[0].State, "the one live project is live")
	for i := 1; i < len(statuses); i++ {
		assert.Equal(t, rt.StateStopped, statuses[i].State, statuses[i].ProjectPath)
	}
	// Exactly two: the preferred engine, then the other one because 25
	// records were still unaccounted for. A ceiling would also accept an
	// implementation that dropped the fall-through entirely.
	assert.Len(t, cmd.calls, 2,
		"the engines are asked for the whole list at once, not once per record")
}

// When the first engine accounts for every record there is nothing the second
// could add, so it is not asked — and on a machine with podman installed but
// its machine down, not asking is worth most of a second.
func TestStatusOfAllStopsAtOneEngineWhenItAccountsForEveryRecord(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasPrefix(call, "docker ps") {
			return []byte("/p/one\tastro-one\n/p/two\tastro-two\n"), nil
		}
		return nil, errors.New("the other engine must not be probed")
	}}
	e := testEngine(t, cmd)

	statuses, err := e.StatusOfAll(context.Background(), []localstate.Record{
		dockerRec("/p/one", "astro-one"),
		dockerRec("/p/two", "astro-two"),
	})

	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.Equal(t, rt.StateRunning, statuses[0].State)
	assert.Equal(t, rt.StateRunning, statuses[1].State)
	assert.Len(t, cmd.calls, 1, "nothing was unaccounted for, so the second engine is left alone")
}

// A record whose containers run under the other engine is still found: the
// fall-through survives the batching.
func TestStatusOfAllFallsThroughToTheOtherEngine(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasPrefix(call, "podman ps") {
			return []byte("/p/pod\tastro-pod\n"), nil
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)

	statuses, err := e.StatusOfAll(context.Background(), []localstate.Record{dockerRec("/p/pod", "astro-pod")})

	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, rt.StateRunning, statuses[0].State)
}

// The batch is docker's, and says so rather than judging a standalone record
// by container liveness — which would report every one of them stopped and
// explain nothing. ReadStatus already refuses the same way.
func TestStatusOfAllRefusesAForeignRecord(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})

	_, err := e.StatusOfAll(context.Background(), []localstate.Record{
		{ProjectPath: "/p/standalone", Mode: rt.ModeStandalone},
	})

	assert.ErrorIs(t, err, ErrNotDockerMode)
}

// An engine is never asked a question that cannot time out.
//
// The probe used to inherit context.Background() from StatusOf and from the
// --clean sweep, so nothing bounded it: a podman whose machine is down takes
// the better part of a second to say so, and a docker socket that accepts a
// connection and then goes quiet never says anything at all. That is what a
// hang in `astro use` was.
func TestEveryEngineProbeIsBounded(t *testing.T) {
	t.Run("the per-project probe", func(t *testing.T) {
		cmd := &fakeCmd{output: noProjects}
		e := testEngine(t, cmd)

		_, _ = e.findProject(context.Background(), "/p/one")

		requireAllBounded(t, cmd)
	})

	t.Run("the whole-machine sweep", func(t *testing.T) {
		cmd := &fakeCmd{output: noProjects}
		e := testEngine(t, cmd)

		_, _ = e.StatusOfAll(context.Background(), []localstate.Record{dockerRec("/p/one", "astro-one")})

		requireAllBounded(t, cmd)
	})
}

// And the deadline is one the plumbing can act on: an engine that accepts the
// call and never answers yields a verdict, in bounded time, instead of hanging
// the command. Asserting a deadline was merely SET passes for an
// implementation that sets one nothing enforces.
func TestAnEngineThatNeverAnswersDoesNotHangTheProbe(t *testing.T) {
	shortProbeTimeout(t, 50*time.Millisecond)
	cmd := &fakeCmd{
		output:   noProjects,
		delayFor: func(string) time.Duration { return time.Hour },
	}
	e := testEngine(t, cmd)

	done := make(chan rt.Status, 1)
	go func() { done <- e.StatusOf(dockerRec("/p/wedged", "astro-wedged")) }()

	select {
	case st := <-done:
		assert.Equal(t, rt.StateStopped, st.State)
	case <-time.After(30 * time.Second):
		t.Fatal("a probe against an engine that never answers did not return")
	}
}

// The same deadline covers resolving the engine CONNECTION, which is a
// separate subprocess and the one the first version of this change missed:
// connFor reaches `podman machine inspect` through an exec.Command that takes
// no context, so on a machine whose podman is wedged the CLI hung before it
// ever reached a probe to time out.
func TestAWedgedEngineConnectionDoesNotHangTheProbe(t *testing.T) {
	shortProbeTimeout(t, 50*time.Millisecond)
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.connFor = func(bin string) engineConn {
		time.Sleep(time.Hour)
		return engineConn{bin: bin}
	}

	done := make(chan rt.Status, 1)
	go func() { done <- e.StatusOf(dockerRec("/p/wedged", "astro-wedged")) }()

	select {
	case st := <-done:
		assert.Equal(t, rt.StateStopped, st.State)
	case <-time.After(30 * time.Second):
		t.Fatal("resolving a wedged engine connection was not bounded")
	}
}

// A probe that could not reach an engine says so, separately from its verdict.
// The start path refuses on it: a missed deadline reads exactly like "no
// containers", and docker mode has no other double-start guard.
func TestStatusOfReachedSeparatesNoAnswerFromNoContainers(t *testing.T) {
	t.Run("an engine that answers is reached", func(t *testing.T) {
		e := testEngine(t, &fakeCmd{output: noProjects})

		st, reached := e.StatusOfReached(dockerRec("/p/one", "astro-one"))

		assert.Equal(t, rt.StateStopped, st.State)
		assert.True(t, reached)
	})

	t.Run("no engine answering is not reached", func(t *testing.T) {
		cmd := &fakeCmd{output: func(string) ([]byte, error) {
			return nil, errors.New("cannot connect to the docker daemon")
		}}
		e := testEngine(t, cmd)

		st, reached := e.StatusOfReached(dockerRec("/p/one", "astro-one"))

		assert.Equal(t, rt.StateStopped, st.State)
		assert.False(t, reached, "nobody answered, so stopped is not evidence")
	})
}

// ContainersGoneAll answers for a whole sweep, and refuses to confirm anything
// when no engine could be reached — because --clean deletes records and routes
// on the answer, and a daemon blip must not read as "everything is stopped".
func TestContainersGoneAll(t *testing.T) {
	t.Run("answers every path from one sweep", func(t *testing.T) {
		cmd := &fakeCmd{output: func(call string) ([]byte, error) {
			if strings.HasPrefix(call, "docker ps") {
				return []byte("/p/live\tastro-live\n"), nil
			}
			return nil, nil
		}}
		e := testEngine(t, cmd)

		gone, err := e.ContainersGoneAll(context.Background(), []string{"/p/live", "/p/stale"})

		require.NoError(t, err)
		assert.False(t, gone["/p/live"])
		assert.True(t, gone["/p/stale"])
		assert.Len(t, cmd.calls, 2, "one sweep per engine, not one per path")
	})

	t.Run("no engine reachable is an error, not a confirmation", func(t *testing.T) {
		cmd := &fakeCmd{output: func(string) ([]byte, error) {
			return nil, errors.New("cannot connect to the docker daemon")
		}}
		e := testEngine(t, cmd)

		gone, err := e.ContainersGoneAll(context.Background(), []string{"/p/one"})

		require.Error(t, err)
		assert.Nil(t, gone)
	})

	t.Run("nothing to ask about asks nothing", func(t *testing.T) {
		cmd := &fakeCmd{output: noProjects}
		e := testEngine(t, cmd)

		gone, err := e.ContainersGoneAll(context.Background(), nil)

		require.NoError(t, err)
		assert.Empty(t, gone)
		assert.Empty(t, cmd.calls)
	})
}

// shortProbeTimeout lowers the probe deadline for one test and restores it.
func shortProbeTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	was := probeTimeout
	probeTimeout = d
	t.Cleanup(func() { probeTimeout = was })
}

func requireAllBounded(t *testing.T, cmd *fakeCmd) {
	t.Helper()
	require.NotEmpty(t, cmd.outputBounded, "the engine should have been asked something")
	for i, bounded := range cmd.outputBounded {
		assert.True(t, bounded, "probe %d (%s) inherited an unbounded context", i, cmd.calls[i])
	}
}
