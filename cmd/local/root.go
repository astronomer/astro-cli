package local

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the whole v2 command surface on one root: the `astro
// local` tree, `astro init`, the root aliases, and the `astro dev` stub.
// cmd/astro/main.go executes it today; final wiring registers the same
// subcommands on the v1 root (outside its IsCloudContext branch) once the
// engine exists.
func NewRootCmd(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "astro",
		Short:         "Run Apache Airflow locally and interact with Astronomer",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(AddCmds(d)...)
	return root
}

// AddCmds returns every top-level v2 command, ready to register on a root.
// Each carries the skip-pre-run annotation on all its leaves, so the v1
// root's pre-run (config load, telemetry, network) never runs for them:
// `astro local` works offline with no account.
func AddCmds(d Deps) []*cobra.Command {
	cmds := []*cobra.Command{
		NewLocalCmd(d),
		NewInitCmd(d),
		NewDevCmd(d),
	}
	return append(cmds, rootAliasCmds(d)...)
}

// rootAliasCmds builds `astro start`, `astro stop`, and `astro logs` as
// their own command instances (a cobra command has one parent) over the
// same run functions as the `astro local` spellings.
func rootAliasCmds(d Deps) []*cobra.Command {
	builders := []func(*cli) *cobra.Command{newStartCmd, newStopCmd, newLogsCmd}
	cmds := make([]*cobra.Command, 0, len(builders))
	for _, build := range builders {
		c := &cli{d: d}
		cmd := build(c)
		cmd.Short += " (alias for `astro local " + cmd.Name() + "`)"
		addOutputFlag(cmd, &c.output)
		markSkipPreRun(cmd)
		cmds = append(cmds, cmd)
	}
	return cmds
}
