package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/deployment/workerqueue"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	label                         string
	runtimeVersion                string
	deploymentID                  string
	forceDelete                   bool
	description                   string
	clusterID                     string
	schedulerAU                   int
	schedulerReplicas             int
	workerAU                      int
	updateWorkerAU                int
	updateSchedulerReplicas       int
	updateSchedulerAU             int
	forceUpdate                   bool
	allDeployments                bool
	warnLogs                      bool
	errorLogs                     bool
	infoLogs                      bool
	waitForStatus                 bool
	logCount                      = 500
	variableKey                   string
	variableValue                 string
	useEnvFile                    bool
	makeSecret                    bool
	deploymentVariableListExample = `
		# List a deployment's variables
		$ astro deployment variable list --deployment-id <deployment-id> --key FOO
		# List a deployment's variables and save them to a file
		$ astro deployment variable list  --deployment-id <deployment-id> --save --env .env.my-deployment
		`
	deploymentVariableCreateExample = `
		# Create a deployment variable
		$ astro deployment variable create FOO=BAR FOO2=BAR2 --deployment-id <deployment-id> --secret
		# Create a deployment variables from a file
		$ astro deployment variable create --deployment-id <deployment-id> --load --env .env.my-deployment
		`
	deploymentVariableUpdateExample = `
		# Update a deployment variable
		$ astro deployment variable update FOO=NEWBAR FOO2=NEWBAR2 --deployment-id <deployment-id> --secret
		# Update a deployment variables from a file
		$ astro deployment variable update --deployment-id <deployment-id> --load --env .env.my-deployment
		`
	wQueueConcurrency int
	wQueueMin         int
	wQueueMax         int
	wQueueWorkerType  string
	wQueueName        string
	isDefaultWQueue   bool

	httpClient = httputil.NewHTTPClient()
)

func newDeploymentRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de"},
		Short:   "Manage your Deployments running on Astronomer",
		Long:    "Create or manage Deployments running on Astro according to your Organization and Workspace permissions.",
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	cmd.AddCommand(
		newDeploymentListCmd(out),
		newDeploymentDeleteCmd(),
		newDeploymentCreateCmd(),
		newDeploymentLogsCmd(),
		newDeploymentUpdateCmd(),
		newDeploymentVariableRootCmd(out),
		newDeploymentWorkerQueueRootCmd(out),
	)
	return cmd
}

func newDeploymentListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Deployments running in your Astronomer Workspace",
		Long:    "List all Deployments running in your Astronomer Workspace. Switch Workspaces to see other Deployments in your Organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentList(cmd, out)
		},
	}
	cmd.Flags().BoolVarP(&allDeployments, "all", "a", false, "Show deployments across all workspaces")
	return cmd
}

func newDeploymentLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs [Deployment-ID]",
		Aliases: []string{"l"},
		Short:   "Show an Astro Deployment's Scheduler logs",
		Long:    "Show an Astro Deployment's Scheduler logs. Use flags to determine what log level to show.",
		RunE:    deploymentLogs,
	}
	cmd.Flags().BoolVarP(&warnLogs, "warn", "w", false, "Show logs with a log level of 'warning'")
	cmd.Flags().BoolVarP(&errorLogs, "error", "e", false, "Show logs with a log level of 'error'")
	cmd.Flags().BoolVarP(&infoLogs, "info", "i", false, "Show logs with a log level of 'info'")
	cmd.Flags().IntVarP(&logCount, "log-count", "c", logCount, "Number of logs to show")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to show logs of")
	return cmd
}

func newDeploymentCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a new Astro Deployment",
		Long:    "Create a new Astro Deployment. All flags are optional",
		RunE:    deploymentCreate,
	}
	cmd.Flags().StringVarP(&label, "name", "n", "", "The Deployment's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "Workspace to create the Deployment in")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description of the Deployment. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&clusterID, "cluster-id", "c", "", "Cluster to create the Deployment in")
	cmd.Flags().StringVarP(&runtimeVersion, "runtime-version", "v", "", "Runtime version for the Deployment")
	cmd.Flags().IntVarP(&schedulerAU, "scheduler-au", "s", deployment.SchedulerAuMin, "The Deployment's Scheduler resources in AUs")
	cmd.Flags().IntVarP(&schedulerReplicas, "scheduler-replicas", "r", deployment.SchedulerReplicasMin, "The number of Scheduler replicas for the Deployment")
	cmd.Flags().IntVarP(&workerAU, "worker-au", "a", deployment.WorkerAuMin, "The Deployment's Worker resources in AUs")
	cmd.Flags().BoolVarP(&waitForStatus, "wait", "i", false, "Wait for the Deployment to become healthy before ending the command")
	return cmd
}

func newDeploymentUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [DEPLOYMENT-ID]",
		Aliases: []string{"up"},
		Short:   "Update an Astro Deployment",
		Long:    "Update the configuration for an Astro Deployment. All flags are optional",
		RunE:    deploymentUpdate,
	}
	cmd.Flags().StringVarP(&label, "name", "n", "", "Update the Deployment's name. If the new name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "Workspace the Deployment is located in")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description of the Deployment. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().IntVarP(&updateSchedulerAU, "scheduler-au", "s", 0, "The Deployment's Scheduler resources in AUs")
	cmd.Flags().IntVarP(&updateSchedulerReplicas, "scheduler-replicas", "r", 0, "The number of Scheduler replicas for the Deployment")
	cmd.Flags().IntVarP(&updateWorkerAU, "worker-au", "a", 0, "The Deployment's Worker resources in AUs")
	cmd.Flags().BoolVarP(&forceUpdate, "force", "f", false, "Force update: Don't prompt a user before Deployment update")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment to update")
	return cmd
}

func newDeploymentDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete DEPLOYMENT-ID",
		Aliases: []string{"de"},
		Short:   "Delete an Astro Deployment",
		Long:    "Delete an Astro Deployment",
		RunE:    deploymentDelete,
	}
	cmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Force delete. Don't prompt a user before Deployment deletion")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to delete")
	return cmd
}

func newDeploymentVariableRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "variable",
		Aliases: []string{"var"},
		Short:   "Manage deployment variables",
		Long:    "Manage environment variables for an Astro Deployment. These variables can be used in DAGs or to customize your Airflow environment",
	}
	cmd.AddCommand(
		newDeploymentVariableListCmd(out),
		newDeploymentVariableCreateCmd(out),
		newDeploymentVariableUpdateCmd(out),
	)
	return cmd
}

func newDeploymentVariableListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List a Deployment's variables",
		Long:    "List the keys and values for a Deployment's variables and save them to an environment file",
		Args:    cobra.NoArgs,
		Example: deploymentVariableListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentVariableList(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "deployment to list variables for")
	cmd.Flags().StringVarP(&variableKey, "key", "k", "", "Specify a key to find a specifc variable")
	cmd.Flags().BoolVarP(&useEnvFile, "save", "s", false, "Save deployment variables to an environment file")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of the file to save environment variables to")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to list variables from")

	return cmd
}

// nolint:dupl
func newDeploymentVariableCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [key1=val1 key2=val2]",
		Short: "Create Deployment-level environment variables",
		Long:  "Create Deployment-level environment variables by supplying either a key and value or an environment file with a list of keys and values",
		// Args:    cobra.NoArgs,
		Example: deploymentVariableCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentVariableCreate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "Deployment assigned to variables")
	cmd.Flags().StringVarP(&variableKey, "key", "k", "", "Key for the new variable")
	cmd.Flags().StringVarP(&variableValue, "value", "v", "", "Value for the new variable")
	cmd.Flags().BoolVarP(&useEnvFile, "load", "l", false, "Create environment variables loaded from an environment file")
	cmd.Flags().BoolVarP(&makeSecret, "secret", "s", false, "Set the new environment variables as secrets")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file to load environment variables from")
	_ = cmd.Flags().MarkHidden("key")
	_ = cmd.Flags().MarkHidden("value")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to create variables from")

	return cmd
}

