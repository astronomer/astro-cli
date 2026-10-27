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
// On Airflow 3 the value is asked for deserialized. Airflow 3.0 does that only
// with [api] enable_xcom_deserialize_support on, and refuses otherwise; 3.1
// dropped the setting and deserializes for anyone. Which one answered is
// learned from the answer, never from the version: a refusal is asked again
// for the stored value. Airflow 2 has the same setting, off by default, and is
// not asked: the stored value there is the string it keeps.
func (c *Client) GetXComEntry(ctx context.Context, dagID, runID, taskID, key string, mapIndex int) (XComEntry, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return XComEntry{}, err
	}
	path := pathf("/dags/%s/dagRuns/%s/taskInstances/%s/xcomEntries/%s", dagID, runID, taskID, key)
	var entry XComEntry
	if generation == Airflow3 {
		err = c.get(ctx, path, xcomQuery(mapIndex, true), &entry)
		if !deserializeRefused(err) {
			return entry, err
		}
	}
	err = c.get(ctx, path, xcomQuery(mapIndex, false), &entry)
	return entry, err
}

// xcomQuery addresses one expansion when there is one and, when asked, has
// Airflow 3 deserialize the value. Without that, Airflow 3 answers with its
// serialization envelopes (__classname__, __data__) rather than the value the
// task returned; stringify=false keeps the answer JSON rather than Python's
// rendering of it.
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

// deserializeRefused reports Airflow 3.0's answer to deserialize=true with its
// [api] enable_xcom_deserialize_support off: a 400 that says so. Any other
// refusal is the caller's to see.
func deserializeRefused(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.StatusCode == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(status.Detail()), "deserialization is disabled")
}
