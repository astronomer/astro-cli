package cliout

import (
	"errors"

	"github.com/spf13/cobra"
)

// AddRemovedFlag tombstones a flag a release removed: it registers name,
// hidden, so that a run passing it fails with msg instead of cobra's bare
// "unknown flag", which names no replacement. isBool is the flag's old kind:
// a string flag still consumes its value, so `--deployment-file f.yaml` does
// not leave f.yaml behind as an argument.
//
// The check runs in Args, which cobra calls before any pre-run, so the run
// fails before it logs in or asks an API anything. The error is a usage error
// (exit 2) whoever runs the command, not only under Execute, which reports it
// under --output json as the error object.
func AddRemovedFlag(cmd *cobra.Command, name, shorthand string, isBool bool, msg string) {
	if isBool {
		cmd.Flags().BoolP(name, shorthand, false, "")
	} else {
		cmd.Flags().StringP(name, shorthand, "", "")
	}
	if err := cmd.Flags().MarkHidden(name); err != nil {
		panic(err) // the flag is defined just above, so this cannot fail
	}
	validate := cmd.Args
	cmd.Args = func(c *cobra.Command, args []string) error {
		if c.Flags().Changed(name) {
			return Usage(errors.New(msg))
		}
		if validate != nil {
			return validate(c, args)
		}
		return nil
	}
}

// ErrJSONFlagRemoved is what a run passing 1.x's --json is told. That flag
// spelled --output json on the commands that had it (`astro api … ls` and
// `describe`, and the `list` commands, which cmd's removedV1Flags covers);
// v2 has -o everywhere instead. Delete it in v3.
const ErrJSONFlagRemoved = "--json was removed in Astro CLI v2: use -o json"
