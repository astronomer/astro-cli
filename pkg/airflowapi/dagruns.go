package airflowapi

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// anyID is the wildcard both generations accept where a dag or run id goes.
const anyID = "~"

// orAnyID widens an empty id to the wildcard.
func orAnyID(id string) string {
	if id == "" {
		return anyID
	}
	return id
}

// DAGRun is one run of a DAG. Absent dates arrive as null and stay zero.
type DAGRun struct {
	DAGRunID          string         `json:"dag_run_id"`
	DAGID             string         `json:"dag_id"`
	State             string         `json:"state"`
	RunType           string         `json:"run_type"`
	LogicalDate       time.Time      `json:"logical_date"`
	QueuedAt          time.Time      `json:"queued_at"`
	StartDate         time.Time      `json:"start_date"`
	EndDate           time.Time      `json:"end_date"`
	DataIntervalStart time.Time      `json:"data_interval_start"`
	DataIntervalEnd   time.Time      `json:"data_interval_end"`
	Conf              map[string]any `json:"conf"`
	Note              string         `json:"note"`
	// RunAfter is Airflow 3's earliest time the run may start.
	RunAfter time.Time `json:"run_after"`
	// TriggeredBy is Airflow 3's origin of the run ("cli", "rest_api", ...).
	// Airflow 2 says only whether it was triggered from outside the
	// schedule, in ExternalTrigger. Neither is set on the other generation,
	// so a renderer picks by the client's generation.
	TriggeredBy     string `json:"triggered_by,omitempty"`
	ExternalTrigger bool   `json:"external_trigger,omitempty"`
}

// DAGRunList is a page of runs.
type DAGRunList struct {
	DAGRuns      []DAGRun `json:"dag_runs"`
	TotalEntries int      `json:"total_entries"`
}

// ListDAGRunsOptions filters a run listing.
type ListDAGRunsOptions struct {
	ListOptions
	// States keeps only runs in one of these states.
	States []string
	// StartDateFrom and StartDateTo bound when the runs started. Either may
	// stand alone, so one end can be left open.
	//
	// Unlike the run's own date — which Airflow 2 calls execution_date and
	// Airflow 3 calls logical_date — both generations spell this filter
	// start_date_gte/start_date_lte, so there is nothing to adapt here.
	StartDateFrom time.Time
	StartDateTo   time.Time
}

// ListDAGRuns lists a DAG's runs. An empty dagID lists runs of every DAG,
// which both generations spell as a "~" in the dag id's place.
func (c *Client) ListDAGRuns(ctx context.Context, dagID string, opts ListDAGRunsOptions) (DAGRunList, error) {
	query := opts.query()
	for _, state := range opts.States {
		query.Add("state", state)
	}
	setTime(query, "start_date_gte", opts.StartDateFrom)
	setTime(query, "start_date_lte", opts.StartDateTo)
	var list DAGRunList
	err := c.getCollection(ctx, pathf("/dags/%s/dagRuns", orAnyID(dagID)), query, &list)
	return list, err
}

// setTime adds a timestamp filter, leaving it out when the time is unset. Both
// generations read these as RFC 3339.
func setTime(query url.Values, name string, at time.Time) {
	if at.IsZero() {
		return
	}
	query.Set(name, at.UTC().Format(time.RFC3339))
}

// GetDAGRun reads one run.
func (c *Client) GetDAGRun(ctx context.Context, dagID, runID string) (DAGRun, error) {
	var run DAGRun
	err := c.get(ctx, pathf("/dags/%s/dagRuns/%s", dagID, runID), nil, &run)
	return run, err
}

// TriggerDAGRunOptions is what a new run may carry. Every field is optional;
// Airflow names the run and dates it itself when they are left out.
type TriggerDAGRunOptions struct {
	// DAGRunID names the run instead of letting Airflow generate one.
	DAGRunID string
	// LogicalDate is the run's date. Airflow 2 calls it execution_date on
	// the way in, Airflow 3 calls it logical_date; this fills in whichever
	// the server expects.
	LogicalDate time.Time
	// Conf is the run configuration the DAG reads.
	Conf map[string]any
	// Note is a free-text note on the run. Airflow 2.10 and up, and
	// Airflow 3.
	Note string
}

// TriggerDAGRun starts a run of a DAG.
func (c *Client) TriggerDAGRun(ctx context.Context, dagID string, opts TriggerDAGRunOptions) (DAGRun, error) {
	version, err := c.Generation(ctx)
	if err != nil {
		return DAGRun{}, err
	}

	body := map[string]any{}
	switch version {
	case Airflow3:
		// Airflow 3 wants the key present even with no date, and reads a
		// null as "date this run for now".
		if opts.LogicalDate.IsZero() {
			body["logical_date"] = nil
		} else {
			body["logical_date"] = opts.LogicalDate
		}
	case Airflow2, GenerationNone:
		if !opts.LogicalDate.IsZero() {
			body["execution_date"] = opts.LogicalDate
		}
	}
	if opts.DAGRunID != "" {
		body["dag_run_id"] = opts.DAGRunID
	}
	if opts.Conf != nil {
		body["conf"] = opts.Conf
	}
	if opts.Note != "" {
		body["note"] = opts.Note
	}

	var run DAGRun
	err = c.do(ctx, Request{
		Method: http.MethodPost,
		Path:   pathf("/dags/%s/dagRuns", dagID),
		Body:   body,
	}, &run)
	return run, err
}

// DeleteDAGRun removes a run and its task instances.
func (c *Client) DeleteDAGRun(ctx context.Context, dagID, runID string) error {
	return c.do(ctx, Request{
		Method: http.MethodDelete,
		Path:   pathf("/dags/%s/dagRuns/%s", dagID, runID),
	}, nil)
}

// ClearDAGRunResult is what a clear answers with. Both generations switch
// shape on the dry run: a dry run lists the task instances it would clear, a
// real one returns the updated run, so exactly one field is set.
type ClearDAGRunResult struct {
	TaskInstances []TaskInstance
	TotalEntries  int
	DAGRun        *DAGRun
}

// ClearDAGRunOptions says how to clear a run.
type ClearDAGRunOptions struct {
	// DryRun reports what would be cleared and changes nothing.
	DryRun bool
}

// ClearDAGRun resets a run's task instances so the scheduler runs them
// again.
func (c *Client) ClearDAGRun(ctx context.Context, dagID, runID string, opts ClearDAGRunOptions) (ClearDAGRunResult, error) {
	resp, err := c.call(ctx, Request{
		Method: http.MethodPost,
		Path:   pathf("/dags/%s/dagRuns/%s/clear", dagID, runID),
		Body:   map[string]any{"dry_run": opts.DryRun},
	})
	if err != nil {
		return ClearDAGRunResult{}, err
	}

	var answer struct {
		TaskInstances []TaskInstance `json:"task_instances"`
		TotalEntries  int            `json:"total_entries"`
		DAGRunID      string         `json:"dag_run_id"`
	}
	if err := resp.Decode(&answer); err != nil {
		return ClearDAGRunResult{}, err
	}
	if answer.DAGRunID == "" {
		return ClearDAGRunResult{TaskInstances: answer.TaskInstances, TotalEntries: answer.TotalEntries}, nil
	}
	var run DAGRun
	if err := resp.Decode(&run); err != nil {
		return ClearDAGRunResult{}, err
	}
	return ClearDAGRunResult{DAGRun: &run}, nil
}
