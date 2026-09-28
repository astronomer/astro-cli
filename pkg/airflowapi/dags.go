package airflowapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DAG is a DAG as both generations describe it. Where they disagree, both
// spellings are fields and the one this instance does not send stays at its
// zero value, so a renderer checks the client's generation rather than
// guessing from emptiness.
type DAG struct {
	DAGID          string   `json:"dag_id"`
	DAGDisplayName string   `json:"dag_display_name,omitempty"`
	Description    string   `json:"description,omitempty"`
	IsPaused       bool     `json:"is_paused"`
	Owners         []string `json:"owners,omitempty"`
	Tags           []DAGTag `json:"tags,omitempty"`
	FileLocation   string   `json:"fileloc,omitempty"`
	// FileToken is the DAG file's handle. Both generations send it, but only
	// Airflow 2 addresses DAG source by it; see GetDAGSource.
	FileToken string `json:"file_token,omitempty"`
	// IsActive is Airflow 2's "the scheduler still sees this file".
	// Airflow 3 spells the opposite as IsStale.
	IsActive bool `json:"is_active,omitempty"`
	IsStale  bool `json:"is_stale,omitempty"`
	// TimetableSummary is Airflow 3's schedule in one line. Airflow 2 sends
	// TimetableDescription instead, and a schedule_interval object this type
	// does not carry.
	TimetableSummary     string `json:"timetable_summary,omitempty"`
	TimetableDescription string `json:"timetable_description,omitempty"`
	MaxActiveRuns        int    `json:"max_active_runs,omitempty"`
	MaxActiveTasks       int    `json:"max_active_tasks,omitempty"`
	HasImportErrors      bool   `json:"has_import_errors,omitempty"`
	// NextDAGRun is Airflow 2's next scheduled logical date. Airflow 3
	// replaced it with the next_dagrun_* set, of which the logical date is
	// the one a listing shows.
	NextDAGRun            time.Time `json:"next_dagrun,omitzero"`
	NextDAGRunLogicalDate time.Time `json:"next_dagrun_logical_date,omitzero"`
	// The window the next run will cover; both generations agree on these.
	NextDAGRunDataIntervalStart time.Time `json:"next_dagrun_data_interval_start,omitzero"`
	NextDAGRunDataIntervalEnd   time.Time `json:"next_dagrun_data_interval_end,omitzero"`
	LastParsedTime              time.Time `json:"last_parsed_time,omitzero"`
}

// DAGTag is one tag on a DAG.
type DAGTag struct {
	Name string `json:"name"`
}

// DAGList is a page of DAGs.
type DAGList struct {
	DAGs         []DAG `json:"dags"`
	TotalEntries int   `json:"total_entries"`
}

// ListDAGsOptions filters a DAG listing. Only filters both generations
// accept are here; anything else goes through Do.
type ListDAGsOptions struct {
	ListOptions
	// Tags keeps only DAGs carrying one of these tags.
	Tags []string
	// DAGIDPattern keeps only DAGs whose id contains it.
	DAGIDPattern string
	// Paused keeps only paused or only unpaused DAGs; nil means both.
	Paused *bool
	// OnlyActive keeps only DAGs whose file the scheduler still sees (true),
	// or includes the ones it no longer does (false). Nil sends nothing, and
	// both generations then default to active DAGs only. Airflow 2 spells the
	// filter only_active and Airflow 3 spells its opposite exclude_stale; the
	// meaning is the same, so the value passes straight through.
	OnlyActive *bool
}

// ListDAGs lists DAGs.
func (c *Client) ListDAGs(ctx context.Context, opts ListDAGsOptions) (DAGList, error) {
	query := opts.query()
	for _, tag := range opts.Tags {
		query.Add("tags", tag)
	}
	if opts.DAGIDPattern != "" {
		query.Set("dag_id_pattern", opts.DAGIDPattern)
	}
	if opts.Paused != nil {
		query.Set("paused", strconv.FormatBool(*opts.Paused))
	}
	if opts.OnlyActive != nil {
		generation, err := c.Generation(ctx)
		if err != nil {
			return DAGList{}, err
		}
		name := "only_active"
		if generation == Airflow3 {
			name = "exclude_stale"
		}
		query.Set(name, strconv.FormatBool(*opts.OnlyActive))
	}
	var list DAGList
	err := c.getCollection(ctx, "/dags", query, &list)
	return list, err
}

