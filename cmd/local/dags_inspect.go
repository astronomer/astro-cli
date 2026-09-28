package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// The three `dags` verbs here answer "what is wrong with the DAGs on this
// Airflow, and what is this one": the files that failed to parse, what the
// scheduler warns about, and one DAG's definition, tasks, and source at once.
// The first two are sections of `health` too; these are the same rows on their
// own, for when that is the whole question.

func newDagsErrorsCmd(q *query) *cobra.Command {
	var list listFlags
	cmd := &cobra.Command{
		Use:   "errors",
		Short: "List the DAG files that failed to parse, with their tracebacks",
		Long: "List the import errors on this Airflow: every DAG file the DAG processor could not parse, and the " +
			"traceback it raised. A DAG missing from `dags list` is nearly always here.\n\nThe command succeeds " +
			"whether or not there are errors; an empty listing is the good news.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runDagsErrors(cmd.Context(), list.options())
		},
	}
	addListFlags(cmd, &list, "")
	return cmd
}

func (q *query) runDagsErrors(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return notServed("import errors", emitList(q, r, opts, func(page airflowapi.ListOptions) ([]airflowapi.ImportError, int, error) {
		list, err := client.ListImportErrors(ctx, page)
		return list.ImportErrors, list.TotalEntries, err
	}, newImportErrorRow, renderImportErrors))
}

// renderImportErrors writes each error as its file and then its whole
// traceback, indented under it. A table would have to cut the traceback to one
// line, and the frames are the part someone fixing the file reads.
func renderImportErrors(w io.Writer, rows []importErrorRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No import errors on this Airflow.")
		return err
	}
	for i, row := range rows {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		var about []string
		if row.Bundle != "" {
			about = append(about, "bundle "+row.Bundle)
		}
		if row.Timestamp != "" {
			about = append(about, row.Timestamp)
		}
		header := row.Filename
		if len(about) > 0 {
			header += " (" + strings.Join(about, ", ") + ")"
		}
		if _, err := fmt.Fprintln(w, header); err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimRight(row.StackTrace, "\n"), "\n") {
			if _, err := fmt.Fprintln(w, "    "+strings.TrimRight(line, "\r")); err != nil {
				return err
			}
		}
	}
	return nil
}

func newDagsWarningsCmd(q *query) *cobra.Command {
	var list listFlags
	cmd := &cobra.Command{
		Use:   "warnings",
		Short: "List what the scheduler warns about in DAGs that did parse",
		Long: "List the DAG warnings on this Airflow: problems the scheduler noticed in DAGs that parsed, such as " +
			"a pool that does not exist or a deprecated argument. Unlike import errors, these DAGs still run.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runDagsWarnings(cmd.Context(), list.options())
		},
	}
	addListFlags(cmd, &list, "")
	return cmd
}

func (q *query) runDagsWarnings(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return notServed("DAG warnings", emitList(q, r, opts, func(page airflowapi.ListOptions) ([]airflowapi.DAGWarning, int, error) {
		list, err := client.ListDAGWarnings(ctx, page)
		return list.DAGWarnings, list.TotalEntries, err
	}, newDAGWarningRow, renderDAGWarningTable))
}

func renderDAGWarningTable(w io.Writer, rows []dagWarningRow) error {
	return renderTable(w, rows, "No DAG warnings on this Airflow.",
		[]string{"DAG_ID", "TYPE", "TIMESTAMP", "MESSAGE"},
		func(row dagWarningRow) []string {
			return []string{row.DAGID, row.WarningType, row.Timestamp, firstLine(row.Message)}
		})
}

func newDagsExploreCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "explore <DAG_ID>",
		Short: "Show one DAG's definition, its tasks, and its source together",
		Long: "Read everything there is to know about one DAG in one go: what `dags get` shows, the tasks it " +
			"defines as `tasks list` shows them, and the source Airflow parsed.\n\nEach part is read on its own, " +
			"and one that fails is reported in its place rather than ending the command. It fails only when none " +
			"of the three could be read, which is what a DAG id Airflow does not know looks like.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runDagsExplore(cmd.Context(), args[0])
		},
	}
}

// dagExploration is what `dags explore` reports: the three reads, each beside
// the reason it could not be made. A part that failed is absent and its error
// is set, so a consumer tells "no tasks" from "could not list the tasks" by the
// error, never by an empty list.
type dagExploration struct {
	DAGID string `json:"dag_id"`
	// DAG is what `dags get` reports.
	DAG      *dagRow `json:"dag,omitempty"`
	DAGError string  `json:"dag_error,omitempty"`
	// Tasks is what `tasks list` reports.
	Tasks      []taskRow `json:"tasks,omitempty"`
	TasksError string    `json:"tasks_error,omitempty"`
	// Source is the file Airflow parsed, as `dags source` prints it.
	Source      string `json:"source,omitempty"`
	SourceError string `json:"source_error,omitempty"`
}

func (q *query) runDagsExplore(ctx context.Context, dagID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	// The reads share nothing, so they go at once, as health's do: one slow
	// endpoint should not hold the other two behind it.
	result := dagExploration{DAGID: dagID}
	var dagErr, tasksErr, sourceErr error
	var wg sync.WaitGroup
	wg.Add(3) //nolint:mnd // the three reads below
	go func() {
		defer wg.Done()
		dag, err := client.GetDAG(ctx, dagID)
		if err != nil {
			dagErr = err
			return
		}
		row := newDAGRow(dag)
		result.DAG = &row
	}()
	go func() {
		defer wg.Done()
		list, err := client.ListTasks(ctx, dagID)
		if err != nil {
			tasksErr = err
			return
		}
		result.Tasks = mapRows(list.Tasks, newTaskRow)
	}()
	go func() {
		defer wg.Done()
		source, err := client.GetDAGSource(ctx, dagID)
		if err != nil {
			sourceErr = err
			return
		}
		result.Source = source.Content
	}()
	wg.Wait()

	if dagErr != nil && tasksErr != nil && sourceErr != nil {
		// Nothing answered, so there is nothing to render. The DAG's own read
		// is the one whose failure explains the others.
		return dagErr
	}
	result.DAGError = errorText(dagErr)
	result.TasksError = errorText(tasksErr)
	result.SourceError = errorText(sourceErr)
	return r.Emit(result, func(w io.Writer) error { return renderDAGExploration(w, result) })
}

// errorText is an error's message, or "" for none.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, airflowapi.ErrForbidden) {
		return sectionFailure(err)
	}
	return err.Error()
}

func renderDAGExploration(w io.Writer, result dagExploration) error {
	if result.DAG != nil {
		if err := renderFields(w, dagFields(*result.DAG)); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "dag %s: could not be read (%s)\n", result.DAGID, result.DAGError); err != nil {
		return err
	}

	if result.TasksError != "" {
		if _, err := fmt.Fprintf(w, "\ntasks: could not be read (%s)\n", result.TasksError); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "\ntasks (%d):\n", len(result.Tasks)); err != nil {
			return err
		}
		if err := renderTaskTable(w, result.Tasks); err != nil {
			return err
		}
	}

	if result.SourceError != "" {
		_, err := fmt.Fprintf(w, "\nsource: could not be read (%s)\n", result.SourceError)
		return err
	}
	label := "source:"
	if result.DAG != nil && result.DAG.FileLocation != "" {
		label = "source (" + result.DAG.FileLocation + "):"
	}
	if _, err := fmt.Fprintf(w, "\n%s\n", label); err != nil {
		return err
	}
	return writeText(w, result.Source)
}