// nolint:dupl
func newDeploymentVariableUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [key1=update_val1 key2=update_val2]",
		Short:   "Update Deployment-level environment variables",
		Long:    "Update Deployment-level environment variables by supplying either a key and value or an environment file with a list of keys and values, variables that don't already exist will be created",
		Example: deploymentVariableUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentVariableUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "Deployment assigned to variables")
	cmd.Flags().StringVarP(&variableKey, "key", "k", "", "Key of the variable to update")
	cmd.Flags().StringVarP(&variableValue, "value", "v", "", "Value of the variable to update")
	cmd.Flags().BoolVarP(&useEnvFile, "load", "l", false, "Update environment variables loaded from an environment file")
	cmd.Flags().BoolVarP(&makeSecret, "secret", "s", false, "Set updated environment variables as secrets")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file to load environment variables to update from")
	_ = cmd.Flags().MarkHidden("key")
	_ = cmd.Flags().MarkHidden("value")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to update varibles from")

	return cmd
}

func deploymentList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Don't validate workspace if viewing all deployments
	if allDeployments {
		ws = ""
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.List(ws, allDeployments, astroClient, out)
}

func deploymentLogs(cmd *cobra.Command, args []string) error {
	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	return deployment.Logs(deploymentID, ws, deploymentName, warnLogs, errorLogs, infoLogs, logCount, astroClient)
}

func deploymentCreate(cmd *cobra.Command, args []string) error {
	// Find Workspace ID
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}
	workspaceID = ws

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get latest runtime version
	if runtimeVersion == "" {
		airflowVersionClient := airflowversions.NewClient(httpClient, false)
		runtimeVersion, err = airflowversions.GetDefaultImageTag(airflowVersionClient, "")
		if err != nil {
			return err
		}
	}
	return deployment.Create(label, workspaceID, description, clusterID, runtimeVersion, schedulerAU, schedulerReplicas, workerAU, astroClient, waitForStatus)
}

func deploymentUpdate(cmd *cobra.Command, args []string) error {
	// Find Workspace ID
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	return deployment.Update(deploymentID, label, ws, description, deploymentName, updateSchedulerAU, updateSchedulerReplicas, updateWorkerAU, forceUpdate, astroClient)
}

func deploymentDelete(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	return deployment.Delete(deploymentID, ws, deploymentName, forceDelete, astroClient)
}

func deploymentVariableList(cmd *cobra.Command, _ []string, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.VariableList(deploymentID, variableKey, ws, envFile, deploymentName, useEnvFile, astroClient, out)
}

func deploymentVariableCreate(cmd *cobra.Command, args []string, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	variableList := args

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.VariableModify(deploymentID, variableKey, variableValue, ws, envFile, deploymentName, variableList, useEnvFile, makeSecret, false, astroClient, out)
}

func deploymentVariableUpdate(cmd *cobra.Command, args []string, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	variableList := args

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.VariableModify(deploymentID, variableKey, variableValue, ws, envFile, deploymentName, variableList, useEnvFile, makeSecret, true, astroClient, out)
}

func newDeploymentWorkerQueueRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worker-queue ",
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
	cmd.Flags().StringVarP(&wQueueName, "name", "n", "", "The name of the worker queue. If the name contains a space, specify the entire name within quotes \"\" .")
	cmd.Flags().BoolVarP(&isDefaultWQueue, "isDefault", "", false, "Make this the default worker queue by passing in a t | f.")
	// TODO confirm the allowed input range and default value
	cmd.Flags().IntVarP(&wQueueMin, "min-count", "", 1, "The min worker count of the worker queue. Possible values are between 1 and 10.")
	// TODO confirm the allowed input range and default value
	cmd.Flags().IntVarP(&wQueueMax, "max-count", "", 1, "The max worker count of the worker queue. Possible values are between 11 and 20.")
	// TODO confirm the allowed input range and default value
	cmd.Flags().IntVarP(&wQueueConcurrency, "concurrency", "", 1, "The concurrency(number of slots) of the worker queue. Possible values are between 21 and 30.")
	cmd.Flags().StringVarP(&wQueueWorkerType, "worker-type", "t", "", "The worker type of the default worker queue.")

	return cmd
}

func deploymentWorkerQueueCreate(cmd *cobra.Command, _ []string, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// TODO are wQueueName and isDefaultWQueue required?
	if deploymentID == "" {
		return errors.New("deployment id is required")
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workerqueue.Create(ws, deploymentID, wQueueName, isDefaultWQueue, astroClient, out)
}
