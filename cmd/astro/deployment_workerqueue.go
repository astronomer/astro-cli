package astro

import (
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/workerqueue"
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
	cliout.AddOutputFlag(cmd, &deploymentWorkerQueueOutput)
	return cmd
}

func newDeploymentWorkerQueueCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a Deployment's worker queue",
		Long:    "Create a worker queue for a Deployment. Worker queues let you assign tasks to different machine types with independent autoscaling. Each queue has its own min/max worker count and concurrency settings. KubernetesExecutor Deployments support only a single default queue. Queue names must be lowercase alphanumeric or hyphens, start with a letter, and not exceed 63 characters.",
		Example: `  # Create a worker queue
  astro deployment worker-queue create --deployment <DEPLOYMENT_ID> --name my-queue \
    --worker-type <WORKER_TYPE>

  # Create one that scales between 2 and 10 workers
  astro deployment worker-queue create --deployment <DEPLOYMENT_ID> --name my-queue \
    --min-count 2 --max-count 10 --concurrency 16`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueCreateOrUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be deleted. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be deleted.")
	addDeploymentFlag(cmd.Flags(), "Deployment that has the worker queue: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue: up to 63 lowercase letters, digits or '-', starting with a letter")
	cmd.Flags().IntVarP(&minWorkerCount, "min-count", "", 0, "The min worker count of the worker queue")
	cmd.Flags().IntVarP(&maxWorkerCount, "max-count", "", 0, "The max worker count of the worker queue")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "", 0, "The concurrency (number of slots) of the worker queue")
	cmd.Flags().StringVarP(&workerType, "worker-type", "t", "", "The worker type of the worker queue")

	return cmd
}

func newDeploymentWorkerQueueUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update a Deployment's worker queue",
		Long:    "Update a worker queue's machine type, scaling limits, or concurrency. Running tasks are not interrupted, but may be affected if the queue is scaled down below the current worker count. The default queue cannot be renamed.",
		Example: `  # Let a worker queue scale up to 20 workers
  astro deployment worker-queue update --deployment <DEPLOYMENT_ID> --name my-queue --max-count 20

  # Change its concurrency without asking for confirmation
  astro deployment worker-queue update --deployment <DEPLOYMENT_ID> --name my-queue \
    --concurrency 32 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueCreateOrUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be created. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be created.")
	addDeploymentFlag(cmd.Flags(), "Deployment that has the worker queue: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue: up to 63 lowercase letters, digits or '-', starting with a letter")
	cmd.Flags().BoolVarP(&force, "yes", "y", false, "Don't ask for confirmation, including after the warning about tasks assigned to the queue")
	cmd.Flags().IntVarP(&minWorkerCount, "min-count", "", 0, "The min worker count of the worker queue")
	cmd.Flags().IntVarP(&maxWorkerCount, "max-count", "", 0, "The max worker count of the worker queue")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "", 0, "The concurrency (number of slots) of the worker queue")
	cmd.Flags().StringVarP(&workerType, "worker-type", "t", "", "The worker type of the worker queue")

	return cmd
}

func newDeploymentWorkerQueueDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"de"},
		Short:   "Delete a Deployment's worker queue",
		Long:    "Delete a worker queue from a Deployment. Tasks currently running on the queue may be interrupted. The default queue cannot be deleted.",
		Example: `  # Delete a worker queue
  astro deployment worker-queue delete --deployment <DEPLOYMENT_ID> --name my-queue

  # Delete it without asking for confirmation
  astro deployment worker-queue delete --deployment <DEPLOYMENT_ID> --name my-queue --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentWorkerQueueDelete(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment where the worker queue should be created. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment where the worker queue should be created.")
	addDeploymentFlag(cmd.Flags(), "Deployment that has the worker queue: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the worker queue to delete")
	cmd.Flags().BoolVarP(&force, "yes", "y", false, "Don't ask for confirmation, including after the warning about tasks assigned to the queue")
	return cmd
}

func deploymentWorkerQueueCreateOrUpdate(cmd *cobra.Command, _ []string, out io.Writer) error {
	format := deploymentWorkerQueueOutput
	cmd.SilenceUsage = true
	// The Deployment picker and the update the change is sent as print to
	// bare stdout; under json that is a note, not the result.
	defer strayStdoutToStderr(format)()

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

	res, err := workerqueue.CreateOrUpdate(ws, deploymentID, deploymentName, name, cmd.Name(), workerType, minWorkerCount, maxWorkerCount, concurrency, force, astroV1Client, cliout.NotesTo(cmd, format, out))
	if err != nil || res == nil {
		// nil, nil is a declined update, which has said so.
		return err
	}
	return emitWorkerQueue(cliout.Renderer{Format: format, Out: out}, res, ws)
}

func deploymentWorkerQueueDelete(cmd *cobra.Command, _ []string, out io.Writer) error {
	format := deploymentWorkerQueueOutput
	cmd.SilenceUsage = true
	defer strayStdoutToStderr(format)()

	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	res, err := workerqueue.Delete(ws, deploymentID, deploymentName, name, force, astroV1Client, cliout.NotesTo(cmd, format, out))
	if err != nil || res == nil {
		// nil, nil is a declined deletion, which has said so.
		return err
	}
	return emitWorkerQueue(cliout.Renderer{Format: format, Out: out}, res, ws)
}
