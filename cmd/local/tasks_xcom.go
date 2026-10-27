package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// defaultXComKey is the key a task's return value is pushed under, which is
// what nearly every read of XCom is after.
const defaultXComKey = "return_value"

// xcomRow is one XCom value and the task instance it came from. The ids come
// from the command line: Airflow 2 does not echo the run id back.
type xcomRow struct {
	DAGID     string `json:"dag_id"`
	RunID     string `json:"dag_run_id"`
	TaskID    string `json:"task_id"`
	MapIndex  int    `json:"map_index"`
	Key       string `json:"key"`
	Timestamp string `json:"timestamp,omitempty"`
	Value     any    `json:"value"`
}

func newTasksXComCmd(q *query) *cobra.Command {
	var opts struct {
		mapIndex int
		key      string
	}
	cmd := &cobra.Command{
		Use:   "xcom <DAG_ID> <RUN_ID> <TASK_ID>",
		Short: "Print the value a task instance pushed to XCom",
		Long: "Print the value one task instance pushed to XCom: by default its return value, or the value " +
			"under --key. Text output is the value alone, a string as it is and anything else as indented " +
			"JSON, so it can be piped; --output json wraps it with the ids it came from.\n\n" +
			"On Airflow 3 the value is deserialized, and anything JSON cannot hold, such as a datetime, " +
			"arrives as a string. Airflow 2 returns the string it stores for the value.",
		Args: cobra.ExactArgs(taskInstanceArgs),
		Example: afExample(q.t,
			"Print what a task returned", "tasks xcom <DAG_ID> <RUN_ID> <TASK_ID>",
			"One expansion of a mapped task", "tasks xcom <DAG_ID> <RUN_ID> <TASK_ID> --map-index 2",
			"A value pushed under its own key", "tasks xcom <DAG_ID> <RUN_ID> <TASK_ID> --key rows_loaded",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			return q.runTasksXCom(cmd.Context(), args[0], args[1], args[2], opts.key, opts.mapIndex)
		},
	}
	cmd.Flags().IntVarP(&opts.mapIndex, "map-index", "m", -1, "Which expansion of a mapped task to read (-1 for an unmapped task)")
	cmd.Flags().StringVar(&opts.key, "key", defaultXComKey, "The XCom key to read")
	return cmd
}

func (q *query) runTasksXCom(ctx context.Context, dagID, runID, taskID, key string, mapIndex int) error {
	r, client, err := q.open(ctx)
	if err != nil {
		return err
	}
	entry, err := client.GetXComEntry(ctx, dagID, runID, taskID, key, mapIndex)
	if err != nil {
		return err
	}
	value, err := decodeXComValue(entry.Value)
	if err != nil {
		return err
	}
	row := xcomRow{
		DAGID:     dagID,
		RunID:     runID,
		TaskID:    taskID,
		MapIndex:  mapIndex,
		Key:       key,
		Timestamp: stamp(entry.Timestamp),
		Value:     value,
	}
	return r.Emit(row, func(w io.Writer) error { return renderXComValue(w, row.Value) })
}

// decodeXComValue reads the value Airflow sent, keeping numbers exact: a row
// count or an id past 2^53 would lose digits as a float64.
func decodeXComValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("airflow sent an XCom value that is not JSON: %w", err)
	}
	return value, nil
}

// renderXComValue writes a value for a person or a pipe: a string without its
// JSON quotes, so text reads as text, and anything else as indented JSON.
func renderXComValue(w io.Writer, value any) error {
	if text, ok := value.(string); ok {
		_, err := fmt.Fprintln(w, text)
		return err
	}
	//astro:non-output-json // the value alone for text output; --output json goes through Emit
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(out))
	return err
}
