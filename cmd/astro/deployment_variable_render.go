package astro

// Rendering for `astro deployment variable`. The platform package returns what
// happened; this file is the only place that decides how it looks.

import (
	"fmt"
	"io"
	"strconv"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// maskedSecret stands in for a secret's value in the table. The API never
// returns one, so this is a rendering choice and belongs here rather than in
// the value a JSON caller receives.
const maskedSecret = "****"

// renderVariables prints the variable table, or says there are none.
func renderVariables(out io.Writer, vars []deployment.VariableInfo, empty string) {
	if len(vars) == 0 {
		fmt.Fprintln(out, "\n"+empty)
		return
	}
	table := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "KEY", "VALUE", "SECRET"},
	}
	for i, v := range vars {
		value := v.Value
		if v.IsSecret {
			value = maskedSecret
		}
		table.AddRow([]string{strconv.Itoa(i + 1), v.Key, value, strconv.FormatBool(v.IsSecret)}, false)
	}
	table.Print(out) //nolint:errcheck // best-effort render to the terminal
}

// renderOutcomes prints one line per input, in the order the inputs were given.
//
// Every line goes to the same writer. Under the old shape half of these went to
// the caller's writer and half to bare stdout, so a captured run saw only some
// of them.
func renderOutcomes(out io.Writer, outcomes []deployment.VariableOutcome) {
	for _, o := range outcomes {
		switch o.Kind {
		case deployment.VariableCreated:
			fmt.Fprintf(out, "adding variable %s\n", o.Key)
		case deployment.VariableUpdated:
			fmt.Fprintf(out, "updating variable %s\n", o.Key)
		case deployment.VariableSkippedExists:
			fmt.Fprintf(out, "key %s already exists, skipping creation. Use the update command to update existing variables\n", o.Key)
		case deployment.VariableInvalid:
			subject := o.Input
			if subject == "" {
				subject = o.Key
			}
			fmt.Fprintf(out, "%s not created or updated: %s\n", subject, o.Reason)
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

// renderVariableModify prints what a modify run did and picks its error.
//
// The outcomes come first, then the Deployment's list under a heading: the two
// answer different questions, and without the heading the table reads as more
// output about the inputs rather than the state they left behind.
func renderVariableModify(out io.Writer, res *deployment.VariableModifyResult) error {
	renderOutcomes(out, res.Outcomes)
	if len(res.Variables) > 0 {
		fmt.Fprintln(out, "\nUpdated list of your Deployment's variables:")
	}
	renderVariables(out, res.Variables, "No variables for this Deployment")
	return errInvalidInputs(res)
}
