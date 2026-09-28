package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// The exit statuses of `runs trigger-wait`, one per way the wait can end.
// Success is zero like any command. A run that failed and a wait that ran out
// are both something a script has to act on, and they call for different
// actions — look at the failure, or wait longer — so they get a status each
// rather than sharing one.
const (
	exitRunFailed   = 1
	exitWaitTimeout = 2
)

// The wait's defaults, in seconds, which are the ones the command came from: an
// hour is longer than a test run of nearly any DAG, and five seconds sees a
// short run finish promptly without hammering the API.
const (
	defaultWaitTimeout  = "3600"
	defaultPollInterval = "5"
)

// The run and task states the wait and the failure filter branch on. Airflow
// owns the full sets; these are the ones this code names.
const (
	stateSuccess        = "success"
	stateFailed         = "failed"
	stateUpstreamFailed = "upstream_failed"
)

// runFinished reports whether a run's state is one it will not leave by
// itself. A DAG run has four states and two of them are terminal; queued and
// running both mean "keep waiting". A run someone marks by hand ends in one of
// the same two.
func runFinished(state string) bool {
	return state == stateSuccess || state == stateFailed
}

func newRunsTriggerWaitCmd(q *query) *cobra.Command {
	var opts triggerFlags
	var timeout, poll string
	cmd := &cobra.Command{
		Use:   "trigger-wait <DAG_ID>",
		Short: "Start a run of a DAG and wait for it to finish",
		Long: "Start a run of a DAG, as `runs trigger` does, then check on it every --poll-interval until it " +
			"succeeds, fails, or --timeout passes. A failed run is reported with the task instances that failed " +
			"or never ran because something upstream did.\n\n" +
			"--timeout and --poll-interval take a number of seconds (300) or a duration (5m, 1h30m).\n\n" +
			"Exit status: 0 when the run succeeded, " + strconv.Itoa(exitRunFailed) + " when it failed (or the " +
			"command itself did), " + strconv.Itoa(exitWaitTimeout) + " when the wait timed out with the run " +
			"still going. The run is reported in every case, and a timeout leaves it running: stopping the wait " +
			"does not stop the run.\n\n" +
			"A paused DAG is unpaused first, as `runs trigger` does, or the wait would last until the timeout " +
			"for a run that is never scheduled.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			trigger, err := opts.options()
			if err != nil {
				return err
			}
			var wait waitOptions
			if wait.timeout, err = parseWait("--timeout", timeout); err != nil {
				return err
			}
			if wait.poll, err = parseWait("--poll-interval", poll); err != nil {
				return err
			}
			return q.runRunsTriggerWait(cmd.Context(), args[0], trigger, !opts.noAutoUnpause, wait)
		},
	}
	opts.register(cmd)
	// -t is --tags across this tree (TestShorthandsMeanOneThingEachAcrossTheV2Tree),
	// so --timeout has no shorthand here.
	cmd.Flags().StringVar(&timeout, "timeout", defaultWaitTimeout, "How long to wait before giving up, in seconds or as a duration")
	cmd.Flags().StringVarP(&poll, "poll-interval", "p", defaultPollInterval, "How often to check on the run, in seconds or as a duration")
	return cmd
}

// waitOptions is how long to wait for a run, and how often to look.
type waitOptions struct {
	timeout time.Duration
	poll    time.Duration
}

// parseWait reads a wait flag. A bare number is seconds, which is how these
// flags were spelled in the CLI they came from and how the agent skills that
// call them already spell them; a Go duration reads as one, for the reader who
// would rather say 5m.
func parseWait(flag, value string) (time.Duration, error) {
	var d time.Duration
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds > math.MaxInt64/float64(time.Second) {
			return 0, fmt.Errorf("%s must be a number of seconds or a duration such as 5m, not %q", flag, value)
		}
		d = time.Duration(seconds * float64(time.Second))
	} else {
		parsed, perr := time.ParseDuration(value)
		if perr != nil {
			return 0, fmt.Errorf("%s must be a number of seconds or a duration such as 5m, not %q", flag, value)
		}
		d = parsed
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be more than zero, not %q", flag, value)
	}
	return d, nil
}

