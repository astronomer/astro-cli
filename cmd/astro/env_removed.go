package astro

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// removedVerbs are the write verbs v2 folded into `set`, with the short alias
// each one carried.
var removedVerbs = map[string]string{"create": "cr", "update": "up"}

// nounsWithFromFile are the two that take a dotenv file. A connection and a
// metrics export are described field-wise or as a whole value, so there is no
// bulk form to point a caller at.
var nounsWithFromFile = map[string]bool{"variable": true, "airflow-variable": true}

// newRemovedVerbCmd builds the tombstone for one write verb on one noun.
//
// v2 has a single write verb, `set`, which upserts. `create` and `update` are
// both gone, and both are tombstones rather than aliases of `set`:
//
//   - `create` aliased would keep the word working while inverting what it
//     does, quietly updating an existing object instead of failing. Its key
//     also moved from --key to the positional argument, so the invocation
//     breaks whatever we alias.
//   - `update` aliased would silently widen: for connection and metrics-export
//     it meant "fail if absent", and `set` creates. A job using it as an
//     existence check would start making objects with no signal. It survived
//     two reviews as an alias and both flagged it; dropping it is what makes
//     the one write verb actually one.
//
// A tombstone rather than nothing, because cobra answers an unknown
// subcommand by printing help and exiting 0 — so a stale `astro env variable
// update FOO` would look like success. Shape follows the `astro dev` removal
// stub in cmd/local/dev.go: hidden, so help teaches only what exists, and with
// flag parsing off, so an old invocation's --key or --strict reaches the
// guidance instead of dying on the flag.
func newRemovedVerbCmd(verb, noun string) *cobra.Command {
	var aliases []string
	if alias := removedVerbs[verb]; alias != "" {
		aliases = []string{alias}
	}
	return removedVerbStub(verb, aliases, func(args []string) string {
		return removedVerbGuidance(verb, noun, args)
	})
}

// removedVerbStub is the shape every tombstone shares; guidance supplies the
// words, which differ by verb and, for `link`, by how the object is addressed.
//
// Hidden, so help teaches only the surface that exists. Flag parsing off, so
// an old invocation's --key or --strict reaches the guidance instead of dying
// on the flag. The failure is a usage error, so under --output json it is the
// one error object every command publishes (removedCmdError).
func removedVerbStub(verb string, aliases []string, guidance func(args []string) string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                verb,
		Aliases:            aliases,
		Short:              "Removed in v2 — use `set`, which creates or updates",
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		// Overrides the env group's pre-run, which resolves the project and
		// checks the login: the guidance should need neither, and a script
		// on a machine that is logged out should still be told what to run.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(_ *cobra.Command, args []string) error {
			return removedCmdError(guidance(args))
		},
	}
	cliout.AddOutputFlag(cmd, new(cliout.Format))
	return cmd
}

// removedCmdError is how the env and deployment tombstones fail: a usage error, the kind
// cobra's unknown command is, so it exits 2 and, under --output json, is
// published as the error object with kind usage. With flag parsing off the
// stub's own --output is never set; cliout.Execute reads it from the raw
// arguments for a usage error, which is why the stub still declares one.
func removedCmdError(guidance string) error {
	return cliout.Usage(errors.New(guidance))
}

// removedVerbGuidance names the replacement for what was actually typed.
//
// The bulk form is worth telling apart: `--from-file` passed no key at all, so
// the general advice — that the key is now positional — is not just unhelpful
// there, it points at an invocation `set` rejects, since a positional and
// --from-file are mutually exclusive. Only for the nouns that have the flag,
// though: connection and metrics-export never took it, so offering it would
// trade a dead verb for a dead flag.
func removedVerbGuidance(verb, noun string, args []string) string {
	head := fmt.Sprintf("`astro env %s %s` was removed in Astro CLI v2.\n", noun, verb)

	if hasFromFileArg(args) && nounsWithFromFile[noun] {
		return head + fmt.Sprintf(
			"  use:  astro env %s set --from-file <file>\n"+
				"`set` creates each entry that does not exist and updates each one that does, "+
				"so it replaces both `create --from-file` and `update --from-file`. "+
				"Pass --no-create to fail on an entry that does not exist instead of creating it.",
			noun)
	}

	body := fmt.Sprintf("  use:  astro env %s set <id-or-key>\n", noun)
	if verb == "create" {
		body += "`set` creates the object when it does not exist and updates it when it does, " +
			"so it replaces both `create` and `update`. The key is now the positional argument " +
			"rather than --key. Pass --no-create to fail instead of creating."
	} else {
		body += "`set` is the same operation and also creates the object when it does not exist. " +
			"Pass --no-create for the old behavior of failing on a key that is not there."
		// --strict only ever existed on the two nouns whose update upserted,
		// so only their readers have a script passing it to rewrite.
		if nounsWithFromFile[noun] {
			body += " (--no-create replaces --strict.)"
		}
	}
	return head + body
}

// hasFromFileArg reports whether the raw args carry --from-file in either
// spelling. The stub parses no flags, so this reads them as written.
func hasFromFileArg(args []string) bool {
	for _, a := range args {
		if a == "--from-file" || strings.HasPrefix(a, "--from-file=") {
			return true
		}
	}
	return false
}
