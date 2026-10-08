package local

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// The group every Airflow-facing command family hangs under. `af` is the
// primary spelling — it is what these commands were called in the CLI they came
// from, and what fingers already type — and `airflow` is the alias, so a reader
// who never saw that CLI can spell it out and get the same tree.
const (
	afName  = "af"
	afAlias = "airflow"
)

// newAfCmd groups the query families under one node. It is called twice, with
// the two targets the families themselves take: at the top level the group acts
// on a deployment, under `astro local` it acts on this machine.
//
// The group earns its place on the family names. `connections` and `variables`
// are also what [tool.astro.env] declares and `astro local env` manages, and
// `health` could as easily be the platform's — so at the top level those three
// words each name two different things. Under `af` every one of them reads as
// the Airflow's own, and one `astro af --help` lists the whole surface that
// talks to an Airflow.
func newAfCmd(d Deps, newTarget func() target) *cobra.Command {
	cmd := &cobra.Command{
		Use:     afName,
		Aliases: []string{afAlias},
		// The Short says "Airflow" in both registrations, because a reader
		// scanning `astro --help` for the Airflow commands is the reader this
		// group was added for. Which Airflow is the Long's job.
		Short: "Talk to an Airflow: dags, runs, tasks, and more",
		Long: "Query and control an Airflow: its dags, runs, tasks, assets, connections, variables, pools, and health, " +
			"and its version, providers, plugins, and configuration.\n\n" +
			"These commands act on " + newTarget().which() + ".",
		Args: cobra.ArbitraryArgs,
		// A bare `astro af` prints help and succeeds; an unknown subcommand
		// fails. The same shape `astro local` and every family parent use:
		// without a RunE cobra treats a non-runnable parent as a help request
		// and exits 0 even on a typo.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cliout.GroupHelp(cmd, nil)
			}
			// This group answers to `airflow`, which used to name the local
			// project and became `astro dev` in 2019. An old invocation
			// arrives here and got told its subcommand was unknown for
			// `astro af` — a different thing, and no route onward.
			if replacement, ok := devReplacementFor(args[0]); ok {
				return cliout.Usage(fmt.Errorf("unknown command %q for %q. Local Airflow lives under `astro local`: use `%s`",
					args[0], cmd.CommandPath(), replacement))
			}
			return cliout.UnknownSubcommand(cmd, args[0])
		},
	}
	// The group renders no data of its own; the flag is here because every
	// runnable core command can reach one (TestTreeInvariants), and because
	// `astro af -o json` should not fail before it can print help. Each family
	// registers its own below, which shadows this one for everything under it.
	var output cliout.Format
	cliout.AddOutputFlag(cmd, &output)
	// The families carry the selector flags themselves, one target each. The
	// group deliberately registers none: a -d on this node would fill a target
	// no leaf reads, and the run would resolve as if nothing had been said.
	cmd.AddCommand(queryFamilies(d, newTarget)...)
	markSkipPreRun(cmd)
	return cmd
}
