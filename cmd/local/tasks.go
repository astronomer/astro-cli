package local

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// newTasksCmd builds the `tasks` family over whichever Airflow the target
// names.
func newTasksCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "tasks",
		Short: "Read a DAG's tasks, their runs, and their logs",
		Long: "Work with tasks on " + t.which() + ": what a DAG defines, what one run " +
			"of a task did, its log, and clearing tasks so they run again.",
	},
		newTasksListCmd,
		newTasksGetCmd,
		newTasksInstanceCmd,
		newTasksLogsCmd,
		newTasksClearCmd,
	)
}

// taskInstanceArgs is how many names address one task instance: the DAG, the
// run, and the task.
const taskInstanceArgs = 3

// taskRow is a task's definition — what the DAG says, not what a run did.
type taskRow struct {
	TaskID          string   `json:"task_id"`
	DisplayName     string   `json:"task_display_name,omitempty"`
	Operator        string   `json:"operator_name,omitempty"`
	Owner           string   `json:"owner,omitempty"`
	Pool            string   `json:"pool,omitempty"`
	Queue           string   `json:"queue,omitempty"`
	Retries         int      `json:"retries"`
	TriggerRule     string   `json:"trigger_rule,omitempty"`
	IsMapped        bool     `json:"is_mapped"`
	DependsOnPast   bool     `json:"depends_on_past"`
	DownstreamTasks []string `json:"downstream_task_ids,omitempty"`
	StartDate       string   `json:"start_date,omitempty"`
	EndDate         string   `json:"end_date,omitempty"`
}

func newTaskRow(t airflowapi.Task) taskRow {
	return taskRow{
		TaskID:          t.TaskID,
		DisplayName:     t.TaskDisplayName,
		Operator:        t.Operator,
		Owner:           t.Owner,
		Pool:            t.Pool,
		Queue:           t.Queue,
		Retries:         int(t.Retries),
		TriggerRule:     t.TriggerRule,
		IsMapped:        t.IsMapped,
		DependsOnPast:   t.DependsOnPast,
		DownstreamTasks: t.DownstreamTasks,
		StartDate:       stamp(t.StartDate),
		EndDate:         stamp(t.EndDate),
	}
}

// taskInstanceRow is one task's execution inside a run.
type taskInstanceRow struct {
	TaskID      string `json:"task_id"`
	DAGID       string `json:"dag_id"`
	RunID       string `json:"dag_run_id"`
	State       string `json:"state"`
	MapIndex    int    `json:"map_index"`
	TryNumber   int    `json:"try_number"`
	MaxTries    int    `json:"max_tries"`
	Operator    string `json:"operator,omitempty"`
	Pool        string `json:"pool,omitempty"`
	Queue       string `json:"queue,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Note        string `json:"note,omitempty"`
	LogicalDate string `json:"logical_date,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
	// Duration is the seconds Airflow measured, kept as a number so a consumer
	// can compare and sum it; the table renders it for people.
	Duration float64 `json:"duration_seconds,omitempty"`
}

func newTaskInstanceRow(t airflowapi.TaskInstance) taskInstanceRow {
	return taskInstanceRow{
		TaskID:      t.TaskID,
		DAGID:       t.DAGID,
		RunID:       t.DAGRunID,
		State:       t.State,
		MapIndex:    t.MapIndex,
		TryNumber:   t.TryNumber,
		MaxTries:    t.MaxTries,
		Operator:    t.Operator,
		Pool:        t.Pool,
		Queue:       t.Queue,
		Hostname:    t.Hostname,
		Note:        t.Note,
		LogicalDate: stamp(t.LogicalDate),
		StartDate:   stamp(t.StartDate),
		EndDate:     stamp(t.EndDate),
		Duration:    max(t.Duration, 0),
	}
}

func newTasksListCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "list <DAG_ID>",
		Short: "List the tasks a DAG defines",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runTasksList(cmd.Context(), args[0])
		},
	}
}

