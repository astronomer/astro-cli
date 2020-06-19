package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/houston"
	"github.com/spf13/cobra"
)

func newClusterRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"cl"},
		Short:   "Manage Astronomer EE clusters",
		Long:    "Clusters represent a single installation of the Astronomer Enterprise platform",
	}
	cmd.AddCommand(
		newClusterListCmd(client, out),
		newClusterSwitchCmd(client, out),
	)
	return cmd
}

func newClusterListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List known Astronomer Enterprise clusters",
		Long:    "List known Astronomer Enterprise clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterList(cmd, args, out)
		},
	}
	return cmd
}

func newClusterSwitchCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch",
		Aliases: []string{"sw"},
		Short:   "Switch to a different cluster context",
		Long:    "Switch to a different cluster context",
		RunE:    clusterSwitch,
		Args:    cobra.MaximumNArgs(1),
	}
	return cmd
}

func clusterList(cmd *cobra.Command, args []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return cluster.List(out)
}

func clusterSwitch(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	d := ""
	if len(args) == 1 {
		d = args[0]
	}

	return cluster.Switch(d)
}