// GetDAG reads one DAG.
func (c *Client) GetDAG(ctx context.Context, dagID string) (DAG, error) {
	var dag DAG
	err := c.get(ctx, pathf("/dags/%s", dagID), nil, &dag)
	return dag, err
}

// DAGSource is a DAG file's contents.
type DAGSource struct {
	DAGID   string `json:"dag_id"`
	Content string `json:"content"`
}

// GetDAGSource reads a DAG's source. Airflow 3 addresses it by dag id;
// Airflow 2 addresses it by the file token on the DAG object, so there it
// costs one extra call.
func (c *Client) GetDAGSource(ctx context.Context, dagID string) (DAGSource, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return DAGSource{}, err
	}
	handle := dagID
	if generation == Airflow2 {
		dag, err := c.GetDAG(ctx, dagID)
		if err != nil {
			return DAGSource{}, err
		}
		if dag.FileToken == "" {
			return DAGSource{}, fmt.Errorf("dag %s has no file token, so airflow 2 cannot serve its source", dagID)
		}
		handle = dag.FileToken
	}
	// Airflow 2 answers with the content alone, so the id comes from the
	// caller rather than the body.
	source := DAGSource{DAGID: dagID}
	if err := c.get(ctx, pathf("/dagSources/%s", handle), nil, &source); err != nil {
		return DAGSource{}, err
	}
	return source, nil
}

// DAGStats is the run count per state, per DAG.
type DAGStats struct {
	DAGs         []DAGStat `json:"dags"`
	TotalEntries int       `json:"total_entries"`
}

// DAGStat is one DAG's run counts.
type DAGStat struct {
	DAGID string          `json:"dag_id"`
	Stats []DAGStateCount `json:"stats"`
}

// DAGStateCount is how many runs of a DAG sit in one state.
type DAGStateCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

// dagStatsListLimit bounds the DAG listing Airflow 2 needs before it will
// report stats at all.
const dagStatsListLimit = 1000

// DAGStats reports run counts by state. Both generations need help here, and
// they need opposite help.
//
// Airflow 2 declares dag_ids a required string, so it refuses without one: an
// empty list means listing the DAGs first and joining their ids.
//
// Airflow 3 declares dag_ids an optional array, so "all DAGs" is the parameter
// left off. Sending it empty is not the same request — it asks for the one DAG
// whose id is the empty string, and the answer is always nothing. Some builds
// also answer 500 when several ids arrive together, so ids are asked for one at
// a time.
func (c *Client) DAGStats(ctx context.Context, dagIDs []string) (DAGStats, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return DAGStats{}, err
	}

	if generation == Airflow2 {
		wanted := dagIDs
		if len(wanted) == 0 {
			list, err := c.ListDAGs(ctx, ListDAGsOptions{ListOptions: ListOptions{Limit: dagStatsListLimit}})
			if err != nil {
				return DAGStats{}, err
			}
			for i := range list.DAGs {
				wanted = append(wanted, list.DAGs[i].DAGID)
			}
			if len(wanted) == 0 {
				return DAGStats{}, nil
			}
		}
		return c.dagStats(ctx, strings.Join(wanted, ","))
	}

	if len(dagIDs) == 0 {
		return c.dagStats(ctx, "")
	}
	var all DAGStats
	for _, dagID := range dagIDs {
		one, err := c.dagStats(ctx, dagID)
		if err != nil {
			return DAGStats{}, err
		}
		all.DAGs = append(all.DAGs, one.DAGs...)
		all.TotalEntries += one.TotalEntries
	}
	return all, nil
}

// dagStats asks for one dag_ids filter, which is the only parameter the
// endpoint takes in either generation. An empty filter is sent as no parameter
// at all: only Airflow 3 asks with one, and there an empty dag_ids selects
// nothing rather than everything.
func (c *Client) dagStats(ctx context.Context, dagIDs string) (DAGStats, error) {
	var stats DAGStats
	query := url.Values{}
	if dagIDs != "" {
		query.Set("dag_ids", dagIDs)
	}
	err := c.getCollection(ctx, "/dagStats", query, &stats)
	return stats, err
}