func (q *query) runTasksList(ctx context.Context, dagID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	list, err := client.ListTasks(ctx, dagID)
	if err != nil {
		return err
	}
	rows := mapRows(list.Tasks, newTaskRow)
	return emitRows(r, rows, len(rows), newTaskList, renderTaskTable)
}

func renderTaskTable(w io.Writer, rows []taskRow) error {
	return renderTable(w, rows, "This DAG defines no tasks.",
		[]string{"TASK_ID", "OPERATOR", "OWNER", "TRIGGER_RULE", "DOWNSTREAM"},
		func(row taskRow) []string {
			return []string{row.TaskID, row.Operator, row.Owner, row.TriggerRule, strings.Join(row.DownstreamTasks, ",")}
		})
}

func newTasksGetCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "get <DAG_ID> <TASK_ID>",
		Short: "Show one task's definition",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runTasksGet(cmd.Context(), args[0], args[1])
		},
	}
}

func (q *query) runTasksGet(ctx context.Context, dagID, taskID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	task, err := client.GetTask(ctx, dagID, taskID)
	if err != nil {
		return err
	}
	return emitDetail(r, newTaskRow(task), taskFields)
}

func taskFields(row taskRow) []field {
	return []field{
		{"task id", row.TaskID},
		{"display name", row.DisplayName},
		{"operator", row.Operator},
		{"owner", row.Owner},
		{"pool", row.Pool},
		{"queue", row.Queue},
		// Zero retries is a real and common answer, so it shows rather than
		// leaving the reader to guess whether this generation sent the field.
		{"retries", count(row.Retries)},
		{"trigger rule", row.TriggerRule},
		{"mapped", onlyIf(row.IsMapped, "yes")},
		{"depends on past", onlyIf(row.DependsOnPast, "yes")},
		{"downstream", strings.Join(row.DownstreamTasks, ", ")},
		{"start", row.StartDate},
		{"end", row.EndDate},
	}
}

func newTasksInstanceCmd(q *query) *cobra.Command {
	var mapIndex int
	cmd := &cobra.Command{
		Use:   "instance <DAG_ID> <RUN_ID> <TASK_ID>",
		Short: "Show what one run of a task did",
		Args:  cobra.ExactArgs(taskInstanceArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runTasksInstance(cmd.Context(), args[0], args[1], args[2], mapIndex)
		},
	}
	cmd.Flags().IntVarP(&mapIndex, "map-index", "m", -1, "Which expansion of a mapped task to show (-1 for an unmapped task)")
	return cmd
}

func (q *query) runTasksInstance(ctx context.Context, dagID, runID, taskID string, mapIndex int) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	var instance airflowapi.TaskInstance
	if mapIndex >= 0 {
		instance, err = client.GetMappedTaskInstance(ctx, dagID, runID, taskID, mapIndex)
	} else {
		instance, err = client.GetTaskInstance(ctx, dagID, runID, taskID)
	}
	if err != nil {
		return err
	}
	return emitDetail(r, newTaskInstanceRow(instance), taskInstanceFields)
}

func taskInstanceFields(row taskInstanceRow) []field {
	return []field{
		{"task id", row.TaskID},
		{"dag id", row.DAGID},
		{"run id", row.RunID},
		{"state", row.State},
		{"try", fmt.Sprintf("%d of %d", row.TryNumber, row.MaxTries+1)},
		{"map index", onlyIf(row.MapIndex >= 0, count(row.MapIndex))},
		{"operator", row.Operator},
		{"pool", row.Pool},
		{"queue", row.Queue},
		{"host", row.Hostname},
		{"logical date", row.LogicalDate},
		{"start", row.StartDate},
		{"end", row.EndDate},
		{"duration", formatDuration(row.Duration)},
		{"note", row.Note},
	}
}

