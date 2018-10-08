package cmd

import (
	"github.com/astronomerio/astro-cli/cluster"
	"github.com/spf13/cobra"
)

var (
	clusterRootCmd = &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"cl"},
		Short:   "Manage Astronomer EE clusters",
		Long:    "Clusters represent a single installation of the Astronomer Enterprise platform",
	}

	clusterListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List known Astronomer Enterprise clusters",
		Long:    "List known Astronomer Enterprise clusters",
		RunE:    clusterList,
	}

	clusterSwitchCmd = &cobra.Command{
		Use:     "switch",
		Aliases: []string{"sw"},
		Short:   "Switch to a different cluster context",
		Long:    "Switch to a different cluster context",
		RunE:    clusterSwitch,
		Args:    cobra.MaximumNArgs(1),
	}
)

func init() {
	// deployment root
	RootCmd.AddCommand(clusterRootCmd)

	clusterRootCmd.AddCommand(clusterListCmd)
	clusterRootCmd.AddCommand(clusterSwitchCmd)
}

func clusterList(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return cluster.List()
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
