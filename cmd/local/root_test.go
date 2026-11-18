package local

import (
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/input"
)

// newRootCmd builds the whole core command surface on one standalone root: the
// `astro local` tree, `astro init`, the root aliases, and the `astro dev`
// stub. Production mounts these on the shell root through AddCmds (cmd/root.go);
// this root is the self-contained entry the package tests drive.
func newRootCmd(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "astro",
		Short:         "Run Apache Airflow locally and interact with Astronomer",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	input.AddFlag(root.PersistentFlags())
	root.AddCommand(AddCmds(d)...)
	return root
}
