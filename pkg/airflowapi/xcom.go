package airflowapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// XComEntry is one value a task instance pushed to XCom.
type XComEntry struct {
	Key      string `json:"key"`
	TaskID   string `json:"task_id"`
	DAGID    string `json:"dag_id"`
	MapIndex int    `json:"map_index"`
	// RunID is Airflow 3's; Airflow 2 names the run by its execution date
	// instead and leaves this empty.
	RunID     string    `json:"run_id"`
	Timestamp time.Time `json:"timestamp"`
	// Value is kept as it arrived. Where Airflow deserializes it (see
	// GetXComEntry) it is the value the task returned as JSON, with anything
	// JSON cannot hold (a datetime, a custom class) rendered as a string.
	// Elsewhere it is the value as Airflow stores it: Airflow 3.0's JSON, in
	// which such a value is a serialization envelope (__classname__,
	// __data__), or Airflow 2's string.
	Value json.RawMessage `json:"value"`
}

// GetXComEntry reads the XCom value a task instance pushed under key. A
// negative mapIndex reads an unmapped task's value; a mapped task keeps one
// value per expansion, so it needs the index.
//
// The value is asked for deserialized first. Airflow 3.1 and later do that
// for anyone; Airflow 3.0, and Airflow 2.10 and later, only with
// [api] enable_xcom_deserialize_support on, and refuse otherwise; an Airflow
// 2 older than the parameters refuses them as unknown. Which one answered is
// learned from the answer, never from the version: a refusal is asked again
// for the stored value, at the cost of one more read where it was refused.
func (c *Client) GetXComEntry(ctx context.Context, dagID, runID, taskID, key string, mapIndex int) (XComEntry, error) {
	path := pathf("/dags/%s/dagRuns/%s/taskInstances/%s/xcomEntries/%s", dagID, runID, taskID, key)
	var entry XComEntry
	err := c.get(ctx, path, xcomQuery(mapIndex, true), &entry)
	if !deserializeRefused(err) {
		return entry, err
	}
	err = c.get(ctx, path, xcomQuery(mapIndex, false), &entry)
	return entry, err
}

// xcomQuery addresses one expansion when there is one and, when asked, has
// Airflow deserialize the value. Without that, Airflow 3 answers with its
// serialization envelopes (__classname__, __data__) rather than the value the
// task returned; stringify=false keeps the answer JSON rather than Python's
// rendering of it, on Airflow 2 as on 3.
func xcomQuery(mapIndex int, deserialize bool) url.Values {
	query := url.Values{}
	if mapIndex >= 0 {
		query.Set("map_index", strconv.Itoa(mapIndex))
	}
	if deserialize {
		query.Set("deserialize", "true")
		query.Set("stringify", "false")
	}
	return query
}

// deserializeRefused reports an Airflow that would not take the deserialized
// read: a 400, whether it says the setting is off or that it does not know the
// parameters. The plain read differs only in those parameters, so a 400 that
// is about something else comes back from it too, and is reported then.
func deserializeRefused(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.StatusCode == http.StatusBadRequest
}
