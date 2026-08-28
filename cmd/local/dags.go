package local

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// newDagsCmd builds the `dags` family over whichever Airflow the target names.
func newDagsCmd(d Deps, t target) *cobra.Command {
	return newQueryCmd(d, t, &cobra.Command{
		Use:   "dags",
		Short: "List and control the DAGs on an Airflow",
		Long: "Read and control the DAGs on " + t.which() + ": list them, read one, " +
			"print its source, count its runs by state, and pause or unpause it.",
	},
		newDagsListCmd,
		newDagsGetCmd,
		newDagsSourceCmd,
		newDagsStatsCmd,
		newDagsPauseCmd,
		newDagsUnpauseCmd,
	)
}

// dagRow is a DAG as this surface reports it: one shape for the listing and the
// detail, so `astro af dags list -o json` and `astro af dags get -o json` never
// disagree about what a DAG's fields are called. The two API generations
// disagree about the schedule and the next run, and both are folded here into
// one field, because which spelling arrived is the client's business, not a
// reader's.
type dagRow struct {
	DAGID           string   `json:"dag_id"`
	DisplayName     string   `json:"dag_display_name,omitempty"`
	IsPaused        bool     `json:"is_paused"`
	Schedule        string   `json:"schedule,omitempty"`
	Owners          []string `json:"owners,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Description     string   `json:"description,omitempty"`
	FileLocation    string   `json:"fileloc,omitempty"`
	NextRun         string   `json:"next_run,omitempty"`
	LastParsed      string   `json:"last_parsed_time,omitempty"`
	MaxActiveRuns   int      `json:"max_active_runs,omitempty"`
	MaxActiveTasks  int      `json:"max_active_tasks,omitempty"`
	HasImportErrors bool     `json:"has_import_errors"`
}

func newDAGRow(d airflowapi.DAG) dagRow {
	row := dagRow{
		DAGID:           d.DAGID,
		DisplayName:     d.DAGDisplayName,
		IsPaused:        d.IsPaused,
		Schedule:        firstNonEmpty(d.TimetableSummary, d.TimetableDescription),
		Owners:          d.Owners,
		Description:     d.Description,
		FileLocation:    d.FileLocation,
		LastParsed:      stamp(d.LastParsedTime),
		MaxActiveRuns:   d.MaxActiveRuns,
		MaxActiveTasks:  d.MaxActiveTasks,
		HasImportErrors: d.HasImportErrors,
	}
	// Airflow 3 sends the next run's logical date, Airflow 2 the run itself;
	// whichever arrived is the one to show.
	row.NextRun = stamp(d.NextDAGRunLogicalDate)
	if row.NextRun == "" {
		row.NextRun = stamp(d.NextDAGRun)
	}
	for _, tag := range d.Tags {
		row.Tags = append(row.Tags, tag.Name)
	}
	return row
}

func newDagsListCmd(q *query) *cobra.Command {
	var list listFlags
	var opts struct {
		tags      []string
		pattern   string
		paused    bool
		notPaused bool
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the DAGs on this Airflow",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o := airflowapi.ListDAGsOptions{
				ListOptions:  list.options(),
				Tags:         opts.tags,
				DAGIDPattern: opts.pattern,
			}
			// The filter is three-valued — paused, active, or both — so it is
			// two flags rather than one boolean, and neither of them means
			// "show me everything" by omission alone.
			switch {
			case opts.paused:
				yes := true
				o.Paused = &yes
			case opts.notPaused:
				no := false
				o.Paused = &no
			}
			return q.runDagsList(cmd.Context(), o)
		},
	}
	addListFlags(cmd, &list, "")
	cmd.Flags().StringSliceVarP(&opts.tags, "tags", "t", nil, "Keep only DAGs carrying one of these tags (repeatable)")
	cmd.Flags().StringVar(&opts.pattern, "dag-id-pattern", "", "Keep only DAGs whose id contains this")
	cmd.Flags().BoolVar(&opts.paused, "paused", false, "Keep only paused DAGs")
	cmd.Flags().BoolVar(&opts.notPaused, "not-paused", false, "Keep only DAGs that are not paused")
	cmd.MarkFlagsMutuallyExclusive("paused", "not-paused")
	return cmd
}

func (q *query) runDagsList(ctx context.Context, opts airflowapi.ListDAGsOptions) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	list, err := client.ListDAGs(ctx, opts)
	if err != nil {
		return err
	}
	return emitRows(r, mapRows(list.DAGs, newDAGRow), renderDAGTable)
}

func renderDAGTable(w io.Writer, rows []dagRow) error {
	return renderTable(w, rows, "No DAGs on this Airflow.",
		[]string{"DAG_ID", "PAUSED", "SCHEDULE", "OWNERS", "TAGS", "NEXT_RUN"},
		func(row dagRow) []string {
			return []string{
				row.DAGID,
				yesNo(row.IsPaused),
				row.Schedule,
				strings.Join(row.Owners, ","),
				strings.Join(row.Tags, ","),
				row.NextRun,
			}
		})
}

func newDagsGetCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "get <DAG_ID>",
		Short: "Show one DAG's schedule, owners, tags, and file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runDagsGet(cmd.Context(), args[0])
		},
	}
}

func (q *query) runDagsGet(ctx context.Context, dagID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	dag, err := client.GetDAG(ctx, dagID)
	if err != nil {
		return err
	}
	return emitDetail(r, newDAGRow(dag), dagFields)
}

func dagFields(row dagRow) []field {
	return []field{
		{"dag id", row.DAGID},
		{"display name", row.DisplayName},
		{"paused", yesNo(row.IsPaused)},
		{"schedule", row.Schedule},
		{"description", row.Description},
		{"owners", strings.Join(row.Owners, ", ")},
		{"tags", strings.Join(row.Tags, ", ")},
		{"file", row.FileLocation},
		{"next run", row.NextRun},
		{"last parsed", row.LastParsed},
		{"max active runs", omitZero(row.MaxActiveRuns)},
		{"max active tasks", omitZero(row.MaxActiveTasks)},
		{"import errors", onlyIf(row.HasImportErrors, "yes")},
	}
}

func newDagsSourceCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "source <DAG_ID>",
		Short: "Print a DAG's source file",
		Long: "Print the Python source Airflow parsed for this DAG. It comes from that Airflow, not from your " +
			"checkout, so it is what is actually running there.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runDagsSource(cmd.Context(), args[0])
		},
	}
}

// dagSource is a DAG file as this surface reports it.
type dagSource struct {
	DAGID   string `json:"dag_id"`
	Content string `json:"content"`
}

func (q *query) runDagsSource(ctx context.Context, dagID string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	source, err := client.GetDAGSource(ctx, dagID)
	if err != nil {
		return err
	}
	// Text mode prints the file and nothing else: this is the one command whose
	// output someone pipes into a file or a diff.
	return r.Emit(dagSource{DAGID: dagID, Content: source.Content}, func(w io.Writer) error {
		return writeText(w, source.Content)
	})
}

func newDagsStatsCmd(q *query) *cobra.Command {
	var dagIDs []string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Count each DAG's runs by state",
		Long: "Count the runs of each DAG by state. With no --dag-id it covers every DAG, which on Airflow 2 means " +
			"listing the DAGs first because that generation refuses the endpoint without ids.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return q.runDagsStats(cmd.Context(), dagIDs)
		},
	}
	cmd.Flags().StringSliceVar(&dagIDs, "dag-id", nil, "Count only these DAGs (repeatable)")
	return cmd
}

// dagStatRow is one DAG's run counts. The states are a map rather than fixed
// fields because Airflow's set of run states is its own to change, and a
// reader asking for counts wants whatever this instance actually has.
type dagStatRow struct {
	DAGID string         `json:"dag_id"`
	Stats map[string]int `json:"stats"`
}

func (q *query) runDagsStats(ctx context.Context, dagIDs []string) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	stats, err := client.DAGStats(ctx, dagIDs)
	if err != nil {
		return notServed("DAG run statistics", err)
	}
	return emitRows(r, dagStatRows(stats), renderDAGStatsTable)
}

// dagStatRows turns the client's per-state counts into rows. `astro af dags stats`
// and the health report read the same endpoint, so they share the shape.
func dagStatRows(stats airflowapi.DAGStats) []dagStatRow {
	return mapRows(stats.DAGs, func(stat airflowapi.DAGStat) dagStatRow {
		row := dagStatRow{DAGID: stat.DAGID, Stats: map[string]int{}}
		for _, s := range stat.Stats {
			row.Stats[s.State] = s.Count
		}
		return row
	})
}

func renderDAGStatsTable(w io.Writer, rows []dagStatRow) error {
	// One column per state anything reported, so the table reads down a state
	// as well as across a DAG. Which states exist is what the instance said,
	// never a list compiled here.
	states := map[string]bool{}
	for _, row := range rows {
		for state := range row.Stats {
			states[state] = true
		}
	}
	columns := make([]string, 0, len(states))
	for state := range states {
		columns = append(columns, state)
	}
	sort.Strings(columns)
	headers := append([]string{"DAG_ID"}, upperAll(columns)...)
	return renderTable(w, rows, "No DAG run statistics on this Airflow.", headers,
		func(row dagStatRow) []string {
			cells := make([]string, 0, len(columns)+1)
			cells = append(cells, row.DAGID)
			for _, state := range columns {
				cells = append(cells, count(row.Stats[state]))
			}
			return cells
		})
}

func upperAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strings.ToUpper(v))
	}
	return out
}

func newDagsPauseCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <DAG_ID>",
		Short: "Stop the scheduler from creating new runs of a DAG",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runSetPaused(cmd.Context(), args[0], (*airflowapi.Client).PauseDAG)
		},
	}
}

func newDagsUnpauseCmd(q *query) *cobra.Command {
	return &cobra.Command{
		Use:   "unpause <DAG_ID>",
		Short: "Let the scheduler create runs of a DAG again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runSetPaused(cmd.Context(), args[0], (*airflowapi.Client).UnpauseDAG)
		},
	}
}

// runSetPaused backs both pause and unpause, which differ only in the call they
// make. It reports the state Airflow came back with rather than the one that
// was asked for, so a call that changed nothing reads as what it was.
func (q *query) runSetPaused(ctx context.Context, dagID string, set func(*airflowapi.Client, context.Context, string) (airflowapi.DAG, error)) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	dag, err := set(client, ctx, dagID)
	if err != nil {
		return err
	}
	row := newDAGRow(dag)
	return r.Emit(row, func(w io.Writer) error {
		verb := "unpaused"
		if row.IsPaused {
			verb = "paused"
		}
		_, werr := fmt.Fprintf(w, "%s %s\n", verb, row.DAGID)
		return werr
	})
}