func renderTaskInstanceTable(w io.Writer, rows []taskInstanceRow) error {
	headers, cells := withMapIndexColumn(rows,
		[]string{"DAG_ID", "RUN_ID", "TASK_ID", "STATE", "TRY", "START", "DURATION"},
		func(row taskInstanceRow) []string {
			return []string{row.DAGID, row.RunID, row.TaskID, row.State, count(row.TryNumber), row.StartDate, formatDuration(row.Duration)}
		})
	return renderTable(w, rows, "No task instances.", headers, cells)
}

// withMapIndexColumn adds MAP_INDEX after TASK_ID when any row is one
// expansion of a mapped task. Without it, the expansions of a task print as
// identical rows. A table with no mapped task keeps its columns as they were.
func withMapIndexColumn(rows []taskInstanceRow, headers []string, cells func(taskInstanceRow) []string) (withHeaders []string, withCells func(taskInstanceRow) []string) {
	if !slices.ContainsFunc(rows, func(row taskInstanceRow) bool { return row.MapIndex >= 0 }) {
		return headers, cells
	}
	at := slices.Index(headers, "TASK_ID") + 1
	return slices.Insert(slices.Clone(headers), at, "MAP_INDEX"), func(row taskInstanceRow) []string {
		return slices.Insert(cells(row), at, onlyIf(row.MapIndex >= 0, count(row.MapIndex)))
	}
}

// groupMappedInstances puts the expansions of each mapped task together, in
// map index order, where the task first appears. Airflow's own order scatters
// them.
func groupMappedInstances(rows []taskInstanceRow) {
	first := make(map[string]int, len(rows))
	for i := range rows {
		if _, seen := first[rows[i].TaskID]; !seen {
			first[rows[i].TaskID] = i
		}
	}
	slices.SortStableFunc(rows, func(a, b taskInstanceRow) int {
		return cmp.Or(cmp.Compare(first[a.TaskID], first[b.TaskID]), cmp.Compare(a.MapIndex, b.MapIndex))
	})
}

func newTasksLogsCmd(q *query) *cobra.Command {
	var opts struct {
		try      int
		mapIndex int
	}
	cmd := &cobra.Command{
		Use:   "logs <DAG_ID> <RUN_ID> <TASK_ID>",
		Short: "Print one try's log for a task instance",
		Long: "Print the whole log Airflow holds for one try of a task instance.\n\nThe content is rendered as " +
			"Airflow sent it: Airflow 3 serves structured entries and Airflow 2 serves one string, and neither " +
			"is reshaped here.",
		Args: cobra.ExactArgs(taskInstanceArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.try < 1 {
				return fmt.Errorf("--try counts attempts from 1, so %d is not one", opts.try)
			}
			o := airflowapi.TaskLogsOptions{TryNumber: opts.try}
			// Index 0 is a real expansion of a mapped task, so "no index" has to
			// be a value outside the range rather than the zero one.
			if opts.mapIndex >= 0 {
				index := opts.mapIndex
				o.MapIndex = &index
			}
			return q.runTasksLogs(cmd.Context(), args[0], args[1], args[2], o)
		},
	}
	cmd.Flags().IntVar(&opts.try, "try", 1, "Which attempt to read, counting from 1")
	cmd.Flags().IntVarP(&opts.mapIndex, "map-index", "m", -1, "Which expansion of a mapped task to read (-1 for an unmapped task)")
	return cmd
}

// taskLog is one try's log as this surface reports it. Content is the rendered
// text in both renderings, so a reader parsing json gets the same lines the
// terminal showed rather than one generation's wire shape.
//
// Airflow 2 also answers with a continuation token for reading a growing log in
// chunks. It is not carried here: nothing in this command sends one back, and
// an output field a caller cannot act on is a promise of paging that does not
// exist.
type taskLog struct {
	DAGID     string `json:"dag_id"`
	RunID     string `json:"dag_run_id"`
	TaskID    string `json:"task_id"`
	TryNumber int    `json:"try_number"`
	Content   string `json:"content"`
}

