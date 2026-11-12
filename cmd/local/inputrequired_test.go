package local

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/input"
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

// The same refusal at the end of the input keeps its words and its exit status;
// only the kind is new.
func TestAConfirmationWithNoAnswerIsInputRequired(t *testing.T) {
	stub := newAirflowStub(t)
	d, _, _ := queryDeps(t)
	err := execute(t, d, afName, "runs", "delete", "orders_etl", "run_1", "--url", stub.URL)
	if err == nil || err.Error() != "confirmation needed but input ended with no answer; pass --yes to proceed" {
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

	err := execute(t, d, afName, "dags", "list", "-o", "json")

	msg := requireInputRequired(t, out.String(), err)
	if !strings.Contains(msg, "several deployments are linked") {
		t.Errorf("the refusal should keep the resolver's words: %q", msg)
	}
	if strings.Contains(errOut.String(), "Which deployment") {
		t.Errorf("a json run prompted: %q", errOut)
	}
}

// With no terminal on stdin, or none on stderr, a confirmation is refused
// before it is written or read, naming --yes, as the same refusal CI and json
// get: a piped answer is never read.
func TestAConfirmationWithNoTerminalIsRefusedBeforeItIsAsked(t *testing.T) {
	for name, tc := range map[string]struct {
		stdin, stderr bool
		why           string
	}{
		"stdin not a terminal":  {false, true, "stdin is not a terminal"},
		"stderr not a terminal": {true, false, "stderr is not a terminal"},
	} {
		t.Run(name, func(t *testing.T) {
			defer input.SetTerminals(tc.stdin, tc.stderr)()
			stub := newAirflowStub(t)
			d, _, errOut := queryDeps(t)
			answer := strings.NewReader("y\n")
			d.Stdin = answer
			err := execute(t, d, afName, "runs", "delete", "orders_etl", "run_1", "--url", stub.URL)
			if !input.IsRequired(err) {
				t.Fatalf("err = %v, want input_required", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.why) || !strings.Contains(msg, "pass --yes") {
				t.Errorf("the refusal should say why and name --yes: %q", msg)
			}
			if answer.Len() == 0 {
				t.Error("an answer was read")
			}
			if strings.Contains(errOut.String(), "[y/N]") {
				t.Errorf("a question was written: %q", errOut)
			}
		})
	}
}
