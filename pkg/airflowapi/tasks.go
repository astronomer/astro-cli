package airflowapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TaskInstance is one task's execution inside a run.
type TaskInstance struct {
	TaskID      string    `json:"task_id"`
	DAGID       string    `json:"dag_id"`
	DAGRunID    string    `json:"dag_run_id"`
	State       string    `json:"state"`
	MapIndex    int       `json:"map_index"`
	TryNumber   int       `json:"try_number"`
	MaxTries    int       `json:"max_tries"`
	Operator    string    `json:"operator"`
	Queue       string    `json:"queue"`
	Pool        string    `json:"pool"`
	Hostname    string    `json:"hostname"`
	Note        string    `json:"note"`
	Duration    float64   `json:"duration"`
	LogicalDate time.Time `json:"logical_date"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
}

// TaskInstanceList is a page of task instances.
type TaskInstanceList struct {
	TaskInstances []TaskInstance `json:"task_instances"`
	TotalEntries  int            `json:"total_entries"`
}

// ListTaskInstances lists a run's task instances. An empty dagID or runID
// widens the listing to every DAG or every run, the "~" wildcard.
func (c *Client) ListTaskInstances(ctx context.Context, dagID, runID string, opts ListOptions) (TaskInstanceList, error) {
	var list TaskInstanceList
	path := pathf("/dags/%s/dagRuns/%s/taskInstances", orAnyID(dagID), orAnyID(runID))
	err := c.getCollection(ctx, path, opts.query(), &list)
	return list, err
}

// GetTaskInstance reads one task instance.
func (c *Client) GetTaskInstance(ctx context.Context, dagID, runID, taskID string) (TaskInstance, error) {
	var instance TaskInstance
	err := c.get(ctx, pathf("/dags/%s/dagRuns/%s/taskInstances/%s", dagID, runID, taskID), nil, &instance)
	return instance, err
}

// TaskLogsOptions picks which try of a task instance to read.
type TaskLogsOptions struct {
	// TryNumber is the attempt, 1-indexed; zero reads the first try.
	TryNumber int
	// MapIndex selects one expansion of a mapped task. Nil, the default,
	// reads the unmapped instance — index 0 is a real expansion, so this
	// cannot be a plain int.
	MapIndex *int
	// Tail asks for the end of the log rather than all of it. The zero
	// value reads the whole log, which is what someone asking for logs
	// almost always wants.
	Tail bool
}

// TaskLog is one try's log, as the instance served it. Airflow 3 sends
// structured entries, kept here as they arrived. Airflow 2 is asked for
// text/plain and its log arrives as text, which is why Content can be empty on
// a log that has plenty to say — read it through Text.
type TaskLog struct {
	Content json.RawMessage `json:"content"`
	// ContinuationToken fetches the next chunk of a log still being
	// written. Airflow 2's JSON answer carries it; the text/plain answer this
	// client asks for does not, so it is empty in practice. Reading a growing
	// log would mean asking for JSON again and living with the repr.
	ContinuationToken string `json:"continuation_token,omitempty"`
	// text is Airflow 2's log exactly as it was served. It is unexported
	// because it is not a wire field: nothing decodes into it, and a value
	// built by hand from JSON still renders through Content below.
	text string
}

// Text renders the log as lines: Airflow 2's text as it arrived, and Airflow
// 3's entries as their event text. An entry with no event text renders as its
// raw JSON, so nothing is silently dropped.
func (l TaskLog) Text() string {
	if l.text != "" {
		return l.text
	}
	var whole string
	if err := json.Unmarshal(l.Content, &whole); err == nil {
		return whole
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(l.Content, &entries); err != nil {
		return string(l.Content)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, logLine(entry))
	}
	return strings.Join(lines, "\n")
}

func logLine(entry json.RawMessage) string {
	var text string
	if err := json.Unmarshal(entry, &text); err == nil {
		return text
	}
	var structured struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(entry, &structured); err == nil && structured.Event != "" {
		return structured.Event
	}
	return string(entry)
}

// TaskLogs reads one try of a task instance's log.
//
// Airflow 2 is asked for text/plain, which its log endpoint offers alongside
// JSON. The JSON answer wraps the log in the Python repr of its
// (hostname, log) pairs — one line holding every real line as an escape — and
// unpicking that means parsing a Python literal to recover text the server will
// hand over as text for the asking.
func (c *Client) TaskLogs(ctx context.Context, dagID, runID, taskID string, opts TaskLogsOptions) (TaskLog, error) {
	tryNumber := opts.TryNumber
	if tryNumber == 0 {
		tryNumber = 1
	}
	query := url.Values{"full_content": {strconv.FormatBool(!opts.Tail)}}
	if opts.MapIndex != nil {
		query.Set("map_index", strconv.Itoa(*opts.MapIndex))
	}
	path := pathf("/dags/%s/dagRuns/%s/taskInstances/%s", dagID, runID, taskID) + "/logs/" + strconv.Itoa(tryNumber)

	generation, err := c.Generation(ctx)
	if err != nil {
		return TaskLog{}, err
	}
	if generation == Airflow2 {
		resp, err := c.call(ctx, Request{
			Method: http.MethodGet,
			Path:   path,
			Query:  query,
			Header: http.Header{"Accept": {textMediaType}},
		})
		switch {
		case err == nil:
			return TaskLog{text: string(resp.Body)}, nil
		case !errors.Is(err, ErrHeadersUnsupported):
			return TaskLog{}, err
		}
		// A door that carries no headers cannot ask for text, so the log comes
		// back in the repr and the reader gets what they always got. MWAA under
		// InvokeRestApi is the case; asking again costs one request, and only on
		// that door.
	}

	var log TaskLog
	err = c.get(ctx, path, query, &log)
	return log, err
}

// ClearTaskInstancesOptions says what to clear and how far to spread. Every
// field is sent as given, so the zero value clears for real and leaves the
// run states alone.
type ClearTaskInstancesOptions struct {
	// DAGRunID limits the clear to one run.
	DAGRunID string
	// TaskIDs are the tasks to clear.
	TaskIDs []string
	// DryRun reports what would be cleared and changes nothing.
	DryRun bool
	// OnlyFailed skips task instances that did not fail.
	OnlyFailed bool
	// IncludeDownstream and IncludeUpstream widen the clear along the
	// dependency graph.
	IncludeDownstream bool
	IncludeUpstream   bool
	// ResetDAGRuns puts the affected runs back into the queued state, which
	// is what makes the scheduler pick the cleared tasks up.
	ResetDAGRuns bool
}

// ClearTaskInstances clears task instances so the scheduler runs them again.
func (c *Client) ClearTaskInstances(ctx context.Context, dagID string, opts ClearTaskInstancesOptions) (TaskInstanceList, error) {
	body := map[string]any{
		"dry_run":            opts.DryRun,
		"only_failed":        opts.OnlyFailed,
		"include_downstream": opts.IncludeDownstream,
		"include_upstream":   opts.IncludeUpstream,
		"reset_dag_runs":     opts.ResetDAGRuns,
	}
	if opts.DAGRunID != "" {
		body["dag_run_id"] = opts.DAGRunID
	}
	if len(opts.TaskIDs) > 0 {
		body["task_ids"] = opts.TaskIDs
	}

	var list TaskInstanceList
	err := c.do(ctx, Request{
		Method: http.MethodPost,
		Path:   pathf("/dags/%s/clearTaskInstances", dagID),
		Body:   body,
	}, &list)
	return list, err
}