func (q *query) runTasksLogs(ctx context.Context, dagID, runID, taskID string, opts airflowapi.TaskLogsOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	log, err := client.TaskLogs(ctx, dagID, runID, taskID, opts)
	if err != nil {
		return err
	}
	text := log.Text()
	result := taskLog{
		DAGID:     dagID,
		RunID:     runID,
		TaskID:    taskID,
		TryNumber: opts.TryNumber,
		Content:   text,
	}
	return r.Emit(result, func(w io.Writer) error { return writeText(w, text) })
}

func newTasksClearCmd(q *query) *cobra.Command {
	var opts struct {
		dryRun         bool
		onlyFailed     bool
		downstream     bool
		upstream       bool
		noResetDAGRuns bool
		yes            bool
	}
	cmd := &cobra.Command{
		Use:   "clear <DAG_ID> <RUN_ID> <TASK_ID>...",
		Short: "Clear task instances so they run again",
		Long: "Reset the named task instances in one run so the scheduler runs them again. Pass --dry-run to see " +
			"what would be cleared without clearing it.\n\nThe affected runs go back into the queued state, " +
			"which is what makes the scheduler pick the cleared tasks up; without it a run in a terminal state " +
			"keeps it and the tasks never start. --no-reset-dagruns leaves the run's state alone.",
		Args: cobra.MinimumNArgs(taskInstanceArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runTasksClear(cmd.Context(), args[0], airflowapi.ClearTaskInstancesOptions{
				DAGRunID:          args[1],
				TaskIDs:           args[2:],
				DryRun:            opts.dryRun,
				OnlyFailed:        opts.onlyFailed,
				IncludeDownstream: opts.downstream,
				IncludeUpstream:   opts.upstream,
				ResetDAGRuns:      !opts.noResetDAGRuns,
			}, opts.yes)
		},
	}
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Report what would be cleared and change nothing")
	cmd.Flags().BoolVar(&opts.onlyFailed, "only-failed", false, "Skip task instances that did not fail")
	cmd.Flags().BoolVar(&opts.downstream, "downstream", false, "Also clear tasks downstream of these")
	cmd.Flags().BoolVar(&opts.upstream, "upstream", false, "Also clear tasks upstream of these")
	cmd.Flags().BoolVar(&opts.noResetDAGRuns, "no-reset-dagruns", false, "Leave the affected runs in the state they are in")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

// clearedTasks is what a task clear reports.
type clearedTasks struct {
	DAGID  string            `json:"dag_id"`
	RunID  string            `json:"dag_run_id"`
	DryRun bool              `json:"dry_run"`
	Tasks  []taskInstanceRow `json:"task_instances,omitempty"`
}

func (q *query) runTasksClear(ctx context.Context, dagID string, opts airflowapi.ClearTaskInstancesOptions, yes bool) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	if !opts.DryRun {
		question := fmt.Sprintf("Clear %s in run %s of %s so they run again?", strings.Join(opts.TaskIDs, ", "), opts.DAGRunID, dagID)
		if err := q.confirmUnless(yes, question); err != nil {
			return err
		}
	}
	list, err := client.ClearTaskInstances(ctx, dagID, opts)
	if err != nil {
		return err
	}
	cleared := clearedTasks{
		DAGID:  dagID,
		RunID:  opts.DAGRunID,
		DryRun: opts.DryRun,
		Tasks:  mapRows(list.TaskInstances, newTaskInstanceRow),
	}
	return r.Emit(cleared, func(w io.Writer) error { return renderClearedTasks(w, cleared) })
}

func renderClearedTasks(w io.Writer, cleared clearedTasks) error {
	verb := "cleared"
	if cleared.DryRun {
		verb = "would clear"
	}
	if _, err := fmt.Fprintf(w, "%s %d task instance(s) in run %s:\n", verb, len(cleared.Tasks), cleared.RunID); err != nil {
		return err
	}
	return renderTaskInstanceTable(w, cleared.Tasks)
}
