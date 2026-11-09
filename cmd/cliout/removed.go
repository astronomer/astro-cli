package cliout

import (
	"fmt"
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
//
// The stub's Short names cmd by its whole path, which is known only once the
// tree is built: NameRemovedAliasTargets sets it then. Until that runs it
// names cmd by the path it has so far.
func RemovedAlias(alias string, cmd *cobra.Command) *cobra.Command {
	stub := RemovedCommand(alias, nil, "", func(stub *cobra.Command, _ []string) error {
		return Usage(fmt.Errorf("%s was removed in Astro CLI v2: %s is no longer short for %s. Use %s.",
			stub.CommandPath(), alias, cmd.Name(), useInstead(cmd)))
	})
	stub.Annotations[RemovedAliasOfAnnotation] = cmd.Name()
	stub.Short = removedAliasShort(cmd)
	return stub
}

// RemovedAliasOfAnnotation names, on a RemovedAlias stub, the sibling command
// the alias used to spell.
const RemovedAliasOfAnnotation = "removedAliasOf"

// RemovedAliasTarget returns the command a RemovedAlias stub's alias used to
// spell, its sibling, or nil when stub is not one.
func RemovedAliasTarget(stub *cobra.Command) *cobra.Command {
	name := stub.Annotations[RemovedAliasOfAnnotation]
	if name == "" || !stub.HasParent() {
		return nil
	}
	for _, sibling := range stub.Parent().Commands() {
		if sibling.Name() == name && sibling != stub {
			return sibling
		}
	}
	return nil
}

// NameRemovedAliasTargets sets the Short of every RemovedAlias stub under root
// to name its command by the whole path, once root is built.
func NameRemovedAliasTargets(root *cobra.Command) {
	for _, c := range root.Commands() {
		if target := RemovedAliasTarget(c); target != nil {
			c.Short = removedAliasShort(target)
		}
		NameRemovedAliasTargets(c)
	}
}

func removedAliasShort(cmd *cobra.Command) string {
	return "Removed in v2 — use " + useInstead(cmd)
}

// useInstead names cmd by its path, and its aliases after it:
// "astro deployment delete, or its alias rm".
func useInstead(cmd *cobra.Command) string {
	switch len(cmd.Aliases) {
	case 0:
		return cmd.CommandPath()
	case 1:
		return cmd.CommandPath() + ", or its alias " + cmd.Aliases[0]
	default:
		return cmd.CommandPath() + ", or one of its aliases " + strings.Join(cmd.Aliases, ", ")
	}
}

// IsRemovedCommand reports whether cmd is a removed command's stub.
func IsRemovedCommand(cmd *cobra.Command) bool {
	return cmd.Annotations[RemovedCommandAnnotation] == "true"
}
