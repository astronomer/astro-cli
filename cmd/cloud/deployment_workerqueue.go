package cloud

import (
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"
)

var (
	concurrency        int
	minWorkerCount     int
	maxWorkerCount     int
	workerType         string
	name               string
	force              bool
	errZeroConcurrency = errors.New("Worker concurrency cannot be 0. Minimum value starts from 1")
)

func newDeploymentWorkerQueueRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worker-queue",
		Aliases: []string{"wq", "worker-queues"},
		Short:   "Manage Deployment worker queues",
		Long:    "Manage worker queues for an Astro Deployment.",
	}
	cmd.AddCommand(
		newDeploymentWorkerQueueCreateCmd(out),
		newDeploymentWorkerQueueUpdateCmd(out),
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
			return deploymentWorkerQueueCreateOrUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be deleted.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be deleted.")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue. Queue names must not exceed 63 characters and contain only lowercase alphanumeric characters or '-' and start with an alphabetical character.")
	cmd.Flags().IntVarP(&minWorkerCount, "min-count", "", 0, "The min worker count of the worker queue.")
	cmd.Flags().IntVarP(&maxWorkerCount, "max-count", "", 0, "The max worker count of the worker queue.")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "", 0, "The concurrency(number of slots) of the worker queue.")
	cmd.Flags().StringVarP(&workerType, "worker-type", "t", "", "The worker type of the worker queue.")

	return cmd
}

func newDeploymentWorkerQueueUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update a Deployment's worker queue",
		Long:    "Update a worker queue for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueCreateOrUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be created.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be created.")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue. Queue names must not exceed 63 characters and contain only lowercase alphanumeric characters or '-' and start with an alphabetical character.")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force update: Don't prompt a user for confirmation")
	cmd.Flags().IntVarP(&minWorkerCount, "min-count", "", 0, "The min worker count of the worker queue.")
	cmd.Flags().IntVarP(&maxWorkerCount, "max-count", "", 0, "The max worker count of the worker queue.")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "", 0, "The concurrency(number of slots) of the worker queue.")
	cmd.Flags().StringVarP(&workerType, "worker-type", "t", "", "The worker type of the worker queue.")

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
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue to delete.")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force delete: Don't prompt a user for confirmation")
	return cmd
}

func deploymentWorkerQueueCreateOrUpdate(cmd *cobra.Command, _ []string, out io.Writer) error {
	cmd.SilenceUsage = true

	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if cmd.Flags().Changed("concurrency") && concurrency == 0 {
		return errZeroConcurrency
	}

	if minWorkerCount == 0 && !cmd.Flags().Changed("min-count") {
		minWorkerCount = -1
	}

	return workerqueue.CreateOrUpdate(ws, deploymentID, deploymentName, name, cmd.Name(), workerType, minWorkerCount, maxWorkerCount, concurrency, force, platformCoreClient, astroCoreClient, out)
}

func deploymentWorkerQueueDelete(cmd *cobra.Command, _ []string, out io.Writer) error {
	cmd.SilenceUsage = true

	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	return workerqueue.Delete(ws, deploymentID, deploymentName, name, force, platformCoreClient, astroCoreClient, out)
}
