package cmd

import (
	"github.com/astronomer/astro-cli/deployment"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	allDeployments bool
	executor       string

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
		Example: "  astro deployment update UUID label=Production-Airflow description=example version=v1.0.0",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) <= 0 {
				return errors.New("must specify a deployment ID and at least one attribute to update.")
			}
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
	deploymentCreateCmd.Flags().StringVarP(&executor, "executor", "e", "", "executor")
	deploymentRootCmd.AddCommand(deploymentCreateCmd)

	// deployment delete
	deploymentRootCmd.AddCommand(deploymentDeleteCmd)

	// deployment list
	deploymentRootCmd.AddCommand(deploymentListCmd)
	deploymentListCmd.Flags().BoolVarP(&allDeployments, "all", "a", false, "Show deployments across all workspaces")

	// deployment update
	deploymentRootCmd.AddCommand(deploymentUpdateCmd)

	// deployment logs
	deploymentRootCmd.AddCommand(logsCmd)
}

func deploymentCreate(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var executorValue string
	switch executor {
	case "local":
		executorValue = "LocalExecutor"
	case "celery":
		executorValue = "CeleryExecutor"
	case "kubernetes":
		executorValue = "KubernetesExecutor"
	default:
		executorValue = "CeleryExecutor"
	}

	return deployment.Create(args[0], ws, map[string]string{"executor": executorValue})
}

func deploymentDelete(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.Delete(args[0])
}

func deploymentList(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	// Don't validate workspace if viewing all deployments
	if allDeployments {
		ws = ""
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.List(ws, allDeployments)
}

func deploymentUpdate(cmd *cobra.Command, args []string) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.Update(args[0], argsMap)
}
