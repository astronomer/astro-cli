package cmd

import (
	"github.com/astronomerio/astro-cli/deployment"
	"github.com/spf13/cobra"
)

var (
	teamId string

	deploymentRootCmd = &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de"},
		Short:   "Manage airflow deployments",
		Long:    "Deployments are individual Airflow clusters running on an installation of the Astronomer platform.",
	}

	deploymentCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new Astronomer Deployment",
		Long:  "Create a new Astronomer Deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  deploymentCreate,
	}

	deploymentDeleteCmd = &cobra.Command{
		Use:   "delete",
		Short: "Delete an airflow deployment",
		Long:  "Delete an airflow deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  deploymentDelete,
	}

	deploymentListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List airflow deployments",
		Long:    "List airflow deployments",
		RunE:    deploymentList,
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
	deploymentCreateCmd.Flags().StringVarP(&teamId, "teamId", "t", "", "team id to associate with deployment")

	// deployment delete
	deploymentRootCmd.AddCommand(deploymentDeleteCmd)

	// deployment list
	deploymentRootCmd.AddCommand(deploymentListCmd)

	// deployment update
	deploymentRootCmd.AddCommand(deploymentUpdateCmd)
}

func deploymentCreate(cmd *cobra.Command, args []string) error {
	return deployment.Create(args[0], teamId)
}

func deploymentDelete(cmd *cobra.Command, args []string) error {
	return deployment.Delete(args[0])
}

func deploymentList(cmd *cobra.Command, args []string) error {
	return deployment.List()
}

func deploymentUpdate(cmd *cobra.Command, args []string) error {
	return nil
}
