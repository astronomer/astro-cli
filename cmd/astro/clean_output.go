package astro

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/utils"
)

// addInertCleanOutputFlag keeps Astro CLI 1.x's --clean-output on a command
// that had it, accepted and hidden, with no effect.
//
// In 1.x the flag kept a command's stdout to the bare result (a token, a
// Deployment's YAML). Every such command now prints only the result on stdout
// and everything else on stderr, so the flag has nothing left to drop. The flag
// stays so a 1.x script that passes it, as
// `T=$(astro workspace token create ... --clean-output)` does, keeps working.
// A run that passes it is recorded in telemetry (the flag name only), which
// tells us when the flag can be removed.
func addInertCleanOutputFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("clean-output", false, "Has no effect; accepted so Astro CLI 1.x scripts keep working")
	// Marked so a run that passes it is recorded (see above).
	utils.MarkInertFlag(cmd, "clean-output")
	_ = cmd.Flags().MarkHidden("clean-output") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
}
