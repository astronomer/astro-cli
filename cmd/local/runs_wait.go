package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
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
	var timing waitFlags
	cmd := &cobra.Command{
		Use:   "trigger-wait <DAG_ID>",
		Short: "Start a run of a Dag and wait for it to finish",
		Long: "Start a run of a Dag, as " + q.t.suggest("runs trigger") + " does, then check on it every --poll-interval until it " +
			"succeeds, fails, or --timeout passes. A failed run is reported with the task instances that failed " +
			"or never ran because something upstream did.\n\n" +
			"--timeout and --poll-interval take a number of seconds (300) or a duration (5m, 1h30m).\n\n" +
			"Exit status: 0 when the run succeeded, " + strconv.Itoa(exitRunFailed) + " when it failed (or the " +
			"command itself did), " + strconv.Itoa(exitWaitTimeout) + " when the wait timed out with the run " +
			"still going. The run is reported in every case, and a timeout leaves it running: stopping the wait " +
			"does not stop the run.\n\n" +
			"A paused Dag is unpaused first, as " + q.t.suggest("runs trigger") + " does, or the wait would last until the timeout " +
			"for a run that is never scheduled.",
		Args: cobra.ExactArgs(1),
		Example: afExample(q.t,
			"Start a run and wait for it to finish", "runs trigger-wait <DAG_ID>",
			"Give up waiting after ten minutes", "runs trigger-wait <DAG_ID> --timeout 10m",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			trigger, err := opts.options()
			if err != nil {
				return err
			}
			wait, err := timing.options()
			if err != nil {
				return err
			}
			return q.runRunsTriggerWait(cmd.Context(), args[0], trigger, !opts.noAutoUnpause, wait)
		},
	}
	opts.register(cmd)
	timing.register(cmd)
	return cmd
}

// waitOptions is how long to wait for a run, and how often to look.
type waitOptions struct {
	timeout time.Duration
	poll    time.Duration
}

// waitFlags is --timeout and --poll-interval, which every wait takes.
type waitFlags struct {
	timeout, poll string
}

func (f *waitFlags) register(cmd *cobra.Command) {
	// -t is --tags across this tree (TestShorthandsMeanOneThingEachAcrossTheCoreTree),
	// so --timeout has no shorthand here.
	cmd.Flags().StringVar(&f.timeout, "timeout", defaultWaitTimeout, "How long to wait before giving up, in seconds or as a duration")
	cmd.Flags().StringVarP(&f.poll, "poll-interval", "p", defaultPollInterval, "How often to check on the run, in seconds or as a duration")
}

func (f *waitFlags) options() (waitOptions, error) {
	var wait waitOptions
	var err error
	if wait.timeout, err = parseWait("--timeout", f.timeout); err != nil {
		return waitOptions{}, err
	}
	if wait.poll, err = parseWait("--poll-interval", f.poll); err != nil {
		return waitOptions{}, err
	}
	return wait, nil
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
	return q.reportWaitedRun(ctx, r, client, run, unpaused, timedOut, started)
}

// reportWaitedRun emits a finished wait on a run and ends with the exit status
// that says how it ended.
func (q *query) reportWaitedRun(ctx context.Context, r cliout.Renderer, client *airflowapi.Client, run airflowapi.DAGRun, unpaused, timedOut bool, started time.Time) error {
	result := waitedRun{
		runRow:         newRunRow(run),
		Unpaused:       unpaused,
		TimedOut:       timedOut,
		ElapsedSeconds: hundredths(time.Since(started).Seconds()),
	}
	if !timedOut && run.State != stateSuccess {
		result.FailedTasks = q.failedTasks(ctx, client, run.DAGID, run.DAGRunID)
	}
	if err := r.Emit(result, func(w io.Writer) error { return q.renderWaitedRun(w, result) }); err != nil {
		return err
	}
	switch {
	case timedOut:
		return &cliout.ExitError{Code: exitWaitTimeout}
	case run.State != stateSuccess:
		return &cliout.ExitError{Code: exitRunFailed}
	}
	return nil
}

