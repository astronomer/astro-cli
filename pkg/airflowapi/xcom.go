package airflowapi

import (
	"context"
	"encoding/json"
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
	// Value is kept as it arrived. On Airflow 3 it is the deserialized value
	// as JSON, with anything JSON cannot hold (a datetime, a custom class)
	// rendered as a string. Airflow 2 deserializes only when its
	// [api] enable_xcom_deserialize_support is on, which is off by default,
	// so there it is the string Airflow 2 stores for the value.
	Value json.RawMessage `json:"value"`
}

// GetXComEntry reads the XCom value a task instance pushed under key. A
// negative mapIndex reads an unmapped task's value; a mapped task keeps one
// value per expansion, so it needs the index.
func (c *Client) GetXComEntry(ctx context.Context, dagID, runID, taskID, key string, mapIndex int) (XComEntry, error) {
	version, err := c.Generation(ctx)
	if err != nil {
		return XComEntry{}, err
	}
	query := url.Values{}
	if mapIndex >= 0 {
		query.Set("map_index", strconv.Itoa(mapIndex))
	}
	if version == Airflow3 {
		// Without these, Airflow 3 answers with its serialization envelopes
		// (__classname__, __data__) rather than the value the task returned.
		query.Set("deserialize", "true")
		query.Set("stringify", "false")
	}
	var entry XComEntry
	err = c.get(ctx, pathf("/dags/%s/dagRuns/%s/taskInstances/%s/xcomEntries/%s", dagID, runID, taskID, key), query, &entry)
	return entry, err
}
