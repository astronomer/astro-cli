package cliout

import (
	"github.com/spf13/cobra"
)

// RemovedCommandAnnotation marks the stub that stands in for a command Astro
// CLI v2 removed (RemovedCommand). The root's pre-run reads it on the command
// run: a stub only says what replaced it, so it needs no login, no project and
// no version check, but it is logged and recorded like any command, which is
// what tells us when nobody types it any more.
const RemovedCommandAnnotation = "removedCommand"

// RemovedCommand builds the stub for a removed command: `astro dev`, `astro
// run`, `astro deployment pool`, `astro env variable create`, ... Every stub
// has this shape, and the 1.x command guard
// (TestEveryV1CommandStillRunsOrSaysWhatReplacedIt in cmd) runs each one.
//
//   - Hidden, so help teaches only what exists.
//   - Any arguments, with flag parsing off, so an old invocation's subcommand
//     and flags reach the guidance instead of dying on an unknown flag.
//   - Usage silenced: the guidance is the whole message.
//   - A pre-run of its own, which runs the root's and no group's.
//   - --output declared, though never parsed: Execute reads it from the raw
//     arguments for a usage error, so under -o json the failure is the one
//     error object.
//
// run fails with a usage error (Usage), whose message says the command "was
// removed in Astro CLI v2" and names what to use instead, or says there is
// nothing. short starts "Removed in v2" and says the same in brief.
func RemovedCommand(use string, aliases []string, short string, run func(cmd *cobra.Command, args []string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:                use,
		Aliases:            aliases,
		Short:              short,
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Annotations:        map[string]string{RemovedCommandAnnotation: "true"},
		// Its own, so cobra never picks a group's on its way up (`astro env`
		// and `astro deployment` look the project up): only the root's runs,
		// which reads the annotation.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if root := cmd.Root(); root != cmd && root.PersistentPreRunE != nil {
				return root.PersistentPreRunE(cmd, args)
			}
			return nil
		},
		RunE: run,
	}
	AddOutputFlag(cmd, new(Format))
	return cmd
}

// IsRemovedCommand reports whether cmd is a removed command's stub.
func IsRemovedCommand(cmd *cobra.Command) bool {
	return cmd.Annotations[RemovedCommandAnnotation] == "true"
}