// afterElapsed is " after <how long>" for a wait that took any measurable
// time, and nothing for a run that had already finished when the wait began,
// which a "0s" would misreport as a wait.
func afterElapsed(seconds float64) string {
	if elapsed := formatDuration(seconds); elapsed != "" {
		return " after " + elapsed
	}
	return ""
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
			return run, false, fmt.Errorf("stopped waiting for %s run %s, which is still %s: %w. Check on it with %s",
				run.DAGID, run.DAGRunID, run.State, ctx.Err(), q.t.suggest("runs get "+run.DAGID+" "+run.DAGRunID))
		case <-timer.C:
		}
		next, err := client.GetDAGRun(ctx, run.DAGID, run.DAGRunID)
		if err != nil {
			return run, false, fmt.Errorf("could not check on %s run %s, which was triggered and may still be running: %w. "+
				"Check on it with %s", run.DAGID, run.DAGRunID, err, q.t.suggest("runs get "+run.DAGID+" "+run.DAGRunID))
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
			"check on it with %s\n",
			result.DAGID, result.RunID, result.State, formatDuration(result.ElapsedSeconds),
			q.t.suggest("runs get "+result.DAGID+" "+result.RunID))
		return err
	}
	if _, err := fmt.Fprintf(w, "%s run %s: %s%s\n",
		result.DAGID, result.RunID, result.State, afterElapsed(result.ElapsedSeconds)); err != nil {
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
	instances, err := allTaskInstances(ctx, client, dagID, runID, q.t.suggest("runs tasks "+dagID+" "+runID))
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
//
// listAll is the command that lists every one of them, which the error names
// when there are more than a single command reads.
func allTaskInstances(ctx context.Context, client *airflowapi.Client, dagID, runID, listAll string) ([]airflowapi.TaskInstance, error) {
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
	return all, errors.New("this run has more task instances than one command reads; list them with " + listAll)
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
		_, err := fmt.Fprintf(w, "\nread why %s failed with %s\n", row.TaskID, q.t.suggest(command))
		return err
	}
	return nil
}

func newRunsWaitCmd(q *query) *cobra.Command {
	var timing waitFlags
	var until string
	cmd := &cobra.Command{
		Use:   "wait <DAG_ID> <RUN_ID>",
		Short: "Wait for a run, or one of its tasks, to reach a state",
		Long: "Check on a run every --poll-interval until it succeeds, fails, or --timeout passes, and report it " +
			"as " + q.t.suggest("runs trigger-wait") + " does. It waits on a run that is already going, such as a " +
			"scheduled one.\n\n" +
			"--until waits for one task instance instead, written TASK_ID=STATE or TASK_ID:MAP_INDEX=STATE for " +
			"one expansion of a mapped task: for example review:0=awaiting_input, to act once a human-in-the-loop " +
			"task is waiting for an answer. The wait ends early when the task instance settles in a different " +
			"state it will not leave by itself, or the run finishes first. A task instance that does not exist " +
			"yet, such as an expansion not yet mapped, is waited for.\n\n" +
			"--timeout and --poll-interval take a number of seconds (300) or a duration (5m, 1h30m).\n\n" +
			"Exit status: 0 when the run succeeded or the task reached the state, " + strconv.Itoa(exitRunFailed) +
			" when the run failed or the task cannot reach the state (or the command itself failed), " +
			strconv.Itoa(exitWaitTimeout) + " when the wait timed out. Stopping the wait never stops the run.",
		Args: cobra.ExactArgs(2),
		Example: afExample(q.t,
			"Wait for a run to finish", "runs wait <DAG_ID> <RUN_ID>",
			"Wait until a task is waiting for a human answer", "runs wait <DAG_ID> <RUN_ID> --until <TASK_ID>=awaiting_input",
			"Wait for one expansion of a mapped task to succeed", "runs wait <DAG_ID> <RUN_ID> --until <TASK_ID>:3=success --timeout 10m",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			wait, err := timing.options()
			if err != nil {
				return err
			}
			if until == "" {
				return q.runRunsWait(cmd.Context(), args[0], args[1], wait)
			}
			target, err := parseUntil(until)
			if err != nil {
				return err
			}
			return q.runRunsWaitUntil(cmd.Context(), args[0], args[1], target, wait)
		},
	}
	timing.register(cmd)
	cmd.Flags().StringVar(&until, "until", "", "Wait for a task instance instead: TASK_ID=STATE, or TASK_ID:MAP_INDEX=STATE")
	return cmd
}

