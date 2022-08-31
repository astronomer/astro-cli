package cloud

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"
)

var (
	concurrency    int
	minWorkerCount int
	maxWorkerCount int
	workerType     string
	name           string
	force          bool
)

func newDeploymentWorkerQueueRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worker-queue",
		Aliases: []string{"wq"},
		Short:   "Manage deployment worker queues",
		Long:    "Manage worker queues for an Astro Deployment.",
	}
	cmd.AddCommand(
		newDeploymentWorkerQueueCreateCmd(out),
		newDeploymentWorkerQueueDeleteCmd(out),
	)
	return cmd
}

func newDeploymentWorkerQueueCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a Deployment's worker queue",
		Long:    "Create a worker queue for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueCreate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be deleted.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be deleted.")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue to delete.")
	cmd.Flags().IntVarP(&minWorkerCount, "min-count", "", 0, "The min worker count of the worker queue.")
	cmd.Flags().IntVarP(&maxWorkerCount, "max-count", "", 0, "The max worker count of the worker queue.")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "", 0, "The concurrency(number of slots) of the worker queue.")
	cmd.Flags().StringVarP(&workerType, "worker-type", "t", "", "The worker type of the default worker queue.")

	return cmd
}

func newDeploymentWorkerQueueDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"de"},
		Short:   "Delete a Deployment's worker queue",
		Long:    "Delete a worker queue from an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueDelete(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be created.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be created.")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue. Queue names must not exceed 63 characters and contain only lowercase alphanumeric characters or '-' and start with an alphabetical character.")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force delete: Don't prompt a user for confirmation")
	return cmd
}

func deploymentWorkerQueueCreate(cmd *cobra.Command, _ []string, out io.Writer) error {
	cmd.SilenceUsage = true

	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	return workerqueue.Create(ws, deploymentID, deploymentName, name, workerType, minWorkerCount, maxWorkerCount, concurrency, astroClient, out)
}

func deploymentWorkerQueueDelete(cmd *cobra.Command, _ []string, out io.Writer) error {
	cmd.SilenceUsage = true

	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}
	return workerqueue.Delete(ws, deploymentID, deploymentName, name, force, astroClient, out)
}
