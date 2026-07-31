package local

import (
	"context"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// NewAssetsCmd builds `astro assets` for the root.
func NewAssetsCmd(d Deps) *cobra.Command {
	return newQueryCmd(d, &cobra.Command{
		Use:   "assets",
		Short: "List the data assets an Airflow tracks, and their updates",
		Long: "Read the data assets on whichever Airflow this project resolves to, and the events that update " +
			"them. Airflow 2 calls the same thing a dataset; both answer here.\n\n" +
			"Which Airflow depends on -i/--instance, " + instanceEnvSentence,
	},
		newAssetsListCmd,
		newAssetsEventsCmd,
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the assets this Airflow tracks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runAssetsList(cmd.Context(), list.options())
		},
	}
	addListFlags(cmd, &list, "")
	return cmd
}

func (q *query) runAssetsList(ctx context.Context, opts airflowapi.ListOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	list, err := client.ListAssets(ctx, opts)
	if err != nil {
		return notServed("assets", err)
	}
	return emitRows(r, mapRows(list.Assets, newAssetRow), renderAssetTable)
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
	// Most recent first, for the same reason `astro runs list` sorts that way:
	// the question behind an event listing is nearly always "what just changed".
	addListFlags(cmd, &list, "-timestamp")
	cmd.Flags().StringVarP(&source.dagID, "dag-id", "d", "", "Only events produced by this DAG")
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
	list, err := client.ListAssetEvents(ctx, opts)
	if err != nil {
		return notServed("asset events", err)
	}
	return emitRows(r, mapRows(list.AssetEvents, newAssetEventRow), renderAssetEventTable)
}

func renderAssetEventTable(w io.Writer, rows []assetEventRow) error {
	return renderTable(w, rows, "No asset events on this Airflow.",
		[]string{"TIMESTAMP", "URI", "SOURCE", "STARTED"},
		func(row assetEventRow) []string {
			source := row.SourceDAGID
			if row.SourceTaskID != "" {
				source += "." + row.SourceTaskID
			}
			return []string{row.Timestamp, row.URI, source, strings.Join(row.CreatedRuns, ",")}
		})
}