func (q *query) runRunsWait(ctx context.Context, dagID, runID string, wait waitOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	run, err := client.GetDAGRun(ctx, dagID, runID)
	if err != nil {
		return err
	}
	run.DAGID, run.DAGRunID = dagID, runID
	started := time.Now()
	if !runFinished(run.State) {
		fmt.Fprintf(q.d.Stderr, "%s run %s is %s; waiting up to %s for it to finish\n", dagID, runID, run.State, formatDuration(wait.timeout.Seconds()))
	}
	run, timedOut, err := q.awaitRun(ctx, client, run, started.Add(wait.timeout), wait.poll)
	if err != nil {
		return err
	}
	return q.reportWaitedRun(ctx, r, client, run, false, timedOut, started)
}

// untilTarget is the task instance --until waits on, and the state it waits
// for.
type untilTarget struct {
	taskID   string
	mapIndex int
	state    string
}

func (t untilTarget) name() string {
	if t.mapIndex >= 0 {
		return t.taskID + "[" + strconv.Itoa(t.mapIndex) + "]"
	}
	return t.taskID
}

// parseUntil reads TASK_ID=STATE or TASK_ID:MAP_INDEX=STATE. A task id cannot
// hold a colon or an equals sign, so the last colon before the equals sign is
// the map index's.
func parseUntil(value string) (untilTarget, error) {
	invalid := fmt.Errorf("--until must be TASK_ID=STATE or TASK_ID:MAP_INDEX=STATE, such as review:0=awaiting_input, not %q", value)
	task, state, ok := strings.Cut(value, "=")
	if !ok || task == "" || state == "" {
		return untilTarget{}, invalid
	}
	target := untilTarget{taskID: task, mapIndex: -1, state: strings.ToLower(state)}
	if id, index, mapped := strings.Cut(task, ":"); mapped {
		n, err := strconv.Atoi(index)
		if id == "" || err != nil || n < 0 {
			return untilTarget{}, invalid
		}
		target.taskID, target.mapIndex = id, n
	}
	return target, nil
}

// taskSettled reports whether a task instance's state is one it will not leave
// by itself. Every other state — none yet, scheduled, queued, running, deferred,
// up_for_retry, awaiting_input and the rest — can still move.
func taskSettled(state string) bool {
	switch state {
	case stateSuccess, stateFailed, stateUpstreamFailed, "skipped", "removed":
		return true
	}
	return false
}

