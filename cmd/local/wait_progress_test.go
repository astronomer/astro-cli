package local

import (
	"runtime"
	"strings"
	"testing"
)

// oscState is a state an Airflow could answer with that carries a clipboard
// write (OSC 52) and a screen clear, as it appears inside a JSON string.
const oscState = `running\u001b]52;c;aGk=\u0007\u001b[2J`

// A value spliced into a printed command is one word of it: a safe one bare,
// as an ordinary run id is, anything else quoted.
func TestShellWordsQuotesOnlyWhatNeedsIt(t *testing.T) {
	if got := shellWords("orders_etl", "manual__2024-05-01T00:00:00+00:00"); got != "orders_etl manual__2024-05-01T00:00:00+00:00" {
		t.Errorf("safe words = %q, want them bare", got)
	}
	got := shellWords("orders_etl", "my run; rm -rf ~")
	if runtime.GOOS == "windows" {
		if got != `orders_etl "my run; rm -rf ~"` {
			t.Errorf("windows = %q", got)
		}
		return
	}
	if got != `orders_etl 'my run; rm -rf ~'` {
		t.Errorf("a run id with a space and shell syntax = %q, want it one quoted word", got)
	}
}

// A failed task's log command quotes a task id that is not safe bare, so it
// pastes as the one argument it is.
func TestTriggerWaitQuotesAnUnsafeTaskIDInItsLogCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the single-quoted form is POSIX; TestShellWordsQuotesOnlyWhatNeedsIt covers Windows")
	}
	stub, prefix := waitStub(t, false, "failed")
	stub.route("GET", prefix+"/dags/orders_etl/dagRuns/manual__1/taskInstances", `{"task_instances":[
		{"task_id":"trans form","dag_id":"orders_etl","dag_run_id":"manual__1","state":"failed","try_number":1,"map_index":-1}
	],"total_entries":1}`)

	out, _, err := runQuery(t, stub, append([]string{"runs", "trigger-wait", "orders_etl"}, fast...)...)
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d, want %d", code, exitRunFailed)
	}
	if want := "tasks logs orders_etl manual__1 'trans form' --try 1"; !strings.Contains(out, want) {
		t.Errorf("text is missing %q:\n%s", want, out)
	}
}

// assertNoTerminalControl fails when a wait's stderr carries the escape
// sequences oscState holds: its progress lines go straight to stderr, past
// the renderer that cleans a command's output, so a state is shown as text.
func assertNoTerminalControl(t *testing.T, errOut string) {
	t.Helper()
	if strings.ContainsRune(errOut, '\x1b') || strings.ContainsRune(errOut, '\a') || strings.Contains(errOut, "]52;") {
		t.Errorf("stderr carries the state's terminal controls: %q", errOut)
	}
	if !strings.Contains(errOut, "running") {
		t.Errorf("stderr lost the state's text: %q", errOut)
	}
}

func TestTasksWaitShowsTheStateAsText(t *testing.T) {
	stub := taskWaitStub(t, "running")
	stub.routeSequence(loadInstancePath, instanceBody("load", -1, oscState), instanceBody("load", -1, "success"))

	_, errOut, err := runQuery(t, stub, append([]string{"tasks", "wait", "orders_etl", "manual__1", "load"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0 once the task succeeded", code)
	}
	assertNoTerminalControl(t, errOut)
}

// stderrLine is the first line of errOut that starts with prefix, or "".
func stderrLine(errOut, prefix string) string {
	for _, line := range strings.Split(errOut, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

// trigger-wait names the run the trigger answered with before it waits; that
// id is the trigger response's, and is shown as text.
func TestTriggerWaitShowsTheTriggeredRunAsText(t *testing.T) {
	stub, prefix := waitStub(t, false, "success")
	stub.route("POST", prefix+"/dags/orders_etl/dagRuns",
		`{"dag_id":"orders_etl","dag_run_id":"manual__1\u001b]52;c;aGk=\u0007","state":"queued","run_type":"manual"}`)

	_, errOut, _ := runQuery(t, stub, append([]string{"runs", "trigger-wait", "orders_etl"}, fast...)...)
	line := stderrLine(errOut, "Triggered ")
	if !strings.Contains(line, "manual__1") {
		t.Fatalf("no Triggered line naming the run in stderr: %q", errOut)
	}
	if strings.ContainsAny(line, "\x1b\a") {
		t.Errorf("the Triggered line carries the run id's terminal controls: %q", line)
	}
}

// A failed run's task instances that cannot be listed are reported on stderr
// with Airflow's own message, shown as text.
func TestRunsWaitShowsAListingErrorAsText(t *testing.T) {
	stub := existingRunStub(t, "failed")
	stub.routeStatus("GET", "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances", 500,
		`{"detail":"listing broke\u001b]52;c;aGk=\u0007"}`)

	_, errOut, _ := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1"}, fast...)...)
	line := stderrLine(errOut, "could not list the run's task instances")
	if line == "" {
		t.Fatalf("no listing error in stderr: %q", errOut)
	}
	if strings.ContainsAny(line, "\x1b\a") {
		t.Errorf("the listing error carries Airflow's terminal controls: %q", line)
	}
}

func TestRunsWaitShowsTheStateAsText(t *testing.T) {
	stub := existingRunStub(t, oscState, oscState+"x", "success")

	_, errOut, err := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0 for a run that succeeded", code)
	}
	assertNoTerminalControl(t, errOut)
}
