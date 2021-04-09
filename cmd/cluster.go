package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/astrohub"
	"github.com/spf13/cobra"
)

func newClusterRootCmd(client *astrohub.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"cl"},
		Short:   "Manage Astronomer Clusters",
		Long:    "Clusters represent a single installation of the Astronomer Enterprise platform",
	}
	cmd.AddCommand(
		newClusterListCmd(client, out),
		newClusterSwitchCmd(client, out),
	)
	return cmd
}

func newClusterListCmd(client *astrohub.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List known Astronomer Clusters",
		Long:    "List known Astronomer Clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterList(cmd, args, out)
		},
	}
	return cmd
}

func newClusterSwitchCmd(client *astrohub.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Cluster context",
		Long:    "Switch to a different Cluster context",
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
