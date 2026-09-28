package local

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newRunsDiagnoseCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose <DAG_ID> <RUN_ID>",
		Short: "Show a run, every task instance in it, and which of them failed",
		Long: "Read one run and every task instance in it, count the instances by state, and pick out the ones " +
			"that failed or never ran because something upstream failed — the first thing to read when a run " +
			"did not succeed.\n\nThe run is the part the command cannot do without, so a run Airflow does not " +
			"know fails the command. A task listing that fails is reported in its place, beside the run.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runRunsDiagnose(cmd.Context(), args[0], args[1])
		},
	}
}

// runDiagnosis is what `runs diagnose` reports.
type runDiagnosis struct {
	Run           runRow            `json:"run"`
	TaskInstances []taskInstanceRow `json:"task_instances"`
	Summary       runSummary        `json:"summary"`
	// TaskInstancesError is why the task instances could not be listed. When
	// it is set the list and the summary are empty for that reason, not
	// because the run has no tasks.
	TaskInstancesError string `json:"task_instances_error,omitempty"`
}

// runSummary is the run's task instances reduced to what a reader scans for.
type runSummary struct {
	TotalTasks int `json:"total_tasks"`
	// StateCounts is how many instances sit in each state. A task that has not
	// been scheduled yet has no state, and counts under noState.
	StateCounts map[string]int `json:"state_counts"`
	// FailedTasks is the failed and upstream_failed instances, in the order
	// Airflow listed them. Always present, so an empty list reads as "none
	// failed" rather than as a field that was not computed.
	FailedTasks []taskInstanceRow `json:"failed_tasks"`
}

// noState is the state key a task instance without one counts under. Airflow
// sends null for a task not yet scheduled, and a map key cannot be null.
const noState = "none"

func (q *query) runRunsDiagnose(ctx context.Context, dagID, runID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	run, err := client.GetDAGRun(ctx, dagID, runID)
	if err != nil {
		return err
	}
	diagnosis := runDiagnosis{
		Run:           newRunRow(run),
		TaskInstances: []taskInstanceRow{},
		Summary:       runSummary{StateCounts: map[string]int{}, FailedTasks: []taskInstanceRow{}},
	}
	instances, err := allTaskInstances(ctx, client, dagID, runID)
	if err != nil {
		diagnosis.TaskInstancesError = errorText(err)
	} else {
		diagnosis.TaskInstances = mapRows(instances, newTaskInstanceRow)
		diagnosis.Summary = summarize(diagnosis.TaskInstances)
	}
	return r.Emit(diagnosis, func(w io.Writer) error { return q.renderRunDiagnosis(w, diagnosis) })
}

func summarize(rows []taskInstanceRow) runSummary {
	summary := runSummary{
		TotalTasks:  len(rows),
		StateCounts: map[string]int{},
		FailedTasks: failedOnly(rows),
	}
	for i := range rows {
		state := rows[i].State
		if state == "" {
			state = noState
		}
		summary.StateCounts[state]++
	}
	return summary
}

func (q *query) renderRunDiagnosis(w io.Writer, diagnosis runDiagnosis) error {
	if err := renderFields(w, runFields(diagnosis.Run)); err != nil {
		return err
	}
	if diagnosis.TaskInstancesError != "" {
		_, err := fmt.Fprintf(w, "\ntasks: could not be read (%s)\n", diagnosis.TaskInstancesError)
		return err
	}
	summary := diagnosis.Summary
	counts := make([]string, 0, len(summary.StateCounts))
	for _, state := range slices.Sorted(maps.Keys(summary.StateCounts)) {
		counts = append(counts, fmt.Sprintf("%s=%d", state, summary.StateCounts[state]))
	}
	line := fmt.Sprintf("\ntasks: %d", summary.TotalTasks)
	if len(counts) > 0 {
		line += " (" + strings.Join(counts, " ") + ")"
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}
	if summary.TotalTasks == 0 {
		return nil
	}
	return q.renderFailedTasks(w, diagnosis.Run.DAGID, diagnosis.Run.RunID, summary.FailedTasks)
}
