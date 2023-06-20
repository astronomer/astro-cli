package registry

import (
	"io"

	"github.com/spf13/cobra"
)

func newRegistryCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"r"},
		Short:   "Interact with the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryDagCmd(out),
		newRegistryProviderCmd(out),
	)
	return cmd
}

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(out io.Writer) []*cobra.Command {
	return []*cobra.Command{
		newRegistryCmd(out),
	}
}
