package cmd

import (
	"github.com/astronomerio/astro-cli/deployment"
	"github.com/spf13/cobra"
)

var (
	deploymentUpdateAttrs = []string{"label"}

	deploymentRootCmd = &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de"},
		Short:   "Manage airflow deployments",
		Long:    "Deployments are individual Airflow clusters running on an installation of the Astronomer platform.",
	}

	deploymentCreateCmd = &cobra.Command{
		Use:     "create DEPLOYMENT",
		Aliases: []string{"cr"},
		Short:   "Create a new Astronomer Deployment",
		Long:    "Create a new Astronomer Deployment",
		Args:    cobra.ExactArgs(1),
		RunE:    deploymentCreate,
	}

	deploymentDeleteCmd = &cobra.Command{
		Use:     "delete DEPLOYMENT",
		Aliases: []string{"de"},
		Short:   "Delete an airflow deployment",
		Long:    "Delete an airflow deployment",
		Args:    cobra.ExactArgs(1),
		RunE:    deploymentDelete,
	}

	deploymentListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List airflow deployments",
		Long:    "List airflow deployments",
		RunE:    deploymentList,
	}

	deploymentUpdateCmd = &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update airflow deployments",
		Long:    "Update airflow deployments",
		Args: func(cmd *cobra.Command, args []string) error {
			return updateArgValidator(args[1:], deploymentUpdateAttrs)
		},
		RunE: deploymentUpdate,
	}
)

func init() {
	// deployment root
	RootCmd.AddCommand(deploymentRootCmd)
	deploymentRootCmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
	// deploymentRootCmd.Flags().StringVar(&workspaceId, "workspace", "", "workspace assigned to deployment")

	// deployment create
	deploymentRootCmd.AddCommand(deploymentCreateCmd)

	// deployment delete
	deploymentRootCmd.AddCommand(deploymentDeleteCmd)

	// deployment list
	deploymentRootCmd.AddCommand(deploymentListCmd)

	// deployment update
	deploymentRootCmd.AddCommand(deploymentUpdateCmd)
}

func deploymentCreate(cmd *cobra.Command, args []string) error {
	ws := workspaceValidator()
	return deployment.Create(args[0], ws)
}

func deploymentDelete(cmd *cobra.Command, args []string) error {
	return deployment.Delete(args[0])
}

func deploymentList(cmd *cobra.Command, args []string) error {
	ws := workspaceValidator()
	return deployment.List(ws)
}

func deploymentUpdate(cmd *cobra.Command, args []string) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	return deployment.Update(args[0], argsMap)
}