// waitedTask is what `runs wait --until` reports: the task instance as it last
// stood, the state it was waited for, and how the wait ended. Reached and
// TimedOut carry no omitempty: each answers a question asked every time.
type waitedTask struct {
	DAGID    string `json:"dag_id"`
	RunID    string `json:"dag_run_id"`
	TaskID   string `json:"task_id"`
	MapIndex int    `json:"map_index"`
	// State is "" when the task instance never appeared.
	State          string  `json:"state"`
	Until          string  `json:"until"`
	Reached        bool    `json:"reached"`
	TimedOut       bool    `json:"timed_out"`
	RunState       string  `json:"run_state"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
}

func (q *query) runRunsWaitUntil(ctx context.Context, dagID, runID string, target untilTarget, wait waitOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	started := time.Now()
	result, err := q.awaitTask(ctx, client, dagID, runID, target, started.Add(wait.timeout), wait.poll)
	if err != nil {
		return err
	}
	result.ElapsedSeconds = hundredths(time.Since(started).Seconds())
	if err := r.Emit(result, func(w io.Writer) error { return q.renderWaitedTask(w, result) }); err != nil {
		return err
	}
	switch {
	case result.Reached:
		return nil
	case result.TimedOut:
		return &cliout.ExitError{Code: exitWaitTimeout}
	}
	return &cliout.ExitError{Code: exitRunFailed}
}

// awaitTask checks on a task instance, and its run, until the instance reaches
// the target state, settles in another, the run finishes without it, or the
// deadline passes. Each change is noted on stderr as it is seen.
func (q *query) awaitTask(ctx context.Context, client *airflowapi.Client, dagID, runID string, target untilTarget, deadline time.Time, poll time.Duration) (waitedTask, error) {
	result := waitedTask{DAGID: dagID, RunID: runID, TaskID: target.taskID, MapIndex: target.mapIndex, Until: target.state}
	check := q.t.suggest("runs tasks " + dagID + " " + runID)
	last := "\x00"
	for {
		state, err := taskState(ctx, client, dagID, runID, target)
		if err != nil {
			return result, fmt.Errorf("could not check on %s in %s run %s: %w. Check on it with %s", target.name(), dagID, runID, err, check)
		}
		run, err := client.GetDAGRun(ctx, dagID, runID)
		if err != nil {
			return result, fmt.Errorf("could not check on %s run %s: %w. Check on it with %s", dagID, runID, err, check)
		}
		result.State, result.RunState = state, run.State
		if state != last {
			fmt.Fprintf(q.d.Stderr, "%s in %s run %s: %s\n", target.name(), dagID, runID, orDash(state))
			last = state
		}
		switch {
		case state == target.state:
			result.Reached = true
			return result, nil
		case taskSettled(state), runFinished(run.State):
			return result, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			result.TimedOut = true
			return result, nil
		}
		timer := time.NewTimer(min(poll, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, fmt.Errorf("stopped waiting for %s in %s run %s, which is %s: %w. Check on it with %s",
				target.name(), dagID, runID, orDash(state), ctx.Err(), check)
		case <-timer.C:
		}
	}
}

// taskState reads one task instance's state. One that does not exist yet —
// an expansion not yet mapped, or a task not yet scheduled on Airflow 2 — has
// no state, and the wait carries on.
func taskState(ctx context.Context, client *airflowapi.Client, dagID, runID string, target untilTarget) (string, error) {
	var instance airflowapi.TaskInstance
	var err error
	if target.mapIndex >= 0 {
		instance, err = client.GetMappedTaskInstance(ctx, dagID, runID, target.taskID, target.mapIndex)
	} else {
		instance, err = client.GetTaskInstance(ctx, dagID, runID, target.taskID)
	}
	if errors.Is(err, airflowapi.ErrNotFound) {
		return "", nil
	}
	return instance.State, err
}

func orDash(state string) string {
	if state == "" {
		return "-"
	}
	return state
}

func (q *query) renderWaitedTask(w io.Writer, result waitedTask) error {
	name := untilTarget{taskID: result.TaskID, mapIndex: result.MapIndex}.name()
	var err error
	switch {
	case result.Reached:
		_, err = fmt.Fprintf(w, "%s in %s run %s: %s%s\n", name, result.DAGID, result.RunID, result.State, afterElapsed(result.ElapsedSeconds))
	case result.TimedOut:
		_, err = fmt.Fprintf(w, "%s in %s run %s is still %s after %s, not %s; stopped waiting, and the run carries on\n",
			name, result.DAGID, result.RunID, orDash(result.State), formatDuration(result.ElapsedSeconds), result.Until)
	case taskSettled(result.State):
		_, err = fmt.Fprintf(w, "%s in %s run %s ended %s, so it will not reach %s\n",
			name, result.DAGID, result.RunID, result.State, result.Until)
	default:
		_, err = fmt.Fprintf(w, "%s run %s finished %s with %s %s, so it will not reach %s\n",
			result.DAGID, result.RunID, result.RunState, name, orDash(result.State), result.Until)
	}
	return err
}
