package local

import (
	"github.com/spf13/cobra"
)

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
		NewPackageCmd(d),
		NewUseCmd(d),
		NewLinkCmd(d),
		newSuperviseCmd(d),
		newSessionWatchCmd(d),
		newProxyServeCmd(d),
	}
	// The query surface at the top level acts on a deployment. The same
	// commands, built by the same code, act on this machine under
	// `astro local af` (NewLocalCmd).
	cmds = append(cmds, newAfCmd(d, func() target { return &deploymentTarget{} }))
	cmds = append(cmds, rootAliasCmds(d)...)
	for _, cmd := range cmds {
		silenceUsage(cmd)
		wrapErrorOutput(d, cmd)
	}
	return cmds
}

// queryFamilies builds every Airflow-facing command family from one
// implementation. It is called twice — once at the top level against a
// deployment, once under `astro local` against this machine — and newTarget is
// the only thing that differs between the two: flags, service calls,
// rendering, and json rows are the same code either way.
//
// A new family goes in this list and lands on both surfaces at once, which is
// what keeps them from drifting into two half-implementations of one idea.
func queryFamilies(d Deps, newTarget func() target) []*cobra.Command {
	builders := []func(Deps, target) *cobra.Command{
		newDagsCmd,
		newRunsCmd,
		newTasksCmd,
		newAssetsCmd,
		newConnectionsCmd,
		newVariablesCmd,
		newPoolsCmd,
		newHealthCmd,
		newVersionCmd,
		newProvidersCmd,
		newPluginsCmd,
		newConfigCmd,
	}
	cmds := make([]*cobra.Command, 0, len(builders))
	for _, build := range builders {
		cmds = append(cmds, build(d, newTarget()))
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
		// The alias answers when typed but stays out of the root list, which
		// already carries `local` one line away.
		cmd.Hidden = true
		addOutputFlag(cmd, &c.output)
		markSkipPreRun(cmd)
		cmds = append(cmds, cmd)
	}
	return cmds
}