// waitedRun is what `runs trigger-wait` reports: the run as it last stood,
// how the wait ended, and — when the run failed — the task instances that
// explain it. TimedOut and Unpaused carry no omitempty: each is the answer to a
// question a consumer asks every time, and false is an answer.
type waitedRun struct {
	runRow
	Unpaused bool `json:"unpaused"`
	TimedOut bool `json:"timed_out"`
	// ElapsedSeconds is how long this command waited, from the trigger to the
	// last check — not the run's own duration, which is duration_seconds.
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	// FailedTasks is the run's failed and upstream_failed task instances,
	// set when the run ended in anything but success. See failedTasks.
	FailedTasks []taskInstanceRow `json:"failed_tasks,omitempty"`
}

func (q *query) runRunsTriggerWait(ctx context.Context, dagID string, trigger airflowapi.TriggerDAGRunOptions, autoUnpause bool, wait waitOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	run, unpaused, err := q.trigger(ctx, client, dagID, trigger, autoUnpause)
	if err != nil {
		return err
	}
	started := time.Now()
	fmt.Fprintf(q.d.Stderr, "triggered %s run %s; waiting up to %s for it to finish\n", dagID, run.DAGRunID, formatDuration(wait.timeout.Seconds()))

	run, timedOut, err := q.awaitRun(ctx, client, run, started.Add(wait.timeout), wait.poll)
	if err != nil {
		return err
	}
	result := waitedRun{
		runRow:         newRunRow(run),
		Unpaused:       unpaused,
		TimedOut:       timedOut,
		ElapsedSeconds: hundredths(time.Since(started).Seconds()),
	}
	if !timedOut && run.State != stateSuccess {
		result.FailedTasks = q.failedTasks(ctx, client, dagID, run.DAGRunID)
	}
	if err := r.Emit(result, func(w io.Writer) error { return q.renderWaitedRun(w, result) }); err != nil {
		return err
	}
	switch {
	case timedOut:
		return &ExitError{Code: exitWaitTimeout}
	case run.State != stateSuccess:
		return &ExitError{Code: exitRunFailed}
	}
	return nil
}

// hundredths rounds seconds to two places, which is as fine as a wait measured
// in polls deserves.
func hundredths(seconds float64) float64 {
	const places = 100
	return math.Round(seconds*places) / places
}

// awaitRun checks on a run until it finishes or the deadline passes, and
// returns it as last seen. The last sleep is cut short at the deadline and the
// run checked once more, so a run that finishes just inside the timeout is
// reported finished rather than timed out.
//
// A check that fails ends the wait with an error: the run was started and may
// well still be going, so the error says how to look at it.
func (q *query) awaitRun(ctx context.Context, client *airflowapi.Client, run airflowapi.DAGRun, deadline time.Time, poll time.Duration) (airflowapi.DAGRun, bool, error) {
	last := run.State
	for !runFinished(run.State) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return run, true, nil
		}
		timer := time.NewTimer(min(poll, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			return run, false, fmt.Errorf("stopped waiting for %s run %s, which is still %s; check on it with `%s`: %w",
				run.DAGID, run.DAGRunID, run.State, q.t.suggest("runs get "+run.DAGID+" "+run.DAGRunID), ctx.Err())
		case <-timer.C:
		}
		next, err := client.GetDAGRun(ctx, run.DAGID, run.DAGRunID)
		if err != nil {
			return run, false, fmt.Errorf("could not check on %s run %s, which was triggered and may still be running; "+
				"check on it with `%s`: %w", run.DAGID, run.DAGRunID, q.t.suggest("runs get "+run.DAGID+" "+run.DAGRunID), err)
		}
		// Keep the ids the trigger answered with: they address the run, and a
		// body that omitted one would otherwise point the next check nowhere.
		next.DAGID, next.DAGRunID = run.DAGID, run.DAGRunID
		run = next
		if run.State != last {
			fmt.Fprintf(q.d.Stderr, "%s run %s: %s\n", run.DAGID, run.DAGRunID, run.State)
			last = run.State
		}
	}
	return run, false, nil
}

