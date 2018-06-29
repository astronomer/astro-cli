package cmd

import (
	"github.com/spf13/cobra"
)

var (
	deploymentRootCmd = &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de", "dep"},
		Short:   "Manage airflow deployments",
		Long:    "Manage airflow deployments",
	}

	deploymentCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new Astronomer Deployment",
		Long:  "Create a new Astronomer Deployment",
		RunE:  deploymentCreate,
	}

	deploymentListCmd = &cobra.Command{
		Use:   "list",
		Short: "List airflow deployments",
		Long:  "List airflow deployments",
		RunE:  deploymentList,
	}

	deploymentUpdateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update airflow deployments",
		Long:  "Update airflow deployments",
		RunE:  deploymentUpdate,
	}
)

func init() {
	// deployment root
	RootCmd.AddCommand(deploymentRootCmd)

	// deployment create
	deploymentRootCmd.AddCommand(deploymentCreateCmd)

	// deployment list
	deploymentRootCmd.AddCommand(deploymentListCmd)

	// deployment update
	deploymentRootCmd.AddCommand(deploymentUpdateCmd)
}

func deploymentCreate(cmd *cobra.Command, args []string) error {
	return nil
}

func deploymentList(cmd *cobra.Command, args []string) error {
	return nil
}

func deploymentUpdate(cmd *cobra.Command, args []string) error {
	return nil
}
