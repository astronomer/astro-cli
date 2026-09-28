package airflowapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// UnmarshalJSON reads a task instance with no map_index as unmapped (-1)
// rather than as expansion 0. Airflow 2 answers a task clear with references
// that carry no map_index at all.
func (t *TaskInstance) UnmarshalJSON(data []byte) error {
	type plain TaskInstance
	p := plain{MapIndex: -1}
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*t = TaskInstance(p)
	return nil
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

// GetMappedTaskInstance reads one expansion of a mapped task. GetTaskInstance
// cannot: a mapped task has no single instance for Airflow to return.
func (c *Client) GetMappedTaskInstance(ctx context.Context, dagID, runID, taskID string, mapIndex int) (TaskInstance, error) {
	var instance TaskInstance
	path := pathf("/dags/%s/dagRuns/%s/taskInstances/%s", dagID, runID, taskID) + "/" + strconv.Itoa(mapIndex)
	err := c.get(ctx, path, nil, &instance)
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
// 3's entries as their event text, with a traceback under any entry that
// carries one. An entry with no event text renders as its raw JSON, so nothing
// is silently dropped.
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
		// Held raw and decoded separately below, so an error_detail in a shape
		// this does not know costs the traceback and nothing else. Typed here,
		// it would fail the whole entry and dump the raw JSON in place of a line
		// that used to read fine.
		Error json.RawMessage `json:"error_detail"`
	}
	if err := json.Unmarshal(entry, &structured); err == nil && structured.Event != "" {
		if trace := tracebackOf(structured.Error); trace != "" {
			return structured.Event + "\n" + trace
		}
		return structured.Event
	}
	return string(entry)
}

// exceptionV3 is one exception in an Airflow 3 log entry's error_detail. The
// entry carries "Task failed with exception" as its event and everything a
// reader actually needs — the type, the value, the frames — beside it, so an
// entry rendered by its event alone says a task failed and never why.
type exceptionV3 struct {
	Type    string    `json:"exc_type"`
	Value   string    `json:"exc_value"`
	Notes   []string  `json:"exc_notes"`
	IsCause bool      `json:"is_cause"`
	Frames  []frameV3 `json:"frames"`
}

// frameV3 is one stack frame of an exceptionV3.
type frameV3 struct {
	Filename string `json:"filename"`
	Lineno   int    `json:"lineno"`
	Name     string `json:"name"`
}

// tracebackOf renders an entry's error_detail, or "" when there is nothing to
// render — absent, empty, or a shape this does not recognize. Every one of the
// three means the same thing to a caller: print the event line alone, as every
// ordinary log line already does.
func tracebackOf(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var chain []exceptionV3
	if err := json.Unmarshal(raw, &chain); err != nil || len(chain) == 0 {
		return ""
	}
	return traceback(chain)
}

// traceback renders error_detail the way Python prints it, so a reader
// recognizes the shape without being told.
//
// Airflow serializes a chain newest first: the exception that actually killed
// the task, then whatever it was raised from or during. Python prints the
// reverse — the oldest first, the raised one last, which is what "most recent
// call last" means about the chain as well as about the frames — so this walks
// the list backwards.
func traceback(chain []exceptionV3) string {
	var b strings.Builder
	for i := len(chain) - 1; i >= 0; i-- {
		exc := chain[i]
		if i < len(chain)-1 {
			// The exception just printed is older than this one. How the two
			// are joined is a fact about the older one: `raise X from Y` makes
			// Y a direct cause, and a bare raise inside an except block leaves
			// it as context.
			b.WriteString("\n")
			if chain[i+1].IsCause {
				b.WriteString("\nThe above exception was the direct cause of the following exception:\n\n")
			} else {
				b.WriteString("\nDuring handling of the above exception, another exception occurred:\n\n")
			}
		}
		b.WriteString("Traceback (most recent call last):")
		for _, f := range exc.Frames {
			fmt.Fprintf(&b, "\n  File %q, line %d, in %s", f.Filename, f.Lineno, f.Name)
		}
		b.WriteString("\n" + exc.Type)
		if exc.Value != "" {
			b.WriteString(": " + exc.Value)
		}
		for _, note := range exc.Notes {
			b.WriteString("\n" + note)
		}
	}
	return b.String()
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
