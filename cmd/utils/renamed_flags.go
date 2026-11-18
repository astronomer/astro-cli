package utils

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

// noteRenamedFlags runs once ApplyPreferredFlags has accepted a run's flags.
// For the 1.x spellings it gave, which v2 keeps as hidden aliases (AddAlias),
// it warns, through the CLI's one deprecation helper (cliout.WarnDeprecated),
// that the spelling is deprecated and what to type instead, so that a script
// gets updated before the alias goes, and records them (RecordRenamedFlags),
// which is what tells us when an alias can go. Stdout is left alone, so -o
// json output stays one document, and nothing turns the warning off.
func noteRenamedFlags(cmd *cobra.Command) {
	var renamed, current []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if p := aliasOf(f); p != "" {
			renamed, current = append(renamed, "--"+f.Name), append(current, "--"+p)
		}
	})
	if len(renamed) == 0 {
		return
	}
	cliout.WarnDeprecatedFlags(cmd.ErrOrStderr(), cmd)
	RecordRenamedFlags(cmd, renamed, current)
}

// RecordRenamedFlags queues the event; a test swaps it to see what was sent.
var RecordRenamedFlags = telemetry.RecordRenamedFlags
