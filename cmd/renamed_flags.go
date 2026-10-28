package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

// renameHintsDisabledEnv hides the hint noteRenamedFlags prints, for a
// script that still has to run on Astro CLI 1.x too, where the new spelling
// does not exist yet. It hides only the hint: the event still goes, unless
// telemetry is off.
const renameHintsDisabledEnv = "ASTRO_RENAME_HINTS_DISABLED"

// noteRenamedFlags runs on every command once its flags are parsed
// (utils.BeforeArgs). For each 1.x spelling the run gave, which v2 keeps as a
// hidden alias of its new one (utils.AddAlias), it says on stderr what the
// flag is called now, so that a script gets updated before the alias goes,
// and records the alias's use (telemetry.TrackRenamedFlag), which is what
// tells us when the alias can go. It never fails the run, and stdout is left
// alone, so -o json output stays one document.
func noteRenamedFlags(cmd *cobra.Command, _ []string) error {
	hint := !renameHintsDisabled()
	utils.VisitGivenAliases(cmd.Flags(), func(alias, preferred string) {
		if hint {
			fmt.Fprintf(cmd.ErrOrStderr(), "Flag --%s has been renamed to --%s, and --%s will be removed in a future release. Set %s=1 to hide this message.\n",
				alias, preferred, alias, renameHintsDisabledEnv)
		}
		recordRenamedFlag(cmd, "--"+alias, "--"+preferred)
	})
	return nil
}

func renameHintsDisabled() bool {
	v := os.Getenv(renameHintsDisabledEnv)
	return v == "1" || strings.EqualFold(v, "true")
}

// recordRenamedFlag sends the event; a test swaps it to see what was sent.
var recordRenamedFlag = telemetry.TrackRenamedFlag
