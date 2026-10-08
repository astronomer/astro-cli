package astro

// Rendering for `astro deployment variable`. The platform package returns what
// happened; this file is the only place that decides how it looks, in text and
// in json.

import (
	"bufio"
	"fmt"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// deploymentVariableOutput is --output for the whole `deployment variable`
// family, registered once on the family's root.
var deploymentVariableOutput cliout.Format

// strayStdoutToStderr points os.Stdout at stderr for the rest of a json-mode
// run, and returns what puts it back.
//
// The platform code these commands call is inherited shell code that still
// prints notes to bare stdout: GetDeployment's "Only one Deployment was found"
// and "More than one Deployment with the name", and Update's Deployment table
// when the variable list it is handed is empty. In text mode that is the
// output people have always seen, so it stays. Under json, stdout carries one
// object and nothing else, and a note is not the result, so it goes to stderr,
// where a person still sees it and a parser does not. The command's own writer
// was bound to the real stdout when the tree was built, so the result still
// leaves by it.
func strayStdoutToStderr(format cliout.Format) (restore func()) {
	if format != cliout.FormatJSON {
		return func() {}
	}
	saved := os.Stdout
	os.Stdout = os.Stderr
	return func() { os.Stdout = saved }
}

// maskedSecret stands in for a secret's value in the table. The API never
// returns one, so this is a rendering choice and belongs here rather than in
// the value a JSON caller receives.
const maskedSecret = "****"

// renderVariables prints the variable table, or says there are none.
func renderVariables(b *bufio.Writer, vars []deployment.VariableInfo, empty string) {
	table := cliout.Table{
		Header: []string{"#", "KEY", "VALUE", "SECRET"},
		Empty:  "\n" + empty,
	}
	for i, v := range vars {
		value := v.Value
		if v.IsSecret {
			value = maskedSecret
		}
		table.AddRow(strconv.Itoa(i+1), v.Key, value, strconv.FormatBool(v.IsSecret))
	}
	table.Render(b)
}

// renderOutcomes prints one line per input, in the order the inputs were given.
//
// Every line goes to the same writer. Under the old shape half of these went to
// the caller's writer and half to bare stdout, so a captured run saw only some
// of them.
func renderOutcomes(b *bufio.Writer, outcomes []deployment.VariableOutcome) {
	for _, o := range outcomes {
		switch o.Kind {
		case deployment.VariableCreated:
			fmt.Fprintf(b, "adding variable %s\n", o.Key)
		case deployment.VariableUpdated:
			fmt.Fprintf(b, "updating variable %s\n", o.Key)
		case deployment.VariableSkippedExists:
			fmt.Fprintf(b, "key %s already exists, skipping creation. Use the update command to update existing variables\n", o.Key)
		case deployment.VariableInvalid:
			subject := o.Input
			if subject == "" {
				subject = o.Key
			}
			fmt.Fprintf(b, "%s not created or updated: %s\n", subject, o.Reason)
		}
	}
}

// errInvalidInputs names what did not become a variable, so a caller that
// captured nothing still learns which inputs failed. The old error said "check
// the command output above", which only answers a human at a terminal.
func errInvalidInputs(res *deployment.VariableModifyResult) error {
	inputs := res.InvalidInputs()
	if len(inputs) == 0 {
		return nil
	}
	subject, verb := "variable", "was"
	if len(inputs) > 1 {
		subject, verb = "variables", "were"
	}
	return errors.Errorf("%d %s %s not created or updated: %s",
		len(inputs), subject, verb, quoteList(inputs))
}

func quoteList(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += strconv.Quote(s)
	}
	return out
}

// renderVariableModify prints what a modify run did.
//
// The outcomes come first, then the Deployment's list under a heading: the two
// answer different questions, and without the heading the table reads as more
// output about the inputs rather than the state they left behind.
func renderVariableModify(b *bufio.Writer, res *deployment.VariableModifyResult) {
	renderOutcomes(b, res.Outcomes)
	if len(res.Variables) > 0 {
		fmt.Fprintln(b, "\nUpdated list of your Deployment's variables:")
	}
	renderVariables(b, res.Variables, "No variables for this Deployment")
}

// emitVariableList publishes `variable list`'s result: {"variables": [...]} in
// json, the table in text.
//
// --save's notice says where the file went. In text it heads the table, as it
// always has. In json it is a note rather than the result, so it goes to
// stderr and stdout keeps only the object.
func emitVariableList(cmd *cobra.Command, r cliout.Renderer, vars *deployment.DeploymentVariables) error {
	if useEnvFile {
		notice := r.Out
		if r.Format == cliout.FormatJSON {
			notice = cmd.ErrOrStderr()
		}
		fmt.Fprintf(notice, "\nThe following environment variables were saved to the file %s,\nsecret environment variables were saved only with a key:\n\n", envFile)
	}
	return r.Emit(vars, cliout.Text(func(b *bufio.Writer) {
		renderVariables(b, vars.Variables, "No variables found")
	}))
}

// emitVariableModify publishes a create or update run's result and picks its
// error: an input that became no variable fails the run, in both modes.
//
// The result is published first either way, because it is what says which
// inputs failed. Under json that object is the run's whole report, so the
// error is marked JSONShown: the process still exits 1, and the root does not
// print a second, plainer object after it. In text the error stays unmarked,
// so the root prints it on stderr as it always has.
func emitVariableModify(r cliout.Renderer, res *deployment.VariableModifyResult) error {
	if err := r.Emit(res, cliout.Text(func(b *bufio.Writer) {
		renderVariableModify(b, res)
	})); err != nil {
		return err
	}
	err := errInvalidInputs(res)
	if err != nil && r.Format == cliout.FormatJSON {
		return cliout.JSONShown(err)
	}
	return err
}
