package registry

import (
	"github.com/spf13/cobra"
)

func newRegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"r"},
		Short:   "Interact with the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryDagCmd(),
		newRegistryProviderCmd(),
		newRegistryInitCmd(),
	)
	return cmd
}

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds() []*cobra.Command {
	return []*cobra.Command{
		newRegistryCmd(),
	}
}