func (q *query) renderWaitedRun(w io.Writer, result waitedRun) error {
	if result.TimedOut {
		_, err := fmt.Fprintf(w, "%s run %s is still %s after %s; stopped waiting, and the run carries on\n"+
			"check on it with `%s`\n",
			result.DAGID, result.RunID, result.State, formatDuration(result.ElapsedSeconds),
			q.t.suggest("runs get "+result.DAGID+" "+result.RunID))
		return err
	}
	if _, err := fmt.Fprintf(w, "%s run %s: %s after %s\n",
		result.DAGID, result.RunID, result.State, formatDuration(result.ElapsedSeconds)); err != nil {
		return err
	}
	if result.State == stateSuccess {
		return nil
	}
	return q.renderFailedTasks(w, result.DAGID, result.RunID, result.FailedTasks)
}

// failedTasks lists the task instances that explain a run that did not
// succeed. A listing that fails costs only this part of the report: the run's
// own state is the answer to "did it work", and it is already in hand.
func (q *query) failedTasks(ctx context.Context, client *airflowapi.Client, dagID, runID string) []taskInstanceRow {
	instances, err := allTaskInstances(ctx, client, dagID, runID)
	if err != nil {
		fmt.Fprintf(q.d.Stderr, "could not list the run's task instances: %v\n", err)
		return nil
	}
	return failedOnly(mapRows(instances, newTaskInstanceRow))
}

// failedTaskState reports whether a task instance's state explains a failed
// run: it failed, or it never ran because a task upstream of it did. The
// second is not the cause, but it is the part of the run the failure cost,
// and it is how a reader traces the failure back up the graph.
func failedTaskState(state string) bool {
	return state == stateFailed || state == stateUpstreamFailed
}

func failedOnly(rows []taskInstanceRow) []taskInstanceRow {
	failed := make([]taskInstanceRow, 0, len(rows))
	for i := range rows {
		if failedTaskState(rows[i].State) {
			failed = append(failed, rows[i])
		}
	}
	return failed
}

// taskInstancePageCap bounds how many pages allTaskInstances reads.
const taskInstancePageCap = 100

// allTaskInstances reads every task instance in a run. The endpoint pages, and
// a failure filter over the first page alone would call a run of a few hundred
// tasks clean when its failure sat on the second.
func allTaskInstances(ctx context.Context, client *airflowapi.Client, dagID, runID string) ([]airflowapi.TaskInstance, error) {
	var all []airflowapi.TaskInstance
	for range taskInstancePageCap {
		page, err := client.ListTaskInstances(ctx, dagID, runID, airflowapi.ListOptions{Offset: len(all)})
		if err != nil {
			return nil, err
		}
		all = append(all, page.TaskInstances...)
		if len(page.TaskInstances) == 0 || len(all) >= page.TotalEntries {
			return all, nil
		}
	}
	return all, errors.New("this run has more task instances than one command reads; list them with `runs tasks`")
}

// renderFailedTasks writes the failed task instances under a run, and how to
// read the log of the one that failed first.
func (q *query) renderFailedTasks(w io.Writer, dagID, runID string, failed []taskInstanceRow) error {
	if len(failed) == 0 {
		_, err := fmt.Fprintln(w, "no task instance failed")
		return err
	}
	if _, err := fmt.Fprintln(w, "\nfailed tasks:"); err != nil {
		return err
	}
	if err := renderRunTaskTable(w, failed); err != nil {
		return err
	}
	for i := range failed {
		row := &failed[i]
		// upstream_failed never ran, so it has no log to read.
		if row.State != stateFailed {
			continue
		}
		command := "tasks logs " + dagID + " " + runID + " " + row.TaskID + " --try " + strconv.Itoa(max(row.TryNumber, 1))
		if row.MapIndex >= 0 {
			command += " -m " + strconv.Itoa(row.MapIndex)
		}
		_, err := fmt.Fprintf(w, "\nread why %s failed with `%s`\n", row.TaskID, q.t.suggest(command))
		return err
	}
	return nil
}
