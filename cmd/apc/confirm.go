package apc

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/input"
)

// addYesFlag registers --yes/-y on a command that asks before it removes
// something, as the Astro commands that remove things do.
func addYesFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVarP(&skipPrompt, "yes", "y", false, usage)
}

// confirmRemoval asks question before a command removes something that cannot
// be brought back, unless --yes answered it already, and reports whether to go
// on. A declined question says canceled on out. A run that may not ask (under
// --output json) is refused, naming --yes, before anything is read.
func confirmRemoval(out io.Writer, question, canceled string) (bool, error) {
	if skipPrompt {
		return true, nil
	}
	ok, err := input.Confirm(question, input.AnsweredBy("--yes"))
	if err != nil {
		return false, err
	}
	if !ok {
		fmt.Fprintln(out, canceled)
	}
	return ok, nil
}
