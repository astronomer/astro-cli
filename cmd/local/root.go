package local

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the whole v2 command surface on one standalone root: the
// `astro local` tree, `astro init`, the root aliases, and the `astro dev`
// stub. Production mounts these on the v1 root through AddCmds (cmd/root.go);
// this root is the self-contained entry the package tests drive.
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
// `astro local` works offline with no account. Usage is silenced tree-wide so
// a failed command shows just its error; the error itself stays unsilenced,
// so whichever root mounts this tree prints it.
func AddCmds(d Deps) []*cobra.Command {
	cmds := []*cobra.Command{
		NewLocalCmd(d),
		NewInitCmd(d),
		NewDevCmd(d),
		newSuperviseCmd(d),
		newSessionWatchCmd(d),
	}
	cmds = append(cmds, rootAliasCmds(d)...)
	for _, cmd := range cmds {
		silenceUsage(cmd)
		wrapErrorOutput(d, cmd)
	}
	return cmds
}

// silenceUsage suppresses cobra's usage dump on error for cmd and every
// descendant. Errors are left to the runner to print.
func silenceUsage(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	for _, sub := range cmd.Commands() {
		silenceUsage(sub)
	}
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
