package local

import (
	"context"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// newAssetsCmd builds the `assets` family over whichever Airflow the target
// names.
func newAssetsCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "assets",
		Short: "List the data assets an Airflow tracks, and their updates",
		Long: "Read the data assets on " + t.which() + ", the events that update " +
			"them, and the events that started a given run. Airflow 2 calls the same thing a dataset; both " +
			"answer here.",
	},
		newAssetsListCmd,
		newAssetsEventsCmd,
		newAssetsTriggersCmd,
	)
}

// assetRow is a data asset as this surface reports it.
type assetRow struct {
	ID   int64  `json:"id"`
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
	// Group is Airflow 3 only, as is Name; Airflow 2 identifies a dataset by
	// its URI alone.
	Group          string   `json:"group,omitempty"`
	ProducingTasks []string `json:"producing_tasks,omitempty"`
	ScheduledDAGs  []string `json:"scheduled_dags,omitempty"`
	CreatedAt      string   `json:"created_at,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

func newAssetRow(a airflowapi.Asset) assetRow {
	row := assetRow{
		ID:        a.ID,
		URI:       a.URI,
		Name:      a.Name,
		Group:     a.Group,
		CreatedAt: stamp(a.CreatedAt),
		UpdatedAt: stamp(a.UpdatedAt),
	}
	for _, task := range a.ProducingTasks {
		row.ProducingTasks = append(row.ProducingTasks, task.DAGID+"."+task.TaskID)
	}
	for _, dag := range a.ScheduledDAGs {
		row.ScheduledDAGs = append(row.ScheduledDAGs, dag.DAGID)
	}
	return row
}

func newAssetsListCmd(q *query) *cobra.Command {
	var list listFlags
	var uriPattern string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the assets this Airflow tracks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runAssetsList(cmd.Context(), airflowapi.ListAssetsOptions{
				ListOptions: list.options(),
				URIPattern:  uriPattern,
			})
		},
	}
	addListFlags(cmd, &list, "")
	cmd.Flags().StringVar(&uriPattern, "uri-pattern", "",
		"Keep only assets whose URI contains this; % matches any run of characters")
	return cmd
}

func (q *query) runAssetsList(ctx context.Context, opts airflowapi.ListAssetsOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return notServed("assets", emitList(q, r, opts.ListOptions, func(page airflowapi.ListOptions) ([]airflowapi.Asset, int, error) {
		list, err := client.ListAssets(ctx, airflowapi.ListAssetsOptions{ListOptions: page, URIPattern: opts.URIPattern})
		return list.Assets, list.TotalEntries, err
	}, newAssetRow, renderAssetTable))
}

func renderAssetTable(w io.Writer, rows []assetRow) error {
	return renderTable(w, rows, "No assets on this Airflow.",
		[]string{"URI", "NAME", "PRODUCED_BY", "SCHEDULES", "UPDATED"},
		func(row assetRow) []string {
			return []string{
				row.URI,
				row.Name,
				strings.Join(row.ProducingTasks, ","),
				strings.Join(row.ScheduledDAGs, ","),
				row.UpdatedAt,
			}
		})
}

func newAssetsEventsCmd(q *query) *cobra.Command {
	var list listFlags
	var source struct {
		dagID  string
		runID  string
		taskID string
	}
	cmd := &cobra.Command{
		Use:   "events",
		Short: "List asset updates and the runs they started",
		Long: "List the events that updated an asset. An event is what a task wrote, and it is what starts the " +
			"DAGs scheduled on that asset — so this is the answer to why a data-aware DAG ran.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runAssetsEvents(cmd.Context(), airflowapi.ListAssetEventsOptions{
				ListOptions:  list.options(),
				SourceDAGID:  source.dagID,
				SourceRunID:  source.runID,
				SourceTaskID: source.taskID,
			})
		},
	}
	// Most recent first, for the same reason `astro af runs list` sorts that way:
	// the question behind an event listing is nearly always "what just changed".
	addListFlags(cmd, &list, "-timestamp")
	cmd.Flags().StringVar(&source.dagID, "dag-id", "", "Only events produced by this DAG")
	cmd.Flags().StringVarP(&source.runID, "run-id", "r", "", "Only events produced by this run")
	cmd.Flags().StringVar(&source.taskID, "task-id", "", "Only events produced by this task")
	return cmd
}

// assetEventRow is one update of an asset: what produced it, and what it
// started.
type assetEventRow struct {
	ID           int64    `json:"id"`
	URI          string   `json:"uri,omitempty"`
	AssetID      int64    `json:"asset_id,omitempty"`
	SourceDAGID  string   `json:"source_dag_id,omitempty"`
	SourceTaskID string   `json:"source_task_id,omitempty"`
	SourceRunID  string   `json:"source_run_id,omitempty"`
	Timestamp    string   `json:"timestamp,omitempty"`
	CreatedRuns  []string `json:"created_dagruns,omitempty"`
}

func newAssetEventRow(e airflowapi.AssetEvent) assetEventRow {
	row := assetEventRow{
		ID:           e.ID,
		URI:          e.URI,
		AssetID:      e.AssetID,
		SourceDAGID:  e.SourceDAGID,
		SourceTaskID: e.SourceTaskID,
		SourceRunID:  e.SourceRunID,
		Timestamp:    stamp(e.Timestamp),
	}
	for _, run := range e.CreatedDAGs {
		row.CreatedRuns = append(row.CreatedRuns, run.DAGID+"/"+run.DAGRunID)
	}
	return row
}

func (q *query) runAssetsEvents(ctx context.Context, opts airflowapi.ListAssetEventsOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	return notServed("asset events", emitList(q, r, opts.ListOptions, func(page airflowapi.ListOptions) ([]airflowapi.AssetEvent, int, error) {
		filtered := opts
		filtered.ListOptions = page
		list, err := client.ListAssetEvents(ctx, filtered)
		return list.AssetEvents, list.TotalEntries, err
	}, newAssetEventRow, renderAssetEventTable))
}

func renderAssetEventTable(w io.Writer, rows []assetEventRow) error {
	return renderAssetEvents(w, rows, "No asset events on this Airflow.")
}

func newAssetsTriggersCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "triggers <DAG_ID> <RUN_ID>",
		Short: "List the asset updates that started a run",
		Long: "List the asset events a run was waiting on: the updates that made the scheduler start this run of " +
			"a data-aware DAG. A run the schedule or a person started lists none.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runAssetsTriggers(cmd.Context(), args[0], args[1])
		},
	}
}

func (q *query) runAssetsTriggers(ctx context.Context, dagID, runID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	list, err := client.UpstreamAssetEvents(ctx, dagID, runID)
	if err != nil {
		return notServed("the asset events behind a run", err)
	}
	return emitRows(r, mapRows(list.AssetEvents, newAssetEventRow), func(w io.Writer, rows []assetEventRow) error {
		return renderAssetEvents(w, rows, "No asset events started this run.")
	})
}

func renderAssetEvents(w io.Writer, rows []assetEventRow, empty string) error {
	return renderTable(w, rows, empty,
		[]string{"TIMESTAMP", "URI", "SOURCE", "STARTED"},
		func(row assetEventRow) []string {
			source := row.SourceDAGID
			if row.SourceTaskID != "" {
				source += "." + row.SourceTaskID
			}
			return []string{row.Timestamp, row.URI, source, strings.Join(row.CreatedRuns, ",")}
		})
}
