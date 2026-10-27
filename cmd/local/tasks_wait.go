package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// defaultWaitState is what `tasks wait` waits for unless --until says
// otherwise: the task finishing well, which is what waiting on a task usually
// means.
const defaultWaitState = stateSuccess

func newTasksWaitCmd(q *query) *cobra.Command {
	var opts struct {
		mapIndex int
		until    string
	}
	var timing waitFlags
	cmd := &cobra.Command{
		Use:   "wait <DAG_ID> <RUN_ID> <TASK_ID>",
		Short: "Wait for a task instance to reach a state",
		Long: "Check on one task instance every --poll-interval until it reaches the state --until names, " +
			"success unless told otherwise, or --timeout passes. The wait ends early when the instance settles " +
			"in another state it will not leave by itself (success, failed, upstream_failed, skipped, removed), " +
			"or when its run finishes without it. An instance that does not exist yet, such as an expansion " +
			"of a mapped task not yet mapped, is waited for.\n\n" +
			"A human-in-the-loop task waits for its answer in awaiting_input on Airflow 3.3 and later. On " +
			"3.1 and 3.2 it is deferred, like any other task waiting on a trigger.\n\n" +
			"--timeout and --poll-interval take a number of seconds (300) or a duration (5m, 1h30m).\n\n" +
			"Exit status: 0 when the task reached the state, " + strconv.Itoa(exitRunFailed) + " when it cannot " +
			"reach it (or the command itself failed), " + strconv.Itoa(exitWaitTimeout) + " when the wait timed " +
			"out. Stopping the wait never stops the task.",
		Args: cobra.ExactArgs(taskInstanceArgs),
		Example: afExample(q.t,
			"Wait for a task to succeed", "tasks wait <DAG_ID> <RUN_ID> <TASK_ID>",
			"Wait until a task is waiting for a human answer", "tasks wait <DAG_ID> <RUN_ID> <TASK_ID> --until awaiting_input",
			"Wait for one expansion of a mapped task", "tasks wait <DAG_ID> <RUN_ID> <TASK_ID> --map-index 3 --timeout 10m",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			wait, err := timing.options()
			if err != nil {
				return err
			}
			state := strings.ToLower(strings.TrimSpace(opts.until))
			if state == "" {
				return errors.New("--until names the state to wait for, such as success or awaiting_input")
			}
			target := taskTarget{taskID: args[2], mapIndex: opts.mapIndex, state: state}
			return q.runTasksWait(cmd.Context(), args[0], args[1], target, wait)
		},
	}
	cmd.Flags().IntVarP(&opts.mapIndex, "map-index", "m", -1, "Which expansion of a mapped task to wait on (-1 for an unmapped task)")
	cmd.Flags().StringVar(&opts.until, "until", defaultWaitState, "The state to wait for")
	timing.register(cmd)
	return cmd
}

// taskTarget is the task instance `tasks wait` waits on, and the state it
// waits for.
type taskTarget struct {
	taskID   string
	mapIndex int
	state    string
}

func (t taskTarget) name() string {
	if t.mapIndex >= 0 {
		return t.taskID + "[" + strconv.Itoa(t.mapIndex) + "]"
	}
	return t.taskID
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

// waitedTask is what `tasks wait` reports: the task instance as it last
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

func (q *query) runTasksWait(ctx context.Context, dagID, runID string, target taskTarget, wait waitOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	// A task instance that is not there yet is waited for, so a task the Dag
	// does not define, or a mapped task named without its expansion, would
	// otherwise be waited for until the run ended: once a mapped task expands
	// it has no single instance for Airflow to return. Airflow's answer to a
	// task it has not got says which of the Dag and the task is missing. The
	// other way round is not refused: a task inside a mapped task group has
	// expansions without being mapped itself.
	task, err := client.GetTask(ctx, dagID, target.taskID)
	if err != nil {
		return err
	}
	if task.IsMapped && target.mapIndex < 0 {
		return fmt.Errorf("%s is a mapped task, so name the expansion to wait on with --map-index", target.taskID)
	}
	started := time.Now()
	result, err := q.awaitTask(ctx, client, dagID, runID, target, wait)
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
// timeout passes. Each change is noted on stderr as it is seen, and the wait
// is announced once the first check has shown there is one.
func (q *query) awaitTask(ctx context.Context, client *airflowapi.Client, dagID, runID string, target taskTarget, wait waitOptions) (waitedTask, error) {
	result := waitedTask{DAGID: dagID, RunID: runID, TaskID: target.taskID, MapIndex: target.mapIndex, Until: target.state}
	check := q.t.suggest("runs tasks " + dagID + " " + runID)
	deadline := time.Now().Add(wait.timeout)
	announced := false
	last := "\x00"
	for {
		// The run before the task: a run read as finished means the task read
		// after it has settled too. The other way round, a task that reached
		// the state between the two reads would be reported as cut off by its
		// run.
		run, err := client.GetDAGRun(ctx, dagID, runID)
		if err != nil {
			return result, fmt.Errorf("could not check on %s run %s: %w. Check on it with %s", dagID, runID, err, check)
		}
		state, err := taskState(ctx, client, dagID, runID, target)
		if err != nil {
			return result, fmt.Errorf("could not check on %s in %s run %s: %w. Check on it with %s", target.name(), dagID, runID, err, check)
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
		if !announced {
			fmt.Fprintf(q.d.Stderr, "waiting up to %s for %s in %s run %s to be %s\n",
				formatDuration(wait.timeout.Seconds()), target.name(), dagID, runID, target.state)
			announced = true
		}
		timer := time.NewTimer(min(wait.poll, remaining))
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
func taskState(ctx context.Context, client *airflowapi.Client, dagID, runID string, target taskTarget) (string, error) {
	instance, err := fetchTaskInstance(ctx, client, dagID, runID, target.taskID, target.mapIndex)
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
	name := taskTarget{taskID: result.TaskID, mapIndex: result.MapIndex}.name()
	var err error
	switch {
	case result.Reached:
		_, err = fmt.Fprintf(w, "%s in %s run %s: %s%s\n", name, result.DAGID, result.RunID, result.State, afterElapsed(result.ElapsedSeconds))
	case result.TimedOut:
		_, err = fmt.Fprintf(w, "%s in %s run %s is still %s after %s, not %s; stopped waiting, and the run carries on\n",
			name, result.DAGID, result.RunID, orDash(result.State), formatDuration(result.ElapsedSeconds), result.Until)
	case taskSettled(result.State) && taskSettled(result.Until):
		_, err = fmt.Fprintf(w, "%s in %s run %s ended %s, so it will not reach %s\n",
			name, result.DAGID, result.RunID, result.State, result.Until)
	case taskSettled(result.State):
		// A state a task passes through can fall between two polls, so an
		// ended task may well have been there; it just was not seen.
		_, err = fmt.Fprintf(w, "%s in %s run %s ended %s and was not seen %s; a state a task passes through can fall between two polls\n",
			name, result.DAGID, result.RunID, result.State, result.Until)
	default:
		_, err = fmt.Fprintf(w, "%s run %s finished %s with %s %s, so it will not reach %s\n",
			result.DAGID, result.RunID, result.RunState, name, orDash(result.State), result.Until)
	}
	return err
}
