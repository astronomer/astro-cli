package astro

// How `astro deployment create`, `update`, `delete`, `hibernate` and
// `wake-up` publish what they did, in text and in json.

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

// deploymentOutput is --output for the five commands here. One run is one
// command, so they share it.
var deploymentOutput string

// emitDeployment publishes d, the Deployment a create or an update left.
//
// In json it is the Deployment as `astro deployment inspect -o json` shows it
// (inspect.FormattedDeployment, pinned by deployment-inspect.json), read
// afresh, so a script reads a created or updated Deployment the way it reads
// an inspected one, and the way `--deployment-file` echoes one. Read afresh
// because the answer to the create or the update can be some minutes old by
// the time a --wait ends, and its status with it.
//
// In text it is what the command always printed, drawn by text from the
// Deployment the API returned. That table is not a rendering of the inspect
// shape (its NAMESPACE is the Deployment's namespace, where inspect's
// release_name is N/A outside hybrid), and building the shape costs reads
// text has no use for, so text mode hands Emit the shape's zero value.
func emitDeployment(r cliout.Renderer, d *astrov1.Deployment, text func(io.Writer) error) error {
	var published inspect.FormattedDeployment
	if r.Format == cliout.FormatJSON {
		current, err := deployment.GetDeploymentByID("", d.Id, astroV1Client)
		if err != nil {
			return err
		}
		if published, err = inspect.Formatted(&current, astroV1Client); err != nil {
			return err
		}
	}
	return r.Emit(published, text)
}

// emitUpdated publishes an update. One that sent nothing (the DAG deploy
// setting was already as asked, or the question was declined) has already
// said so in text, and prints no table; in json it publishes the Deployment
// as it is, which is what the run left.
func emitUpdated(r cliout.Renderer, res *deployment.UpdateResult) error {
	return emitDeployment(r, &res.Deployment, func(w io.Writer) error {
		if !res.Updated {
			return nil
		}
		return deployment.WriteUpdated(w, &res.Deployment)
	})
}

// failedAfterResult is the error of a command that published its result and
// then failed: a --wait that ran out after the Deployment was created, or
// after its override was set. The result is still what happened, and a
// script needs it (the new Deployment's id, to wait again or clean up), so
// under json it stays the one object on stdout and the run exits 1 with the
// error marked JSONShown. The error's words go to stderr, after the progress
// the wait wrote there. In text the error is returned as it always was.
func failedAfterResult(cmd *cobra.Command, format cliout.Format, err error) error {
	if err == nil || format != cliout.FormatJSON {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Error: "+err.Error())
	return cliout.JSONShown(err)
}

// emitRemoval publishes a delete: in text, the line it always printed.
func emitRemoval(r cliout.Renderer, removal *deployment.Removal) error {
	return r.Emit(removal, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, "\nSuccessfully deleted deployment "+ansi.Bold(removal.Name))
	}))
}

// emitHibernation publishes the override hibernate or wake-up set, or, with
// no override, its removal. In text, the lines they always printed: action is
// the command's verb, "hibernate" or "wake up".
func emitHibernation(r cliout.Renderer, res *deployment.HibernationResult, action string) error {
	return r.Emit(res, cliout.Text(func(b *bufio.Writer) {
		switch {
		case res.Override == nil:
			fmt.Fprintln(b, "\nSuccessfully removed hibernation override")
			fmt.Fprintln(b, "If set, hibernation schedule will resume immediately.")
		case res.Override.OverrideUntil != nil:
			until := *res.Override.OverrideUntil
			fmt.Fprintf(b, "\nSuccessfully overrode to %s until %s\n", ansi.Bold(action), ansi.Bold(until.Format(time.RFC3339)))
			fmt.Fprintf(b, "If set, hibernation schedule will resume in %s.\n", ansi.Bold(time.Until(until).Round(time.Second).String()))
		default:
			fmt.Fprintf(b, "\nSuccessfully overrode to %s until further notice\n", ansi.Bold(action))
			fmt.Fprintln(b, "Any configured hibernation schedules will not resume until override is removed.")
		}
	}))
}
