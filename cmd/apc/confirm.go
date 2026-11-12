package apc

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/input"
)

// addYesFlag registers --yes/-y on a command that asks before it changes
// something that is not easily changed back, as the Astro commands that do.
func addYesFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVarP(&skipPrompt, "yes", "y", false, usage)
}

// confirm asks question before a command changes something that is not
// easily changed back (a removal, a hibernation, a move onto dag_deploy),
// unless --yes answered it already. A declined question is input.ErrAborted,
// as it is on Astro. A run that may not ask (under --output json) is refused,
// naming --yes, before the question is printed, stdin is read, or anything is changed (a command that looks something up in Houston first, as hibernate and delete do, has done that read by then).
func confirm(question string) error {
	return confirmAfter(question, nil)
}

// confirmAfter is confirm for a command that has work to do before it asks,
// which it does only once the run is known to be able to ask (or to have
// been answered by --yes), and runs exactly once on every path: prepare
// reads what the question is about, and what it read is what the command
// sends. A run that is refused, or whose prepare fails, asks nothing. A nil
// prepare is no work.
func confirmAfter(question string, prepare func() error) error {
	if skipPrompt {
		if prepare == nil {
			return nil
		}
		return prepare()
	}
	if prepare != nil {
		if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
			return err
		}
		if err := prepare(); err != nil {
			return err
		}
	}
	return input.ConfirmOrAbort(question, input.AnsweredBy("--yes"))
}
