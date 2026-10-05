package local

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// requireInputRequired asserts stdout is the one error object a refused
// question publishes, and returns its message.
func requireInputRequired(t *testing.T, stdout string, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("the command should fail: it needed an answer")
	}
	var obj cliout.ErrorObject
	if jerr := json.Unmarshal([]byte(stdout), &obj); jerr != nil || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout is not one error object: %q (%v)", stdout, jerr)
	}
	if obj.Kind != cliout.KindInputRequired || obj.Code != cliout.ExitFailure {
		t.Fatalf("got %+v, want kind %q and code %d", obj, cliout.KindInputRequired, cliout.ExitFailure)
	}
	return obj.Error
}

// A confirmation under --output json is refused before it is asked: on a
// terminal with an answer waiting, nothing is read and nothing is changed, and
// the refusal names the flag that gives consent.
func TestAJSONConfirmationIsNeverAsked(t *testing.T) {
	stub := newAirflowStub(t)
	path := "/api/v2/dags/orders_etl/dagRuns/run_1"
	stub.route(http.MethodDelete, path, `{}`)

	d, out, errOut := queryDeps(t)
	answer := strings.NewReader("y\n")
	d.Stdin = answer
	d.Interactive = func() bool { return true }
	err := execute(t, d, afName, "runs", "delete", "orders_etl", "run_1", "--url", stub.URL, "-o", "json")

	msg := requireInputRequired(t, out.String(), err)
	if !strings.Contains(msg, "Delete run run_1 of orders_etl?") || !strings.Contains(msg, "pass --yes") {
		t.Errorf("the refusal should name the question and --yes: %q", msg)
	}
	if answer.Len() == 0 {
		t.Error("a json run read stdin")
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Errorf("a json run asked: %q", errOut)
	}
	if stub.sawRequest(http.MethodDelete, path) {
		t.Error("the run was deleted without consent")
	}
}

// The same refusal without a terminal keeps its words and its exit status;
// only the kind is new.
func TestAConfirmationWithNoTerminalIsInputRequired(t *testing.T) {
	stub := newAirflowStub(t)
	d, _, _ := queryDeps(t)
	err := execute(t, d, afName, "runs", "delete", "orders_etl", "run_1", "--url", stub.URL)
	if err == nil || err.Error() != "confirmation needed but stdin is not interactive; pass --yes to proceed" {
		t.Fatalf("err = %v, want the refusal in its own words", err)
	}
	if code := cliout.ExitCode(t.Context(), err); code != cliout.ExitFailure {
		t.Errorf("exit %d, want %d", code, cliout.ExitFailure)
	}
	if kind := ProblemKinds.Of(err); kind != cliout.KindInputRequired {
		t.Errorf("kind %q, want %q", kind, cliout.KindInputRequired)
	}
}

// A picker under --output json is not drawn: `astro link remove` with no NAME
// asks which link at a terminal, and here fails naming how to say it instead.
func TestAJSONPickerIsNeverDrawn(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, out, _ := instanceDeps(t, dir)
	answer := strings.NewReader("1\n")
	d.Stdin = answer
	d.Interactive = func() bool { return true }
	d.OutputTerminal = func() bool { return true }

	err := execute(t, d, "link", "remove", "-o", "json")

	msg := requireInputRequired(t, out.String(), err)
	if !strings.Contains(msg, "astro link remove NAME") {
		t.Errorf("the refusal should say how to name the link: %q", msg)
	}
	if answer.Len() == 0 {
		t.Error("a json run read stdin")
	}
}

// A deployment the project cannot resolve on its own is the question a
// terminal run asks; under --output json it is input_required.
func TestAJSONAmbiguousDeploymentIsInputRequired(t *testing.T) {
	dir := instanceProject(t, ambiguousManifest)
	d, out, errOut := instanceDeps(t, dir)
	d.Stdin = strings.NewReader("2\n")
	d.Interactive = func() bool { return true }

	err := execute(t, d, afName, "dags", "list", "-o", "json")

	msg := requireInputRequired(t, out.String(), err)
	if !strings.Contains(msg, "several deployments are linked") {
		t.Errorf("the refusal should keep the resolver's words: %q", msg)
	}
	if strings.Contains(errOut.String(), "Which deployment") {
		t.Errorf("a json run prompted: %q", errOut)
	}
}
