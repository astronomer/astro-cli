package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	allDeployments bool
	executor       string
	CreateExample  = `
# Create new deployment with Celery executor (default: celery without params).
astro deployment create new-deployment-name --executor=celery

# Create new deployment with Local executor.
astro deployment create new-deployment-name-local --executor=local

# Create new deployment with Kubernetes executor.
astro deployment create new-deployment-name-k8s --executor=k8s
`
	deploymentUpdateAttrs = []string{"label"}
)

func newDeploymentRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de"},
		Short:   "Manage airflow deployments",
		Long:    "Deployments are individual Airflow clusters running on an installation of the Astronomer platform.",
	}
	cmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
	cmd.AddCommand(
		newDeploymentCreateCmd(client, out),
		newDeploymentListCmd(client, out),
		newDeploymentUpdateCmd(client, out),
		newDeploymentDeleteCmd(client, out),
		newLogsCmd(client, out),
	)
	return cmd
}

func newDeploymentCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create DEPLOYMENT",
		Aliases: []string{"cr"},
		Short:   "Create a new Astronomer Deployment",
		Long:    "Create a new Astronomer Deployment",
		Example: CreateExample,
		Args:    cobra.ExactArgs(1),
		RunE:    deploymentCreate,
	}
	cmd.Flags().StringVarP(&executor, "executor", "e", "", "Add executor parameter: local or celery")
	return cmd
}

func newDeploymentDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete DEPLOYMENT",
		Aliases: []string{"de"},
		Short:   "Delete an airflow deployment",
		Long:    "Delete an airflow deployment",
		Args:    cobra.ExactArgs(1),
		RunE:    deploymentDelete,
	}
	return cmd
}

func newDeploymentListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List airflow deployments",
		Long:    "List airflow deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentList(cmd, args, client, out)
		},
	}
	cmd.Flags().BoolVarP(&allDeployments, "all", "a", false, "Show deployments across all workspaces")
	return cmd
}

func newDeploymentUpdateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
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
	return cmd
}

func deploymentCreate(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	deploymentConfig := make(map[string]string)
	switch executor {
	case "local":
		deploymentConfig["executor"] = "LocalExecutor"
	case "celery":
		deploymentConfig["executor"] = "CeleryExecutor"
	case "kubernetes", "k8s":
		deploymentConfig["executor"] = "KubernetesExecutor"
	}

	return deployment.Create(args[0], ws, deploymentConfig)
}

func deploymentDelete(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.Delete(args[0])
}

func deploymentList(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
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

	return deployment.List(ws, allDeployments, client, out)
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
