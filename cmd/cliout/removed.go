package cliout

import (
	"fmt"
	"slices"
	"strings"

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

// RemovedAlias builds the stub for a 1.x alias v2 took away from cmd, to be
// added as cmd's sibling: `astro deployment de` once `de` no longer spells
// `astro deployment delete`. A dropped alias is a removed spelling like any
// removed command, so a 1.x script using it fails naming cmd rather than
// being told only that the word is unknown, and never runs cmd.
func RemovedAlias(alias string, cmd *cobra.Command) *cobra.Command {
	// Both the Short and the message read cmd's aliases here, as it is built,
	// so the stub's help and its error never disagree.
	aliases := slices.Clone(cmd.Aliases)
	short := "Removed in v2 — use " + strings.Join(append([]string{cmd.Name()}, aliases...), " or ")
	var others string
	switch len(aliases) {
	case 0:
	case 1:
		others = ", or its alias " + aliases[0]
	default:
		others = ", or one of its aliases " + strings.Join(aliases, ", ")
	}
	return RemovedCommand(alias, nil, short, func(stub *cobra.Command, _ []string) error {
		return Usage(fmt.Errorf("%s was removed in Astro CLI v2: %s is no longer short for %s. Use %s%s.",
			stub.CommandPath(), alias, cmd.Name(), cmd.CommandPath(), others))
	})
}

// IsRemovedCommand reports whether cmd is a removed command's stub.
func IsRemovedCommand(cmd *cobra.Command) bool {
	return cmd.Annotations[RemovedCommandAnnotation] == "true"
}
