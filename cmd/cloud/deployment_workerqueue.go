package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"
	"github.com/spf13/cobra"
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
	)
	return cmd
}

func newDeploymentWorkerQueueCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a Deployment's worker queue",
		Long:    "Create a worker queue for an Astro Deployment",
		Example: "", // TODO should we add some examples for default and custom worker-queue commands?
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueCreate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be created.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be created.")
	cmd.Flags().StringVarP(&wQueueName, "name", "n", "", "The name of the worker queue. Queue names must not exceed 63 characters and contain only lowercase alphanumeric characters or '-' and start with an alphabetical character.")
	cmd.Flags().IntVarP(&wQueueMinWorkerCount, "min-count", "", 0, "The min worker count of the worker queue. Possible values are between 1 and 10.")
	cmd.Flags().IntVarP(&wQueueMaxWorkerCount, "max-count", "", 0, "The max worker count of the worker queue. Possible values are between 11 and 20.")
	cmd.Flags().IntVarP(&wQueueConcurrency, "concurrency", "", 0, "The concurrency(number of slots) of the worker queue. Possible values are between 21 and 30.")
	cmd.Flags().StringVarP(&wQueueWorkerType, "worker-type", "t", "", "The worker type of the default worker queue.")

	return cmd
}

func deploymentWorkerQueueCreate(cmd *cobra.Command, _ []string, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workerqueue.Create(ws, deploymentID, deploymentName, wQueueName, wQueueMinWorkerCount, wQueueMaxWorkerCount, wQueueConcurrency, astroClient, out)
}
