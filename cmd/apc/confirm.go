package apc

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/input"
)

// addYesFlag registers --yes/-y on a command that asks before it removes
// something, as the Astro commands that remove things do.
func addYesFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVarP(&skipPrompt, "yes", "y", false, usage)
}

// confirm asks question before a command changes something that is not
// easily changed back (a removal, a hibernation, a move onto dag_deploy),
// unless --yes answered it already. A declined question is input.ErrAborted,
// as it is on Astro. A run that may not ask (under --output json) is refused,
// naming --yes, before anything is read.
func confirm(question string) error {
	if skipPrompt {
		return nil
	}
	return input.ConfirmOrAbort(question, input.AnsweredBy("--yes"))
}