// Task is a task's definition inside a DAG — what the DAG says, not what a
// run did. TaskInstance is the execution.
type Task struct {
	TaskID          string   `json:"task_id"`
	TaskDisplayName string   `json:"task_display_name,omitempty"`
	Operator        string   `json:"operator_name,omitempty"`
	Owner           string   `json:"owner,omitempty"`
	Pool            string   `json:"pool,omitempty"`
	Queue           string   `json:"queue,omitempty"`
	Retries         float64  `json:"retries,omitempty"`
	DownstreamTasks []string `json:"downstream_task_ids,omitempty"`
	IsMapped        bool     `json:"is_mapped,omitempty"`
	DependsOnPast   bool     `json:"depends_on_past,omitempty"`
	TriggerRule     string   `json:"trigger_rule,omitempty"`
	// StartDate and EndDate bound when the task may run at all.
	StartDate time.Time `json:"start_date,omitzero"`
	EndDate   time.Time `json:"end_date,omitzero"`
}

// TaskList is a DAG's tasks.
type TaskList struct {
	Tasks        []Task `json:"tasks"`
	TotalEntries int    `json:"total_entries"`
}

// ListTasks lists a DAG's task definitions.
func (c *Client) ListTasks(ctx context.Context, dagID string) (TaskList, error) {
	var list TaskList
	err := c.get(ctx, pathf("/dags/%s/tasks", dagID), nil, &list)
	return list, err
}

// GetTask reads one task's definition.
func (c *Client) GetTask(ctx context.Context, dagID, taskID string) (Task, error) {
	var task Task
	err := c.get(ctx, pathf("/dags/%s/tasks/%s", dagID, taskID), nil, &task)
	return task, err
}

// ImportError is a DAG file the scheduler could not parse.
type ImportError struct {
	ImportErrorID int       `json:"import_error_id"`
	Filename      string    `json:"filename"`
	StackTrace    string    `json:"stack_trace"`
	Timestamp     time.Time `json:"timestamp,omitzero"`
	// Bundle names the dag bundle the file came from. Airflow 3 only.
	Bundle string `json:"bundle_name,omitempty"`
}

// ImportErrorList is a page of import errors.
type ImportErrorList struct {
	ImportErrors []ImportError `json:"import_errors"`
	TotalEntries int           `json:"total_entries"`
}

// ListImportErrors lists the DAG files that failed to parse — the first
// question to ask when a DAG is missing.
func (c *Client) ListImportErrors(ctx context.Context, opts ListOptions) (ImportErrorList, error) {
	var list ImportErrorList
	err := c.getCollection(ctx, "/importErrors", opts.query(), &list)
	return list, err
}

// DAGWarning is something the scheduler noticed about a DAG that did not stop
// it from parsing.
type DAGWarning struct {
	DAGID       string    `json:"dag_id"`
	WarningType string    `json:"warning_type"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp,omitzero"`
}

// DAGWarningList is a page of DAG warnings.
type DAGWarningList struct {
	DAGWarnings  []DAGWarning `json:"dag_warnings"`
	TotalEntries int          `json:"total_entries"`
}

// ListDAGWarnings lists the scheduler's warnings about DAGs.
func (c *Client) ListDAGWarnings(ctx context.Context, opts ListOptions) (DAGWarningList, error) {
	var list DAGWarningList
	err := c.getCollection(ctx, "/dagWarnings", opts.query(), &list)
	return list, err
}

// PauseDAG stops the scheduler from creating new runs of a DAG.
func (c *Client) PauseDAG(ctx context.Context, dagID string) (DAG, error) {
	return c.setPaused(ctx, dagID, true)
}

// UnpauseDAG lets the scheduler create runs of a DAG again.
func (c *Client) UnpauseDAG(ctx context.Context, dagID string) (DAG, error) {
	return c.setPaused(ctx, dagID, false)
}

func (c *Client) setPaused(ctx context.Context, dagID string, paused bool) (DAG, error) {
	var dag DAG
	err := c.do(ctx, Request{
		Method: http.MethodPatch,
		Path:   pathf("/dags/%s", dagID),
		Body:   map[string]any{"is_paused": paused},
	}, &dag)
	return dag, err
}
