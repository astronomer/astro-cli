package local

import (
	"strings"
	"testing"
)

// oscState is a state an Airflow could answer with that carries a clipboard
// write (OSC 52) and a screen clear, as it appears inside a JSON string.
const oscState = `running\u001b]52;c;aGk=\u0007\u001b[2J`

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

func TestRunsWaitShowsTheStateAsText(t *testing.T) {
	stub := existingRunStub(t, oscState, oscState+"x", "success")

	_, errOut, err := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0 for a run that succeeded", code)
	}
	assertNoTerminalControl(t, errOut)
}
