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
		Long:    "Clusteres represent a single installation of the Astronomer Enterprise platform",
	}

	clusterListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List known Astronomer Enterprise clusters",
		Long:    "List known Astronomer Enterprise clusters",
		RunE:    clusterList,
	}
)

func init() {
	// deployment root
	RootCmd.AddCommand(clusterRootCmd)

	// deployment create
	clusterRootCmd.AddCommand(clusterListCmd)

}

func clusterList(cmd *cobra.Command, args []string) error {
	cluster.ListClusters()
	return nil
}
