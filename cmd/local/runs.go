package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// newRunsCmd builds the `runs` family over whichever Airflow the target names.
func newRunsCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "runs",
		Short: "List, trigger, and clear DAG runs on an Airflow",
		Long: "Work with the runs on " + t.which() + ": list them, read one, see what its " +
			"tasks did, start one and optionally wait for it to finish, diagnose a failed one, and delete or " +
			"clear one.",
	},
		newRunsListCmd,
		newRunsGetCmd,
		newRunsTasksCmd,
		newRunsTriggerCmd,
		newRunsTriggerWaitCmd,
		newRunsDiagnoseCmd,
		newRunsDeleteCmd,
		newRunsClearCmd,
	)
}

// runRow is a DAG run as this surface reports it. The origin folds Airflow 3's
// triggered_by and Airflow 2's external_trigger into the one question a reader
// is asking: did a person start this, or the schedule?
type runRow struct {
	DAGID       string `json:"dag_id"`
	RunID       string `json:"dag_run_id"`
	State       string `json:"state"`
	RunType     string `json:"run_type,omitempty"`
	LogicalDate string `json:"logical_date,omitempty"`
	QueuedAt    string `json:"queued_at,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
	// Duration is seconds, so a consumer can compare and sum it. The table
	// renders it for people; json keeps the number.
	Duration    float64        `json:"duration_seconds,omitempty"`
	TriggeredBy string         `json:"triggered_by,omitempty"`
	Note        string         `json:"note,omitempty"`
	Conf        map[string]any `json:"conf,omitempty"`
}

func newRunRow(r airflowapi.DAGRun) runRow {
	row := runRow{
		DAGID:       r.DAGID,
		RunID:       r.DAGRunID,
		State:       r.State,
		RunType:     r.RunType,
		LogicalDate: stamp(r.LogicalDate),
		QueuedAt:    stamp(r.QueuedAt),
		StartDate:   stamp(r.StartDate),
		EndDate:     stamp(r.EndDate),
		Duration:    span(r.StartDate, r.EndDate),
		TriggeredBy: r.TriggeredBy,
		Note:        r.Note,
		Conf:        r.Conf,
	}
	if row.TriggeredBy == "" && r.ExternalTrigger {
		// Airflow 2 says only whether the run came from outside the schedule.
		row.TriggeredBy = "external"
	}
	return row
}

func newRunsListCmd(q *query) *cobra.Command {
	var list listFlags
	var opts struct {
		dagID        string
		states       []string
		startDateGTE string
		startDateLTE string
	}
	cmd := &cobra.Command{
		Use:   "list [DAG_ID]",
		Short: "List runs, most recent first",
		Long:  "List runs, most recent first: those of one DAG when it is named, every DAG's otherwise.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dagID, err := dagIDArg(args, opts.dagID)
			if err != nil {
				return err
			}
			from, err := parseBound("--start-date-gte", opts.startDateGTE)
			if err != nil {
				return err
			}
			to, err := parseBound("--start-date-lte", opts.startDateLTE)
			if err != nil {
				return err
			}
			return q.runRunsList(cmd.Context(), dagID, airflowapi.ListDAGRunsOptions{
				ListOptions:   list.options(),
				States:        opts.states,
				StartDateFrom: from,
				StartDateTo:   to,
			})
		},
	}
	// Most recent first without being asked: a bare `astro af runs list` is nearly
	// always "what just happened", and Airflow's own default order is not that.
	addListFlags(cmd, &list, "-start_date")
	cmd.Flags().StringVar(&opts.dagID, "dag-id", "", "Only runs of this DAG, the same as naming it as the argument (default: every DAG)")
	cmd.Flags().StringSliceVarP(&opts.states, "state", "s", nil, "Only runs in these states, such as running or failed (repeatable)")
	cmd.Flags().StringVar(&opts.startDateGTE, "start-date-gte", "", "Only runs that started at or after this RFC 3339 time")
	cmd.Flags().StringVar(&opts.startDateLTE, "start-date-lte", "", "Only runs that started at or before this RFC 3339 time")
	return cmd
}

// dagIDArg is the DAG a list is narrowed to, named either as the argument or
// with --dag-id. Both may be given as long as they agree.
func dagIDArg(args []string, flag string) (string, error) {
	if len(args) == 0 {
		return flag, nil
	}
	if flag != "" && flag != args[0] {
		return "", fmt.Errorf("the DAG_ID argument %q and --dag-id %q name different DAGs; give one", args[0], flag)
	}
	return args[0], nil
}

// parseBound reads a time flag, naming the flag when it cannot. An empty value
// leaves the bound open rather than pinning it to year one.
func parseBound(flag, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC 3339 time such as 2024-01-01T00:00:00Z: %w", flag, err)
	}
	return at, nil
}

func (q *query) runRunsList(ctx context.Context, dagID string, opts airflowapi.ListDAGRunsOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return emitList(q, r, opts.ListOptions, func(page airflowapi.ListOptions) ([]airflowapi.DAGRun, int, error) {
		filtered := opts
		filtered.ListOptions = page
		list, err := client.ListDAGRuns(ctx, dagID, filtered)
		return list.DAGRuns, list.TotalEntries, err
	}, newRunRow, newRunList, renderRunTable)
}

func renderRunTable(w io.Writer, rows []runRow) error {
	return renderTable(w, rows, "No runs on this Airflow.",
		[]string{"DAG_ID", "RUN_ID", "STATE", "TYPE", "LOGICAL_DATE", "START", "DURATION"},
		func(row runRow) []string {
			return []string{row.DAGID, row.RunID, row.State, row.RunType, row.LogicalDate, row.StartDate, formatDuration(row.Duration)}
		})
}

func newRunsGetCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "get <DAG_ID> <RUN_ID>",
		Short: "Show one run's state, timing, and configuration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runRunsGet(cmd.Context(), args[0], args[1])
		},
	}
}

// newRunsTasksCmd answers the question a failed run raises: which task, and in
// what state. `tasks list` reads a DAG's definitions and says nothing about a
// run; `tasks instance` reads one instance and needs the task named already.
// Until this, the only view of a whole run's instances was the table
// `runs clear --dry-run` prints, which is a strange door to walk through to
// read something.
func newRunsTasksCmd(q *query) *cobra.Command {
	var f listFlags
	cmd := &cobra.Command{
		Use:   "tasks <DAG_ID> <RUN_ID>",
		Short: "List what each task in a run did, and how it ended",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runRunsTasks(cmd.Context(), args[0], args[1], f)
		},
	}
	addListFlags(cmd, &f, "")
	return cmd
}

func (q *query) runRunsTasks(ctx context.Context, dagID, runID string, f listFlags) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	wire, total, err := listPages(f.options(), func(page airflowapi.ListOptions) ([]airflowapi.TaskInstance, int, error) {
		list, err := client.ListTaskInstances(ctx, dagID, runID, page)
		return list.TaskInstances, list.TotalEntries, err
	})
	if err != nil {
		return err
	}
	rows := mapRows(wire, newTaskInstanceRow)
	if f.orderBy == "" {
		groupMappedInstances(rows)
	}
	return emitListed(q, r, f.options(), rows, total, newTaskInstanceList, renderRunTaskTable)
}

// renderRunTaskTable drops the dag and run columns renderTaskInstanceTable
// carries: both are on the command line, and repeating a long run id on every
// row pushes the states off the right of a terminal.
func renderRunTaskTable(w io.Writer, rows []taskInstanceRow) error {
	headers, cells := withMapIndexColumn(rows,
		[]string{"TASK_ID", "STATE", "TRY", "START", "DURATION"},
		func(row taskInstanceRow) []string {
			return []string{row.TaskID, row.State, count(row.TryNumber), row.StartDate, formatDuration(row.Duration)}
		})
	return renderTable(w, rows, "This run has no task instances.", headers, cells)
}

func (q *query) runRunsGet(ctx context.Context, dagID, runID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	run, err := client.GetDAGRun(ctx, dagID, runID)
	if err != nil {
		return err
	}
	return emitDetail(r, newRunRow(run), runFields)
}

func runFields(row runRow) []field {
	return []field{
		{"dag id", row.DAGID},
		{"run id", row.RunID},
		{"state", row.State},
		{"type", row.RunType},
		{"triggered by", row.TriggeredBy},
		{"logical date", row.LogicalDate},
		{"queued", row.QueuedAt},
		{"start", row.StartDate},
		{"end", row.EndDate},
		{"duration", formatDuration(row.Duration)},
		{"note", row.Note},
		{"conf", renderConf(row.Conf)},
	}
}

func renderConf(conf map[string]any) string {
	if len(conf) == 0 {
		return ""
	}
	//nolint:errcheck // the map came off a JSON decode, so it re-encodes
	//astro:non-output-json // renders a conf map into a text table cell; not a published payload
	encoded, _ := json.Marshal(conf)
	return string(encoded)
}

// triggerFlags is what starting a run takes. `runs trigger` and
// `runs trigger-wait` both register it, so a run started either way is started
// the same way.
type triggerFlags struct {
	conf          string
	runID         string
	logicalDate   string
	note          string
	noAutoUnpause bool
}

func (f *triggerFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.conf, "conf", "c", "", "Run configuration as a JSON object")
	cmd.Flags().StringVar(&f.runID, "run-id", "", "Name the run instead of letting Airflow generate one")
	cmd.Flags().StringVar(&f.logicalDate, "logical-date", "", "The run's logical date, as RFC 3339")
	cmd.Flags().StringVar(&f.note, "note", "", "A note to attach to the run")
	cmd.Flags().BoolVar(&f.noAutoUnpause, "no-auto-unpause", false, "Fail if the DAG is paused instead of unpausing it first")
}

// options validates the flags into a trigger, before anything reaches Airflow.
func (f *triggerFlags) options() (airflowapi.TriggerDAGRunOptions, error) {
	trigger := airflowapi.TriggerDAGRunOptions{
		DAGRunID: f.runID,
		Note:     f.note,
	}
	if f.conf != "" {
		if err := json.Unmarshal([]byte(f.conf), &trigger.Conf); err != nil {
			return trigger, fmt.Errorf("--conf is not valid JSON: %w", err)
		}
	}
	at, err := parseBound("--logical-date", f.logicalDate)
	if err != nil {
		return trigger, err
	}
	trigger.LogicalDate = at
	return trigger, nil
}

func newRunsTriggerCmd(q *query) *cobra.Command {
	var opts triggerFlags
	cmd := &cobra.Command{
		Use:   "trigger <DAG_ID>",
		Short: "Start a run of a DAG",
		Long: "Start a run of a DAG. Airflow names the run and dates it unless --run-id or --logical-date say " +
			"otherwise.\n\nA paused DAG is unpaused first, because a run triggered on a paused DAG is never " +
			"scheduled and the command would look like it worked. The unpause is announced on stderr; pass " +
			"--no-auto-unpause to fail instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			trigger, err := opts.options()
			if err != nil {
				return err
			}
			return q.runRunsTrigger(cmd.Context(), args[0], trigger, !opts.noAutoUnpause)
		},
	}
	opts.register(cmd)
	return cmd
}

// triggeredRun is what a trigger reports: the new run, plus whether the DAG had
// to be unpaused to make it real. Unpaused carries no omitempty — it reports a
// change to the instance, and a reader has to be able to tell "was not paused"
// from a key that was never written.
type triggeredRun struct {
	runRow
	Unpaused bool `json:"unpaused"`
}

func (q *query) runRunsTrigger(ctx context.Context, dagID string, opts airflowapi.TriggerDAGRunOptions, autoUnpause bool) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	run, unpaused, err := q.trigger(ctx, client, dagID, opts, autoUnpause)
	if err != nil {
		return err
	}
	result := triggeredRun{runRow: newRunRow(run), Unpaused: unpaused}
	return r.Emit(result, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "triggered %s run %s (%s)\n", result.DAGID, result.RunID, result.State)
		return werr
	})
}

// trigger starts a run, unpausing the DAG first when it has to, and reports
// whether it did.
func (q *query) trigger(ctx context.Context, client *airflowapi.Client, dagID string, opts airflowapi.TriggerDAGRunOptions, autoUnpause bool) (airflowapi.DAGRun, bool, error) {
	unpaused, err := q.ensureUnpaused(ctx, client, dagID, autoUnpause)
	if err != nil {
		return airflowapi.DAGRun{}, false, err
	}
	run, err := client.TriggerDAGRun(ctx, dagID, opts)
	if err != nil {
		if unpaused {
			// The unpause already happened and outlives this failure, so say so
			// rather than leave the DAG changed by a command that reported none.
			return airflowapi.DAGRun{}, true, fmt.Errorf("%w\n%s is now unpaused; pause it again with `%s`", err, dagID, q.t.suggest("dags pause "+dagID))
		}
		return airflowapi.DAGRun{}, false, err
	}
	return run, unpaused, nil
}

// ensureUnpaused makes the DAG able to run before triggering it, and says so.
// Triggering a paused DAG is accepted by Airflow and then quietly never
// scheduled, which is the worst answer available: the command succeeds and
// nothing happens.
func (q *query) ensureUnpaused(ctx context.Context, client *airflowapi.Client, dagID string, auto bool) (bool, error) {
	dag, err := client.GetDAG(ctx, dagID)
	if err != nil {
		return false, err
	}
	if !dag.IsPaused {
		return false, nil
	}
	if !auto {
		return false, fmt.Errorf("DAG %q is paused, so a new run would never be scheduled; unpause it with `%s`, or drop --no-auto-unpause", dagID, q.t.suggest("dags unpause "+dagID))
	}
	if _, err := client.UnpauseDAG(ctx, dagID); err != nil {
		return false, err
	}
	fmt.Fprintf(q.d.Stderr, "unpaused %s so the run can be scheduled\n", dagID)
	return true, nil
}

func newRunsDeleteCmd(q *query) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <DAG_ID> <RUN_ID>",
		Short: "Delete a run and its task instances",
		Long:  "Delete a run and everything Airflow recorded about it. This cannot be undone.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runRunsDelete(cmd.Context(), args[0], args[1], yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

// deletedRun is what a delete reports.
type deletedRun struct {
	DAGID  string `json:"dag_id"`
	RunID  string `json:"dag_run_id"`
	Status string `json:"status"`
}

func (q *query) runRunsDelete(ctx context.Context, dagID, runID string, yes bool) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	// Ask after resolution, so the question names the Airflow the stderr line
	// just announced rather than one the user has yet to see.
	if err := q.confirmUnless(yes, fmt.Sprintf("Delete run %s of %s? This cannot be undone.", runID, dagID)); err != nil {
		return err
	}
	if err := client.DeleteDAGRun(ctx, dagID, runID); err != nil {
		return err
	}
	return r.Emit(deletedRun{DAGID: dagID, RunID: runID, Status: "deleted"}, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "deleted run %s of %s\n", runID, dagID)
		return werr
	})
}

func newRunsClearCmd(q *query) *cobra.Command {
	var opts struct {
		dryRun bool
		yes    bool
	}
	cmd := &cobra.Command{
		Use:   "clear <DAG_ID> <RUN_ID>",
		Short: "Clear a run so its tasks run again",
		Long: "Reset a run's task instances so the scheduler runs them again. Pass --dry-run to see what would " +
			"be cleared without clearing it.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runRunsClear(cmd.Context(), args[0], args[1], opts.dryRun, opts.yes)
		},
	}
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Report what would be cleared and change nothing")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

// clearedRun is what a clear reports. A dry run lists what it would have
// cleared and a real one reports the run it reset, which is the shape both API
// generations answer in.
type clearedRun struct {
	DAGID  string `json:"dag_id"`
	RunID  string `json:"dag_run_id"`
	DryRun bool   `json:"dry_run"`
	// Tasks is what a dry run would clear.
	Tasks []taskInstanceRow `json:"task_instances,omitempty"`
	// State is the run's state after a real clear.
	State string `json:"state,omitempty"`
}

func (q *query) runRunsClear(ctx context.Context, dagID, runID string, dryRun, yes bool) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	if !dryRun {
		if err := q.confirmUnless(yes, fmt.Sprintf("Clear run %s of %s so its tasks run again?", runID, dagID)); err != nil {
			return err
		}
	}
	result, err := client.ClearDAGRun(ctx, dagID, runID, airflowapi.ClearDAGRunOptions{DryRun: dryRun})
	if err != nil {
		return err
	}
	cleared := clearedRun{
		DAGID:  dagID,
		RunID:  runID,
		DryRun: dryRun,
		Tasks:  mapRows(result.TaskInstances, newTaskInstanceRow),
	}
	if result.DAGRun != nil {
		cleared.State = result.DAGRun.State
	}
	return r.Emit(cleared, func(w io.Writer) error { return renderClearedRun(w, cleared) })
}

func renderClearedRun(w io.Writer, cleared clearedRun) error {
	if cleared.DryRun {
		if _, err := fmt.Fprintf(w, "would clear %d task instance(s) of run %s:\n", len(cleared.Tasks), cleared.RunID); err != nil {
			return err
		}
		return renderTaskInstanceTable(w, cleared.Tasks)
	}
	state := ""
	if cleared.State != "" {
		state = " (" + cleared.State + ")"
	}
	_, err := fmt.Fprintf(w, "cleared run %s of %s%s\n", cleared.RunID, cleared.DAGID, state)
	return err
}
