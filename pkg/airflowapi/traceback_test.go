package airflowapi

import (
	"net/http"
	"strings"
	"testing"
)

// An Airflow 3 failure carries "Task failed with exception" as its event and
// the exception itself in error_detail. Rendering the event alone tells a
// reader a task failed and never why, which is the one thing they opened the
// log for.
func TestTaskLogRendersTheTracebackOnAirflow3(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/etl/dagRuns/r1/taskInstances/load/logs/1", `{"content":[
		{"event":"Filling up the DagBag from /tmp/etl.py"},
		{"event":"Task failed with exception","level":"error","error_detail":[
			{"exc_type":"KeyError","exc_value":"'row_count'","exc_notes":[],"is_cause":false,"frames":[
				{"filename":"/src/airflow/sdk/bases/decorator.py","lineno":252,"name":"execute"},
				{"filename":"/tmp/etl.py","lineno":12,"name":"transform"}
			]}
		]}
	]}`)
	client := stub.client()

	log, err := client.TaskLogs(t.Context(), "etl", "r1", "load", TaskLogsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := log.Text()
	for _, want := range []string{
		"Filling up the DagBag from /tmp/etl.py",
		"Task failed with exception",
		"Traceback (most recent call last):",
		`File "/tmp/etl.py", line 12, in transform`,
		"KeyError: 'row_count'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Text() is missing %q\ngot:\n%s", want, got)
		}
	}
}

// A chained exception reads the way Python prints one, so the two halves are
// not mistaken for two separate failures. is_cause picks which sentence joins
// them: `raise X from Y` is a direct cause, a raise inside an except block is
// not.
func TestTracebackJoinsAChainTheWayPythonDoes(t *testing.T) {
	cases := []struct {
		name    string
		isCause bool
		want    string
	}{
		{"raise from", true, "The above exception was the direct cause of the following exception:"},
		{"raise inside except", false, "During handling of the above exception, another exception occurred:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := traceback([]exceptionV3{
				{Type: "KeyError", Value: "'rows'", IsCause: tc.isCause, Frames: []frameV3{{Filename: "/tmp/etl.py", Lineno: 12, Name: "transform"}}},
				{Type: "ValueError", Value: "no rows", Frames: []frameV3{{Filename: "/tmp/etl.py", Lineno: 14, Name: "transform"}}},
			})
			if !strings.Contains(got, tc.want) {
				t.Errorf("traceback() is missing %q\ngot:\n%s", tc.want, got)
			}
			if strings.Index(got, "KeyError") > strings.Index(got, "ValueError") {
				t.Errorf("chain is out of order, want the raised-last exception last\ngot:\n%s", got)
			}
		})
	}
}

// An entry with an event and no error_detail is unchanged — the common case,
// and the one every other line of a log takes.
func TestTracebackIsAbsentFromAnOrdinaryEntry(t *testing.T) {
	if got := logLine([]byte(`{"event":"Done. Returned value was: 3"}`)); got != "Done. Returned value was: 3" {
		t.Errorf("logLine() = %q, want the event alone", got)
	}
}

// An error_detail in a shape this does not know costs the traceback and
// nothing else. Decoding it as part of the entry would fail the whole entry
// and print raw JSON in place of a line that read fine before there was any
// traceback rendering at all.
func TestAnUnknownErrorDetailStillLeavesTheEventReadable(t *testing.T) {
	for _, entry := range []string{
		`{"event":"Task failed with exception","error_detail":{"exc_type":"KeyError"}}`,
		`{"event":"Task failed with exception","error_detail":"KeyError: 'rows'"}`,
		`{"event":"Task failed with exception","error_detail":[]}`,
		`{"event":"Task failed with exception","error_detail":null}`,
	} {
		if got := logLine([]byte(entry)); got != "Task failed with exception" {
			t.Errorf("logLine(%s) = %q, want the event alone", entry, got)
		}
	}
}

// exc_notes are what PEP 678 add_note attaches, and Airflow passes them
// through. They print under the exception, as Python prints them.
func TestTracebackKeepsExceptionNotes(t *testing.T) {
	got := traceback([]exceptionV3{{
		Type:  "KeyError",
		Value: "'rows'",
		Notes: []string{"batch came from extract()"},
	}})
	if !strings.Contains(got, "batch came from extract()") {
		t.Errorf("traceback() dropped the note\ngot:\n%s", got)
	}
}
