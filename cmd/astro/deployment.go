package astro

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
)

const (
	enable    = "enable"
	disable   = "disable"
	standard  = "standard"
	dedicated = "dedicated"

	// --type also takes the deployment types the API names, as `deployment
	// inspect` prints them.
	hostedStandard  = "HOSTED_STANDARD"
	hostedShared    = "HOSTED_SHARED"
	hostedDedicated = "HOSTED_DEDICATED"

	deploymentWaitTime = 600 * time.Second
)

var (
	label                      string
	runtimeVersion             string
	deploymentID               string
	logsKeyword                string
	forceDelete                bool
	description                string
	clusterID                  string
	dagDeploy                  string
	schedulerAU                int
	schedulerReplicas          int
	updateSchedulerReplicas    int
	updateSchedulerAU          int
	forceUpdate                bool
	allDeployments             bool
	warnLogs                   bool
	errorLogs                  bool
	infoLogs                   bool
	waitForStatus              bool
	waitTimeForDeployment      time.Duration
	logCount                   = 500
	variableKey                string
	variableValue              string
	useEnvFile                 bool
	makeSecret                 bool
	executor                   string
	cloudProvider              string
	region                     string
	schedulerSize              string
	highAvailability           string
	developmentMode            string
	cicdEnforcement            string
	defaultTaskPodMemory       string
	resourceQuotaCPU           string
	resourceQuotaMemory        string
	defaultTaskPodCPU          string
	addDeploymentRole          string
	updateDeploymentRole       string
	workloadIdentity           string
	until                      string
	forDuration                string
	removeOverride             bool
	forceOverride              bool
	logApiserver               bool
	logWebserver               bool
	logScheduler               bool
	logWorkers                 bool
	logTriggerer               bool
	logDagProcessor            bool
	logComponents              []string
	flagRemoteExecutionEnabled bool
	flagAllowedIPAddressRanges string
	flagTaskLogBucket          string
	flagTaskLogURLPattern      string
	allowedIPAddressRanges     *[]string
	taskLogBucket              *string
	taskLogURLPattern          *string
	deploymentListOutput       string
	deploymentUserOutput       string
	deploymentTeamOutput       string

	deploymentType                = standard
	deploymentVariableListExample = `
		# List a deployment's variables
		$ astro deployment variable list --deployment <deployment-id> --key FOO
		# List a deployment's variables and save them to a file
		$ astro deployment variable list  --deployment <deployment-id> --save --env .env.my-deployment
		`
	deploymentVariableCreateExample = `
		# Create a deployment variable
		$ astro deployment variable create FOO=BAR FOO2=BAR2 --deployment <deployment-id> --secret
		# Create a deployment variables from a file
		$ astro deployment variable create --deployment <deployment-id> --load --env .env.my-deployment
		`
	deploymentVariableUpdateExample = `
		# Update a deployment variable
		$ astro deployment variable update FOO=NEWBAR FOO2=NEWBAR2 --deployment <deployment-id> --secret
		# Update a deployment variables from a file
		$ astro deployment variable update --deployment <deployment-id> --load --env .env.my-deployment
		`
	httpClient              = httputil.NewHTTPClient()
	errInvalidExecutor      = errors.New("not a valid executor")
	errInvalidCloudProvider = errors.New("not a valid cloud provider. It can only be gcp, azure or aws")
)

func newDeploymentRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de", "deployments"},
		Short:   "Manage your Deployments running on Astronomer",
		Long:    "Create or manage Deployments running on Astro according to your Organization and Workspace permissions.",
	}
	cmd.PersistentPreRunE = followProjectPreRun(cmd)
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment (default: the project's workspace inside a project with a pyproject.toml, else the current one)")
	addWorkspaceFlag(cmd.PersistentFlags(), "", "Workspace the Deployment is in (default: the project's workspace inside a project with a pyproject.toml, else the current one)")
	cmd.AddCommand(
		newDeploymentListCmd(out),
		newDeploymentDeleteCmd(out),
		newDeploymentCreateCmd(out),
		newDeploymentLogsCmd(out),
		newDeploymentUpdateCmd(out),
		newDeploymentVariableRootCmd(out),
		newDeploymentWorkerQueueRootCmd(out),
		newDeploymentInspectCmd(out),
		newDeploymentUserRootCmd(out),
		newDeploymentTeamRootCmd(out),
		newDeploymentTokenRootCmd(out),
		newDeploymentBundleRootCmd(out),
		newDeploymentHibernateCmd(out),
		newDeploymentWakeUpCmd(out),
	)
	cmd.AddCommand(newRemovedDeploymentObjectCmds()...)
	for _, c := range cmd.Commands() {
		switch c.Name() {
		case "inspect", "logs", "update", "delete", "hibernate", "wake-up":
			if c.Annotations == nil {
				c.Annotations = map[string]string{}
			}
			c.Annotations[deploymentArgAnnotation] = "true"
		}
	}
	applyPreferredFlagsIn(cmd)
	return cmd
}

func newDeploymentTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage teams in your Astro Deployment",
		Long:    "Manage teams in your Astro Deployment.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newDeploymentTeamListCmd(out),
		newDeploymentTeamUpdateCmd(out),
		newDeploymentTeamRemoveCmd(out),
		newDeploymentTeamAddCmd(out),
	)
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment where you'd like to manage teams. Run 'astro deployment list' to find valid IDs")
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment whose teams you'd like to manage: a Deployment id, or a link name from pyproject.toml. Run 'astro deployment list' to find valid IDs")
	cliout.AddOutputFlag(cmd, &deploymentTeamOutput)
	return cmd
}

func newDeploymentTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the teams in an Astro Deployment",
		Long:    "List all teams and their roles in a Deployment.",
		Example: `  astro deployment team list --deployment <deployment-id>
  astro deployment team list --deployment <id> -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listDeploymentTeam(cmd, out)
		},
	}
	return cmd
}

func newDeploymentTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a team from an Astro Deployment",
		Long:    "Remove a team's role from a Deployment. Team members lose Deployment access unless they have access through another team or a direct user role assignment.",
		Example: `
  $ astro deployment team remove <team-id> --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeDeploymentTeam(cmd, args, out)
		},
	}
	return cmd
}

func listDeploymentTeam(cmd *cobra.Command, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}

	format, err := cliout.ParseFormat(deploymentTeamOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return team.ListDeploymentTeamsWithFormat(astroV1Client, deploymentID, cliout.Renderer{Format: format, Out: out})
}

func removeDeploymentTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := cliout.ParseFormat(deploymentTeamOutput)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	r, err := team.RemoveDeploymentTeam(id, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &r, fmt.Sprintf("Astro Team %s was successfully removed from deployment %s", r.Name, r.DeploymentID))
}

func newDeploymentTeamAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [id]",
		Short: "Add a team to an Astro Deployment with a specific role",
		Long:  "Add a team to an Astro Deployment with a specific role\n$astro deployment team add [id] --role [DEPLOYMENT_ADMIN or the custom role name].",
		Example: `
  $ astro deployment team add <team-id> --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addDeploymentTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "w", "", "The Deployment's unique identifier. Run 'astro deployment list' to find valid IDs")
	addDeploymentFlag(cmd.Flags(), "Deployment to add the team to: a Deployment id, or a link name from pyproject.toml. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&addDeploymentRole, "role", "r", "DEPLOYMENT_ADMIN", "The role for the "+
		"new team. Possible values are DEPLOYMENT_ADMIN or the custom role name.")
	return cmd
}

func addDeploymentTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := cliout.ParseFormat(deploymentTeamOutput)
	if err != nil {
		return err
	}
	var id string

	if len(args) > 0 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	t, err := team.AddDeploymentTeam(id, addDeploymentRole, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &t, fmt.Sprintf("The team %s was successfully added to the deployment with the role %s", t.ID, t.DeploymentRole))
}

func newDeploymentTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [id]",
		Aliases: []string{"up"},
		Short:   "Update the role of a team in an Astro Deployment",
		Long:    "Update the role of a team in an Astro Deployment\n$astro deployment team update [id] --role [DEPLOYMENT_ADMIN or the custom role name].",
		Example: `
  $ astro deployment team update <team-id> --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateDeploymentTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateDeploymentRole, "role", "r", "", "The new role for the "+
		"team. Possible values are DEPLOYMENT_ADMIN or the custom role name.")
	return cmd
}

func updateDeploymentTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := cliout.ParseFormat(deploymentTeamOutput)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	if updateDeploymentRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a Deployment role or custom role name to update team: ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		updateDeploymentRole = answer
	}

	cmd.SilenceUsage = true
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	t, err := team.UpdateDeploymentTeamRole(id, updateDeploymentRole, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &t, fmt.Sprintf("The deployment team %s role was successfully updated to %s", t.ID, t.DeploymentRole))
}

func newDeploymentUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us", "users"},
		Short:   "Manage users in your Astro Deployment",
		Long:    "Manage users in your Astro Deployment.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newDeploymentUserListCmd(out),
		newDeploymentUserUpdateCmd(out),
		newDeploymentUserRemoveCmd(out),
		newDeploymentUserAddCmd(out),
	)
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment where you'd like to manage users. Run 'astro deployment list' to find valid IDs")
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment whose users you'd like to manage: a Deployment id, or a link name from pyproject.toml. Run 'astro deployment list' to find valid IDs")
	cliout.AddOutputFlag(cmd, &deploymentUserOutput)
	return cmd
}

func newDeploymentUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [email]",
		Short: "Add a user to an Astro Deployment with a specific role",
		Long:  "Add a user to an Astro Deployment with a specific role\n$astro deployment user add [email] --role [DEPLOYMENT_ADMIN or the custom role name].",
		Example: `
  $ astro deployment user add user@company.com --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addDeploymentUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&addDeploymentRole, "role", "r", "DEPLOYMENT_ADMIN", "The role for the "+
		"new user. Possible values are DEPLOYMENT_ADMIN or the custom role name.")
	return cmd
}

func newDeploymentUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the users in an Astro Deployment",
		Long:    "List all users and their roles in a Deployment.",
		Example: `  astro deployment user list --deployment <deployment-id>
  astro deployment user list --deployment <id> -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listDeploymentUser(cmd, out)
		},
	}
	return cmd
}

func newDeploymentUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [email]",
		Aliases: []string{"up"},
		Short:   "Update the role of a user in an Astro Deployment",
		Long:    "Update the role of a user in an Astro Deployment\n$astro deployment user update [email] --role [DEPLOYMENT_ADMIN or the custom role name].",
		Example: `
  $ astro deployment user update user@company.com --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateDeploymentUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateDeploymentRole, "role", "r", "", "The new role for the "+
		"user. Possible values are DEPLOYMENT_ADMIN or the custom role name.")
	return cmd
}

func newDeploymentUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a user from an Astro Deployment",
		Long:    "Remove a user's direct role from a Deployment. The user may retain access through team membership.",
		Example: `
  $ astro deployment user remove user@company.com --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeDeploymentUser(cmd, args, out)
		},
	}
	return cmd
}

func newDeploymentListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Deployments running in your Astronomer Workspace",
		Long:    "List all Deployments running in your Astronomer Workspace. Switch Workspaces to see other Deployments in your Organization.",
		Example: `  astro deployment list
  astro deployment list --all
  astro deployment list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentList(cmd, out)
		},
	}
	cmd.Flags().BoolVarP(&allDeployments, "all", "a", false, "Show deployments across all workspaces")
	cliout.AddOutputFlag(cmd, &deploymentListOutput)
	addJSONFlag(cmd)
	return cmd
}

func newDeploymentLogsCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs [Deployment-ID]",
		Aliases: []string{"l"},
		Short:   "Show an Astro Deployment's Scheduler logs",
		Long:    "Show an Astro Deployment's Scheduler logs. Use flags to determine what log level to show.",
		Example: `
  $ astro deployment logs <deployment-id>
  $ astro deployment logs <deployment-id> --error --info --log-count 50
  $ astro deployment logs --deployment my-deployment --keyword "task failed"
`,
		RunE: func(cmd *cobra.Command, args []string) error { return deploymentLogs(cmd, args, out) },
	}
	cmd.Flags().BoolVarP(&warnLogs, "warn", "w", false, "Show logs with a log level of 'warning'")
	cmd.Flags().BoolVarP(&errorLogs, "error", "e", false, "Show logs with a log level of 'error'")
	cmd.Flags().BoolVarP(&infoLogs, "info", "i", false, "Show logs with a log level of 'info'")
	cmd.Flags().StringVarP(&logsKeyword, "keyword", "k", "", "Show logs that contain this exact keyword or phrase.")
	cmd.Flags().IntVarP(&logCount, "log-count", "c", logCount, "Number of logs to show")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to show logs of")
	addDeploymentFlag(cmd.Flags(), "Deployment to show logs of: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().BoolVar(&logWebserver, "webserver", false, "Show logs from the webserver")
	cmd.Flags().BoolVar(&logApiserver, "apiserver", false, "Show logs from the api server")
	cmd.Flags().BoolVar(&logScheduler, "scheduler", false, "Show logs from the scheduler")
	cmd.Flags().BoolVar(&logWorkers, "workers", false, "Show logs from the workers")
	cmd.Flags().BoolVar(&logTriggerer, "triggerer", false, "Show logs from the triggerer")
	cmd.Flags().BoolVar(&logDagProcessor, "dag-processor", false, "Show logs from the DAG processor")
	cmd.Flags().StringSliceVar(&logComponents, "component", nil, "Show logs from a component by name (repeatable or comma-separated). Alternative to passing individual flags like --scheduler or --triggerer.")
	cliout.AddOutputFlag(cmd, &deploymentLogsOutput)
	return cmd
}

func newDeploymentCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a new Astro Deployment",
		Long:    "Create a new Deployment — an Airflow environment running on Astro. Configurable options include the executor type (Celery, Kubernetes, Astro), runtime version, worker queues, cloud provider, and region. On hosted Astro, a Deployment can be standard (shared infrastructure) or dedicated (isolated cluster). Use --clone to copy an existing Deployment. Use --wait to block until the Deployment is healthy.",
		Example: deploymentCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentCreate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&label, "name", "n", "", "The Deployment's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace to create the Deployment in")
	addWorkspaceFlag(cmd.Flags(), "w", "Workspace to create the Deployment in")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description of the Deployment. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&runtimeVersion, "runtime-version", "v", "", "Runtime version for the Deployment")
	cmd.Flags().StringVarP(&dagDeploy, "dag-deploy", "", "", "Enables DAG-only deploys for the Deployment")
	cmd.Flags().StringVarP(&executor, "executor", "e", "CeleryExecutor", "The executor to use for the Deployment. Possible values can be CeleryExecutor, KubernetesExecutor, or AstroExecutor.")
	cmd.Flags().StringVarP(&cicdEnforcement, "cicd-enforcement", "", "", "When enabled CI/CD Enforcement where deploys to deployment must use an API Key or Token. This essentially forces Deploys to happen through CI/CD. Possible values disable/enable")
	cmd.Flags().StringVar(&cloneSource, "clone", "", cloneFlagUsage)
	cmd.Flags().BoolVarP(&waitForStatus, "wait", "i", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVar(&waitTimeForDeployment, "wait-time", deploymentWaitTime, "Wait time for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cmd.Flags().BoolVarP(&cleanOutput, "clean-output", "", false, "clean output to only include inspect yaml or json file in any situation.")
	cmd.Flags().StringVarP(&workloadIdentity, "workload-identity", "", "", "The Workload Identity to use for the Deployment")
	if organization.IsOrgHosted() {
		cmd.Flags().StringVarP(&deploymentType, "type", "", standard, "The Type to use for the Deployment. Possible values can be standard or dedicated.")
		cmd.Flags().StringVarP(&defaultTaskPodCPU, "default-task-pod-cpu", "", "", "The default task pod CPU to use for the Deployment. Example value: 0.25")
		cmd.Flags().StringVarP(&defaultTaskPodMemory, "default-task-pod-memory", "", "", "The default task pod memory to use for the Deployment. Example value: 0.5Gi")
		cmd.Flags().StringVarP(&resourceQuotaCPU, "resource-quota-cpu", "", "", "The Deployment's CPU resource quota. Example value: 10")
		cmd.Flags().StringVarP(&resourceQuotaMemory, "resource-quota-memory", "", "", "The Deployment's memory resource quota. Example value: 20Gi")
		cmd.Flags().StringVarP(&cloudProvider, "cloud-provider", "p", "azure", "The Cloud Provider to use for the Deployment. Possible values can be gcp, aws, azure.")
		cmd.Flags().StringVarP(&region, "region", "", "", "The Cloud Provider region to use for the Deployment.")
		cmd.Flags().StringVarP(&schedulerSize, "scheduler-size", "", "", "The size of scheduler for the Deployment. Possible values can be small, medium, large, extra_large")
		cmd.Flags().StringVarP(&highAvailability, "high-availability", "a", "disable", "Enables High Availability for the Deployment")
		cmd.Flags().StringVarP(&developmentMode, "development-mode", "m", "disable", "Set to 'enable' to enable development-only features such as hibernation. When enabled, the Deployment will not have guaranteed uptime SLAs.'")
		cmd.Flags().BoolVarP(&flagRemoteExecutionEnabled, "remote-execution-enabled", "", false, "Enables Remote Execution for the Deployment.")
		cmd.Flags().StringVarP(&flagAllowedIPAddressRanges, "allowed-ip-address-ranges", "", "", "A comma-separated list of allowed IP address ranges for the Deployment. By default, there's no IP restriction. Example: 203.0.113.0/24,198.51.100.42/32")
		cmd.Flags().StringVarP(&flagTaskLogBucket, "task-log-bucket", "", "", "The bucket to use for storing task logs. Example: s3://my-bucket/airflow-logs")
		cmd.Flags().StringVarP(&flagTaskLogURLPattern, "task-log-url-pattern", "", "", "The URL pattern to use for accessing task logs. Example: dag_id={{ ti.dag_id }}/run_id={{ ti.run_id }}/task_id={{ ti.task_id }}/{% if ti.map_index >= 0 %}map_index={{ ti.map_index }}/{% endif %}attempt={{ try_number|default(ti.try_number) }}/{{ ti.id }}.log")
	} else {
		cmd.Flags().IntVarP(&schedulerAU, "scheduler-au", "s", 0, "The Deployment's scheduler resources in AUs")
		cmd.Flags().IntVarP(&schedulerReplicas, "scheduler-replicas", "r", 0, "The number of scheduler replicas for the Deployment")
	}
	cmd.Flags().StringVarP(&clusterID, "cluster-id", "c", "", "Cluster to create the Deployment in. Run \"astro organization cluster list\" to see your Organization's cluster IDs")
	cmd.Flags().BoolVarP(&forceUpdate, "yes", "y", false, "Don't ask for confirmation, including after a warning about the Deployment's CI/CD enforcement")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	addRemovedFlag(cmd, "deployment-file", "", false, errDeploymentFileRemoved)
	return cmd
}

func newDeploymentUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [DEPLOYMENT-ID]",
		Aliases: []string{"up"},
		Short:   "Update an Astro Deployment",
		Long:    "Update the configuration for an Astro Deployment. All flags are optional",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&label, "name", "n", "", "Update the Deployment's name. If the new name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace the Deployment is located in")
	addWorkspaceFlag(cmd.Flags(), "w", "Workspace the Deployment is located in")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Description of the Deployment. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&executor, "executor", "e", "", "The executor to use for the deployment. Possible values can be CeleryExecutor, KubernetesExecutor or AstroExecutor.")
	cmd.Flags().BoolVarP(&forceUpdate, "yes", "y", false, "Don't ask for confirmation, including after a warning about the Deployment's CI/CD enforcement")
	cmd.Flags().StringVarP(&cicdEnforcement, "cicd-enforcement", "", "", "When enabled CI/CD Enforcement where deploys to deployment must use an API Key or Token. This essentially forces Deploys to happen through CI/CD. Possible values disable/enable.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "", "", "Name of the deployment to update")
	addDeploymentFlag(cmd.Flags(), "Deployment to update: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&dagDeploy, "dag-deploy", "", "", "Enables DAG-only deploys for the deployment")
	cmd.Flags().BoolVarP(&cleanOutput, "clean-output", "c", false, "clean output to only include inspect yaml or json file in any situation.")
	cmd.Flags().StringVarP(&workloadIdentity, "workload-identity", "", "", "The Workload Identity to use for the Deployment")
	if organization.IsOrgHosted() {
		cmd.Flags().StringVarP(&schedulerSize, "scheduler-size", "", "", "The size of Scheduler for the Deployment. Possible values can be small, medium, large, extra_large")
		cmd.Flags().StringVarP(&highAvailability, "high-availability", "a", "", "Enables High Availability for the Deployment")
		cmd.Flags().StringVarP(&defaultTaskPodCPU, "default-task-pod-cpu", "", "", "The Default Task Pod CPU to use for the Deployment. Example value: 0.25")
		cmd.Flags().StringVarP(&defaultTaskPodMemory, "default-task-pod-memory", "", "", "The Default Taks Pod Memory to use for the Deployment. Example value: 0.5Gi")
		cmd.Flags().StringVarP(&resourceQuotaCPU, "resource-quota-cpu", "", "", "The Resource Quota CPU to use for the Deployment. Example value: 10")
		cmd.Flags().StringVarP(&resourceQuotaMemory, "resource-quota-memory", "", "", "The Resource Quota Memory to use for the Deployment. Example value: 20Gi")
		cmd.Flags().StringVarP(&developmentMode, "development-mode", "m", "", "Whether the Deployment is for development only. If 'disable', the Deployment can be considered production for the purposes of support case priority, but development-only features such as hibernation will not be available. You can't update this value to `enable` for existing non-development Deployments.'")
		cmd.Flags().StringVarP(&flagAllowedIPAddressRanges, "allowed-ip-address-ranges", "", "", "A comma-separated list of allowed IP address ranges for the Deployment. By default, there's no IP restriction. Example: 203.0.113.0/24,198.51.100.42/32")
		cmd.Flags().StringVarP(&flagTaskLogBucket, "task-log-bucket", "", "", "The bucket to use for storing task logs. Example: s3://my-bucket/airflow-logs")
		cmd.Flags().StringVarP(&flagTaskLogURLPattern, "task-log-url-pattern", "", "", "The URL pattern to use for accessing task logs. Example: dag_id={{ ti.dag_id }}/run_id={{ ti.run_id }}/task_id={{ ti.task_id }}/{% if ti.map_index >= 0 %}map_index={{ ti.map_index }}/{% endif %}attempt={{ try_number|default(ti.try_number) }}/{{ ti.id }}.log")
	} else {
		cmd.Flags().IntVarP(&updateSchedulerAU, "scheduler-au", "s", 0, "The Deployment's Scheduler resources in AUs.")
		cmd.Flags().IntVarP(&updateSchedulerReplicas, "scheduler-replicas", "r", 0, "The number of Scheduler replicas for the Deployment.")
	}
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	addRemovedFlag(cmd, "deployment-file", "", false, errDeploymentFileRemoved)
	return cmd
}

func newDeploymentDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete DEPLOYMENT-ID",
		Aliases: []string{"de"},
		Short:   "Delete an Astro Deployment",
		Long:    "Permanently delete a Deployment and all of its data including DAGs, task logs, Airflow metadata, environment variables, connections, API tokens, and alerts. Running tasks are terminated without waiting. Cluster resources are deallocated asynchronously. This action cannot be undone.",
		Example: `
  $ astro deployment delete <deployment-id>
  $ astro deployment delete --deployment my-deployment --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error { return deploymentDelete(cmd, args, out) },
	}
	cmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Don't ask for confirmation before deleting the Deployment")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to delete")
	addDeploymentFlag(cmd.Flags(), "Deployment to delete: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentVariableRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "variable",
		Aliases: []string{"var", "variables"},
		Short:   "Manage Deployment environment variables",
		Long: `Manage environment variables stored on the Deployment record. These variables can be used in DAGs or to customize your Airflow environment.

For variables shared across deployments or scoped to a workspace, see 'astro env variable'.`,
	}
	cliout.AddOutputFlag(cmd, &deploymentVariableOutput)
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
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "Deployment to list variables for. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&variableKey, "key", "k", "", "Specify a key to find a specific variable")
	cmd.Flags().BoolVarP(&useEnvFile, "save", "s", false, "Save Deployment variables to an environment file")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of the file to save environment variables to")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the Deployment to list variables from")
	addDeploymentFlag(cmd.Flags(), "Deployment to list variables for: a link name from pyproject.toml, a Deployment id, or a Deployment name")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
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
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "Deployment assigned to variables. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&variableKey, "key", "k", "", "Key for the new variable")
	cmd.Flags().StringVarP(&variableValue, "value", "v", "", "Value for the new variable")
	cmd.Flags().BoolVarP(&useEnvFile, "load", "l", false, "Create environment variables loaded from an environment file")
	cmd.Flags().BoolVarP(&makeSecret, "secret", "s", false, "Set the new environment variables as secrets")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file to load environment variables from")
	_ = cmd.Flags().MarkHidden("key")   //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	_ = cmd.Flags().MarkHidden("value") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to create variables from")
	addDeploymentFlag(cmd.Flags(), "Deployment to create variables for: a link name from pyproject.toml, a Deployment id, or a Deployment name")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
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
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "Deployment assigned to variables. Run 'astro deployment list' to find valid IDs")
	cmd.Flags().StringVarP(&variableKey, "key", "k", "", "Key of the variable to update")
	cmd.Flags().StringVarP(&variableValue, "value", "v", "", "Value of the variable to update")
	cmd.Flags().BoolVarP(&useEnvFile, "load", "l", false, "Update environment variables loaded from an environment file")
	cmd.Flags().BoolVarP(&makeSecret, "secret", "s", false, "Set updated environment variables as secrets")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file to load environment variables to update from")
	_ = cmd.Flags().MarkHidden("key")   //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	_ = cmd.Flags().MarkHidden("value") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to update variables from")
	addDeploymentFlag(cmd.Flags(), "Deployment to update variables for: a link name from pyproject.toml, a Deployment id, or a Deployment name")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentHibernateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "hibernate [DEPLOYMENT-ID]",
		Aliases: []string{"hb"},
		Short:   "Hibernate an Astro development Deployment",
		Long:    "Hibernate an Astro development Deployment for a set amount of time. Overrides any existing hibernation schedule and sets the Deployment to hibernate for a specific duration or until a specific time. Use the '--remove-override' flag to remove any existing override and resume the regular hibernation schedule.",
		Example: `
  $ astro deployment hibernate <deployment-id>
  $ astro deployment hibernate <deployment-id> --for 2h30m
  $ astro deployment hibernate <deployment-id> --until 2024-06-01T12:00:00Z
  $ astro deployment hibernate <deployment-id> --remove-override
  $ astro deployment hibernate <deployment-id> --wait
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentOverrideHibernation(cmd, args, out, true)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the Deployment to hibernate")
	addDeploymentFlag(cmd.Flags(), "Deployment to hibernate: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&until, "until", "u", "", "Specify the hibernation period using an end date and time. Example value: 2021-01-01T00:00:00Z")
	cmd.Flags().StringVarP(&forDuration, "for", "d", "", "Specify the hibernation period using a duration. Example value: 1h30m")
	cmd.Flags().BoolVarP(&removeOverride, "remove-override", "r", false, "Remove any existing override and resume regular hibernation schedule.")
	cmd.Flags().BoolVarP(&forceOverride, "yes", "y", false, "Don't ask for confirmation before hibernating the Deployment")
	cmd.Flags().BoolVarP(&waitForStatus, "wait", "i", false, "Wait for the Deployment to hibernate before ending the command")
	cmd.Flags().DurationVar(&waitTimeForDeployment, "wait-time", deploymentWaitTime, "Wait time for the Deployment to hibernate before ending the command. Can only be used with --wait=true")
	cmd.MarkFlagsMutuallyExclusive("until", "for", "remove-override")
	cmd.MarkFlagsMutuallyExclusive("wait", "remove-override")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentWakeUpCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "wake-up [DEPLOYMENT-ID]",
		Aliases: []string{"wu"},
		Short:   "Wake up an Astro development Deployment",
		Long:    "Wake up an Astro development Deployment from hibernation. Overrides any existing hibernation schedule and sets the Deployment to run for a specific duration or until a specific time. Use the '--remove-override' flag to remove any existing override and resume the regular hibernation schedule.",
		Example: `
  $ astro deployment wake-up <deployment-id>
  $ astro deployment wake-up <deployment-id> --for 4h
  $ astro deployment wake-up <deployment-id> --until 2024-06-01T18:00:00Z
  $ astro deployment wake-up <deployment-id> --remove-override
  $ astro deployment wake-up <deployment-id> --wait
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentOverrideHibernation(cmd, args, out, false)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the Deployment to wake up.")
	addDeploymentFlag(cmd.Flags(), "Deployment to wake up: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&until, "until", "u", "", "Specify the awake period using an end date. Example value: 2021-01-01T00:00:00Z")
	cmd.Flags().StringVarP(&forDuration, "for", "d", "", "Specify the awake period using a duration. Example value: 1h30m")
	cmd.Flags().BoolVarP(&removeOverride, "remove-override", "r", false, "Remove any existing override and resume the regular hibernation schedule.")
	cmd.Flags().BoolVarP(&forceOverride, "yes", "y", false, "Don't ask for confirmation before waking up the Deployment")
	cmd.Flags().BoolVarP(&waitForStatus, "wait", "i", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVar(&waitTimeForDeployment, "wait-time", deploymentWaitTime, "Wait time for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cmd.MarkFlagsMutuallyExclusive("until", "for", "remove-override")
	cmd.MarkFlagsMutuallyExclusive("wait", "remove-override")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func deploymentList(cmd *cobra.Command, out io.Writer) error {
	// Reject a bad -o before anything that needs a login.
	format, err := cliout.ParseFormat(deploymentListOutput)
	if err != nil {
		return err
	}

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

	return deployment.ListWithFormat(ws, allDeployments, astroV1Client, cliout.Renderer{Format: format, Out: out})
}

func deploymentLogs(cmd *cobra.Command, args []string, out io.Writer) error {
	// Reject a bad -o before anything else, so it is a usage error.
	format, err := cliout.ParseFormat(deploymentLogsOutput)
	if err != nil {
		return err
	}
	// The Deployment lookup prints its notes to bare stdout; under json they
	// are notes, not log records.
	defer strayStdoutToStderr(format)()

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}
	logServer := logWebserver || logApiserver

	res, err := deployment.Logs(deploymentID, ws, deploymentName, logsKeyword, logServer, logScheduler, logTriggerer, logWorkers, logDagProcessor, logComponents, warnLogs, errorLogs, infoLogs, logCount, astroV1Client)
	if err != nil {
		return err
	}
	return emitLogs(cmd, cliout.Renderer{Format: format, Out: out}, res)
}

func deploymentCreate(cmd *cobra.Command, _ []string, out io.Writer) error { //nolint:gocognit,gocyclo // v1 complexity, refactor tracked separately
	// Reject a bad -o before anything else, so it is a usage error.
	format, err := cliout.ParseFormat(deploymentOutput)
	if err != nil {
		return err
	}
	// The create prints its progress (the Workspace, a picker, the wait) to
	// bare stdout; under json that is a note, not the result.
	defer strayStdoutToStderr(format)()
	if cmd.Flags().Changed("clone") {
		return deploymentClone(cmd, out, format)
	}
	if err := normalizeSchedulerSizeFlag(); err != nil {
		return err
	}
	// Find Workspace ID
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}
	workspaceID = ws

	// clean output
	deployment.CleanOutput = cleanOutput

	// Get latest runtime version
	if runtimeVersion == "" {
		airflowVersionClient := airflowversions.NewClient(httpClient, false, false)
		runtimeVersion, err = airflowversions.GetDefaultImageTag(airflowVersionClient, "", "", false)
		if err != nil {
			return err
		}
	}

	// set default executor if none was specified
	if executor == "" {
		if airflowversions.IsAirflow3(runtimeVersion) {
			executor = deployment.AstroExecutor
		} else {
			executor = deployment.CeleryExecutor
		}
	}

	// check if executor is valid
	if !deployment.IsValidExecutor(executor, runtimeVersion, deploymentType) {
		return fmt.Errorf("%s is %w for runtime version %s deployment type %s", executor, errInvalidExecutor, runtimeVersion, deploymentType)
	}

	if highAvailability != "" && !(highAvailability == enable || highAvailability == disable) {
		return errors.New("Invalid --high-availability value")
	}
	if developmentMode != "" && !(developmentMode == enable || developmentMode == disable) {
		return errors.New("Invalid --development-mode value")
	}
	if organization.IsOrgHosted() && !(deploymentType == standard || deploymentType == dedicated || deploymentType == hostedStandard || deploymentType == hostedShared || deploymentType == hostedDedicated) {
		return errors.New("Invalid --type value")
	}
	if cicdEnforcement != "" && !(cicdEnforcement == enable || cicdEnforcement == disable) {
		return errors.New("Invalid --cicd-enforcement value")
	}
	if organization.IsOrgHosted() && clusterID != "" && (deploymentType == standard || deploymentType == hostedStandard || deploymentType == hostedShared) {
		return errors.New("flag --cluster-id cannot be used to create a standard deployment. If you want to create a dedicated deployment, use --type dedicated along with --cluster-id")
	}
	if cmd.Flags().Changed("allowed-ip-address-ranges") {
		if !flagRemoteExecutionEnabled {
			return errors.New("flag --allowed-ip-address-ranges cannot be used when remote execution is disabled")
		}
		allowedIPAddressRanges = fromCsv(flagAllowedIPAddressRanges)
	}
	if cmd.Flags().Changed("task-log-bucket") {
		if !flagRemoteExecutionEnabled {
			return errors.New("flag --task-log-bucket cannot be used when remote execution is disabled")
		}
		taskLogBucket = &flagTaskLogBucket
	}
	if cmd.Flags().Changed("task-log-url-pattern") {
		if !flagRemoteExecutionEnabled {
			return errors.New("flag --task-log-url-pattern cannot be used when remote execution is disabled")
		}
		taskLogURLPattern = &flagTaskLogURLPattern
	}

	if cmd.Flags().Changed("wait-time") && !waitForStatus {
		return errors.New("cannot use --wait-time with --wait=false")
	}

	var coreDeploymentType astrov1.DeploymentType
	if deploymentType == standard || deploymentType == hostedStandard || deploymentType == hostedShared {
		coreDeploymentType = astrov1.DeploymentTypeSTANDARD
	}
	if deploymentType == dedicated || deploymentType == hostedDedicated {
		coreDeploymentType = astrov1.DeploymentTypeDEDICATED
	}

	if !organization.IsOrgHosted() {
		coreDeploymentType = astrov1.DeploymentTypeHYBRID
	}
	if dagDeploy != "" && !(dagDeploy == enable || dagDeploy == disable) {
		return errors.New("Invalid --dag-deploy value)")
	}
	if dagDeploy == "" {
		if organization.IsOrgHosted() && !flagRemoteExecutionEnabled {
			dagDeploy = enable
		} else {
			dagDeploy = disable
		}
	}

	// validate cloudProvider
	if cloudProvider != "" {
		if !isValidCloudProvider(astrov1.ClusterCloudProvider(strings.ToUpper(cloudProvider))) {
			return fmt.Errorf("%s is %w", cloudProvider, errInvalidCloudProvider)
		}
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	d, err := deployment.Create(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCPU, defaultTaskPodMemory, resourceQuotaCPU, resourceQuotaMemory, workloadIdentity, coreDeploymentType, schedulerAU, schedulerReplicas, flagRemoteExecutionEnabled, allowedIPAddressRanges, taskLogBucket, taskLogURLPattern, astroV1Client, waitForStatus, waitTimeForDeployment, cmd.ErrOrStderr())
	if d.Id == "" {
		return err
	}
	// The Deployment exists even when the --wait for it failed, so it is
	// published either way.
	r := cliout.Renderer{Format: format, Out: out}
	if emitErr := emitDeployment(r, &d, func(w io.Writer) error { return deployment.WriteCreated(w, workspaceID, &d) }); emitErr != nil {
		if err != nil {
			return err
		}
		return emitErr
	}
	return failedAfterResult(cmd, format, err)
}

func deploymentUpdate(cmd *cobra.Command, args []string, out io.Writer) error {
	// Reject a bad -o before anything else, so it is a usage error.
	format, err := cliout.ParseFormat(deploymentOutput)
	if err != nil {
		return err
	}
	// The update prints its warnings and notes to bare stdout; under json
	// they are notes, not the result.
	defer strayStdoutToStderr(format)()
	if err := normalizeSchedulerSizeFlag(); err != nil {
		return err
	}
	// Find Workspace ID
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// clean output
	deployment.CleanOutput = cleanOutput

	// check if executor is valid
	if executor != "" && !deployment.IsValidExecutor(executor, runtimeVersion, deploymentType) {
		return fmt.Errorf("%s is %w", executor, errInvalidExecutor)
	}
	if dagDeploy != "" && !(dagDeploy == enable || dagDeploy == disable) {
		return errors.New("Invalid --dag-deploy value")
	}

	if highAvailability != "" && !(highAvailability == enable || highAvailability == disable) {
		return errors.New("Invalid --high-availability value")
	}
	if developmentMode != "" && !(developmentMode == enable || developmentMode == disable) {
		return errors.New("Invalid --development-mode value")
	}
	if cicdEnforcement != "" && !(cicdEnforcement == enable || cicdEnforcement == disable) {
		return errors.New("Invalid --cicd-enforcement value")
	}
	if cmd.Flags().Changed("allowed-ip-address-ranges") {
		allowedIPAddressRanges = fromCsv(flagAllowedIPAddressRanges)
	}
	if cmd.Flags().Changed("task-log-bucket") {
		taskLogBucket = &flagTaskLogBucket
	}
	if cmd.Flags().Changed("task-log-url-pattern") {
		taskLogURLPattern = &flagTaskLogURLPattern
	}

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	res, err := deployment.Update(deploymentID, label, ws, description, deploymentName, dagDeploy, executor, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCPU, defaultTaskPodMemory, resourceQuotaCPU, resourceQuotaMemory, workloadIdentity, updateSchedulerAU, updateSchedulerReplicas, []astrov1.WorkerQueueRequest{}, []astrov1.HybridWorkerQueueRequest{}, []astrov1.DeploymentEnvironmentVariableRequest{}, allowedIPAddressRanges, taskLogBucket, taskLogURLPattern, forceUpdate, astroV1Client, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	return emitUpdated(cliout.Renderer{Format: format, Out: out}, &res)
}

// normalizeSchedulerSizeFlag refuses a --scheduler-size that is not a size,
// before anything is asked of the API, and lowercases one that is. Create
// and Update match the lowercase spelling and send no size for anything
// else, so --scheduler-size extra-large used to succeed having set nothing.
func normalizeSchedulerSizeFlag() error {
	if schedulerSize == "" {
		return nil
	}
	size, ok := deployment.NormalizeSchedulerSize(schedulerSize)
	if !ok {
		return cliout.Usage(fmt.Errorf("invalid --scheduler-size %q: use one of %s", schedulerSize, strings.Join(deployment.SchedulerSizes, ", ")))
	}
	schedulerSize = size
	return nil
}

func deploymentDelete(cmd *cobra.Command, args []string, out io.Writer) error {
	// Reject a bad -o before anything else, so it is a usage error.
	format, err := cliout.ParseFormat(deploymentOutput)
	if err != nil {
		return err
	}
	defer strayStdoutToStderr(format)()
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	removal, err := deployment.Delete(deploymentID, ws, deploymentName, forceDelete, astroV1Client)
	if err != nil || removal == nil {
		// nil, nil is a declined question, which has said so.
		return err
	}
	return emitRemoval(cliout.Renderer{Format: format, Out: out}, removal)
}

func deploymentVariableList(cmd *cobra.Command, _ []string, out io.Writer) error {
	format, err := cliout.ParseFormat(deploymentVariableOutput)
	if err != nil {
		return err
	}
	defer strayStdoutToStderr(format)()

	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	vars, err := deployment.VariableList(deploymentID, variableKey, ws, envFile, deploymentName, useEnvFile, astroV1Client)
	if err != nil {
		return err
	}
	return emitVariableList(cmd, cliout.Renderer{Format: format, Out: out}, vars)
}

func deploymentVariableCreate(cmd *cobra.Command, args []string, out io.Writer) error {
	return deploymentVariableModify(cmd, args, out, false, "failed to find a valid Workspace")
}

func deploymentVariableUpdate(cmd *cobra.Command, args []string, out io.Writer) error {
	return deploymentVariableModify(cmd, args, out, true, "failed to find a valid workspace")
}

// deploymentVariableModify is create and update, which differ only in whether
// an existing key takes the new value, and in the capital of their workspace
// error, which is kept as it was.
func deploymentVariableModify(cmd *cobra.Command, args []string, out io.Writer, update bool, wsErr string) error {
	format, err := cliout.ParseFormat(deploymentVariableOutput)
	if err != nil {
		return err
	}
	defer strayStdoutToStderr(format)()

	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, wsErr)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	res, err := deployment.VariableModify(deploymentID, variableKey, variableValue, ws, envFile,
		deploymentName, args, useEnvFile, makeSecret, update, astroV1Client)
	if err != nil {
		return err
	}
	return emitVariableModify(cliout.Renderer{Format: format, Out: out}, res)
}

func deploymentOverrideHibernation(cmd *cobra.Command, args []string, out io.Writer, isHibernating bool) error {
	// Reject a bad -o before anything else, so it is a usage error.
	format, err := cliout.ParseFormat(deploymentOutput)
	if err != nil {
		return err
	}
	// The --wait prints its progress to bare stdout; under json that is a
	// note, not the result.
	defer strayStdoutToStderr(format)()
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Get deploymentId from args, if passed
	if len(args) > 0 {
		deploymentID = args[0]
	}

	if cmd.Flags().Changed("wait-time") && !waitForStatus {
		return errors.New("cannot use --wait-time with --wait=false")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	r := cliout.Renderer{Format: format, Out: out}
	action := "wake up"
	if isHibernating {
		action = "hibernate"
	}

	if removeOverride {
		res, err := deployment.DeleteDeploymentHibernationOverride(deploymentID, ws, deploymentName, forceOverride, astroV1Client)
		if err != nil || res == nil {
			// nil, nil is a declined question, which has said so.
			return err
		}
		return emitHibernation(r, res, action)
	}

	overrideUntil, err := getOverrideUntil(until, forDuration)
	if err != nil {
		return err
	}

	res, err := deployment.UpdateDeploymentHibernationOverride(deploymentID, ws, deploymentName, isHibernating, overrideUntil, forceOverride, astroV1Client)
	if err != nil || res == nil {
		return err
	}
	// The override is set, and said so, before the wait for it starts.
	if err := emitHibernation(r, res, action); err != nil {
		return err
	}
	if !waitForStatus {
		return nil
	}
	return failedAfterResult(cmd, format, deployment.WaitForHibernationOverride(cmd.ErrOrStderr(), res.DeploymentID, isHibernating, waitTimeForDeployment, astroV1Client))
}

// isValidCloudProvider returns true for valid CloudProvider values and false if not.
func isValidCloudProvider(cloudProvider astrov1.ClusterCloudProvider) bool {
	return cloudProvider == astrov1.ClusterCloudProviderGCP || cloudProvider == astrov1.ClusterCloudProviderAWS || cloudProvider == astrov1.ClusterCloudProviderAZURE
}

func addDeploymentUser(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := cliout.ParseFormat(deploymentUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.AddDeploymentUser(email, addDeploymentRole, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &u, fmt.Sprintf("The user %s was successfully added to the deployment with the role %s", u.Email, u.DeploymentRole))
}

func listDeploymentUser(cmd *cobra.Command, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}

	format, err := cliout.ParseFormat(deploymentUserOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return user.ListDeploymentUsersWithFormat(astroV1Client, deploymentID, cliout.Renderer{Format: format, Out: out})
}

func updateDeploymentUser(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := cliout.ParseFormat(deploymentUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if updateDeploymentRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a user Deployment role or custom role name to update user: ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		updateDeploymentRole = answer
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.UpdateDeploymentUserRole(email, updateDeploymentRole, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &u, fmt.Sprintf("The deployment user %s role was successfully updated to %s", u.Email, u.DeploymentRole))
}

func removeDeploymentUser(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := cliout.ParseFormat(deploymentUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	r, err := user.RemoveDeploymentUser(email, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &r, fmt.Sprintf("The user %s was successfully removed from the deployment", r.Email))
}

func newDeploymentTokenRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "token",
		Aliases: []string{"to"},
		Short:   "Manage tokens in your Astro Deployment",
		Long:    "Manage tokens in your Astro Deployment.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newDeploymentTokenListCmd(out),
		newDeploymentTokenCreateCmd(out),
		newDeploymentTokenUpdateCmd(out),
		newDeploymentTokenRotateCmd(out),
		newDeploymentTokenDeleteCmd(out),
		newWorkspaceTokenManageCmd(out),
		newOrgTokenManageCmd(out),
	)
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment where you would like to manage tokens. Run 'astro deployment list' to find valid IDs")
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment whose tokens you'd like to manage: a Deployment id, or a link name from pyproject.toml. Run 'astro deployment list' to find valid IDs")
	cliout.AddOutputFlag(cmd, &deploymentTokenOutput)
	return cmd
}

func newDeploymentTokenListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the API tokens in an Astro Deployment",
		Long:    "List all API tokens with a role in a Deployment, including Deployment-scoped tokens and any Organization or Workspace tokens that have been granted a Deployment role.",
		Example: `
  $ astro deployment token list --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listDeploymentToken(cmd, out)
		},
	}
	return cmd
}

func newDeploymentTokenCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an API token in an Astro Deployment",
		Long:    "Create a Deployment-scoped API token. The token value is displayed only once at creation and cannot be retrieved later — store it securely. Use --clean-output to print only the raw token value for scripts. Use --expiration to set a TTL in days (default: no expiration).",
		Example: `
  $ astro deployment token create --name my-token --role DEPLOYMENT_ADMIN --deployment <deployment-id>
  $ astro deployment token create --name my-token --role DEPLOYMENT_ADMIN --deployment <deployment-id> --expiration 30
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createDeploymentToken(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&tokenName, "name", "n", "", "The token's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Description of the token. If the description contains a space, specify the entire description within quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The role for the "+
		"token. Possible values are DEPLOYMENT_ADMIN or a custom role name")
	cmd.Flags().IntVarP(&tokenExpiration, "expiration", "e", 0, "Expiration of the token in days. If the flag isn't used the token won't have an expiration. Must be between 1 and 3650 days. ")
	return cmd
}

func newDeploymentTokenUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TOKEN_ID]",
		Aliases: []string{"up"},
		Short:   "Update a Deployment API token",
		Long:    "Update a Deployment API token's name, description, or role. Identify the token by its ID (positional argument) or current name (--name). Only what you pass changes: without --role the token keeps its role, and a --role the token already holds is refused before anything changes.",
		Example: `
  $ astro deployment token update <token-id> --deployment <deployment-id> --new-name my-new-token-name --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateDeploymentToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The current name of the token. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenName, "new-name", "n", "", "The token's new name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "updated description of the token. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&deploymentTokenUpdateRole, "role", "r", "", "The new role for the "+
		"token. Possible values are DEPLOYMENT_ADMIN or a custom role name. Without it, the token keeps its role")
	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentTokenRotateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate [TOKEN_ID]",
		Aliases: []string{"ro"},
		Short:   "Rotate a Deployment API token",
		Long:    "Rotate a Deployment API token, generating a new token value and invalidating the old one. The new value is displayed only once. You can only rotate Deployment-scoped tokens from this command — use the workspace or organization token rotate commands for tokens at other scopes.",
		Example: `
  $ astro deployment token rotate <token-id> --deployment <deployment-id>
  $ astro deployment token rotate <token-id> --deployment <deployment-id> --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rotateDeploymentToken(cmd, args, out)
		},
	}
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be rotated. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceRotate, "yes", "y", false, "Don't ask for confirmation before rotating the Deployment API token")

	return cmd
}

func newDeploymentTokenDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [TOKEN_ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Deployment API token",
		Long:    "Permanently revoke a Deployment API token. All access the token grants is immediately revoked.",
		Example: `
  $ astro deployment token delete <token-id> --deployment <deployment-id>
  $ astro deployment token delete <token-id> --deployment <deployment-id> --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteDeploymentToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be deleted. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Don't ask for confirmation before deleting or removing the API token")

	return cmd
}

func newOrgTokenManageCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organization-token",
		Short: "Manage organization tokens in a deployment",
		Long:  "Manage organization tokens in a deployment",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newAddOrganizationTokenDeploymentRole(out),
		newUpdateOrganizationTokenDeploymentRole(out),
		newRemoveOrganizationTokenDeploymentRole(out),
		newListOrganizationTokensInDeployment(out),
	)
	return cmd
}

func newWorkspaceTokenManageCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace-token",
		Short: "Manage workspace tokens in a deployment",
		Long:  "Manage workspace tokens in a deployment",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newAddWorkspaceTokenDeploymentRole(out),
		newUpdateWorkspaceTokenDeploymentRole(out),
		newRemoveWorkspaceTokenDeploymentRole(out),
		newListWorkspaceTokensInDeployment(out),
	)
	return cmd
}

func newAddOrganizationTokenDeploymentRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [ORG_TOKEN_ID]",
		Short: "Add an Organization API token to a Deployment",
		Long:  "Add an Organization API token to a Deployment\n$astro deployment token organization-token add [ORG_TOKEN_ID] --org-token-name [token name] --role [DEPLOYMENT_ADMIN or a custom role name].",
		Example: `
  $ astro deployment token organization-token add <org-token-id> --deployment <deployment-id> --role DEPLOYMENT_ADMIN
  $ astro deployment token organization-token add --org-token-name my-org-token --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenToDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to add to a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Deployment role to add to the "+
		"Organization API token. Possible values are DEPLOYMENT_ADMIN or a custom role name.")
	return cmd
}

func newUpdateOrganizationTokenDeploymentRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [ORG_TOKEN_ID]",
		Short: "Update an Organization API token's Deployment Role",
		Long:  "Update an Organization API token's Deployment Role\n$astro deployment token organization-token update [ORG_TOKEN_ID] --org-token-name [token name] --role [DEPLOYMENT_ADMIN or a custom role name].",
		Example: `
  $ astro deployment token organization-token update <org-token-id> --deployment <deployment-id> --role DEPLOYMENT_ADMIN
  $ astro deployment token organization-token update --org-token-name my-org-token --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateOrgTokenToDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to update in a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Deployment role to update the "+
		"Organization API token. Possible values are DEPLOYMENT_ADMIN or a custom role name.")
	return cmd
}

func newAddWorkspaceTokenDeploymentRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [WORKSPACE_TOKEN_ID]",
		Short: "Add a Workspace API token's Deployment Role",
		Long:  "Add a Workspace API token's Deployment Role\n$astro deployment token workspace-token add [WORKSPACE_TOKEN_ID] --workspace-token-name [token name] --role [DEPLOYMENT_ADMIN or a custom role name].",
		Example: `
  $ astro deployment token workspace-token add <workspace-token-id> --deployment <deployment-id> --role DEPLOYMENT_ADMIN
  $ astro deployment token workspace-token add --workspace-token-name my-ws-token --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceTokenDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "workspace-token-name", "n", "", "The name of the WORKSPACE API token you want to add to a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Deployment role to grant to the "+
		"Workspace API token. Possible values are DEPLOYMENT_ADMIN or a custom role name.")
	return cmd
}

func newUpdateWorkspaceTokenDeploymentRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [WORKSPACE_TOKEN_ID]",
		Short: "Update a Workspace API token's Deployment Role",
		Long:  "Update a Workspace API token's Deployment Role\n$astro deployment token workspace-token update [WORKSPACE_TOKEN_ID] --workspace-token-name [token name] --role [DEPLOYMENT_ADMIN or a custom role name].",
		Example: `
  $ astro deployment token workspace-token update <workspace-token-id> --deployment <deployment-id> --role DEPLOYMENT_ADMIN
  $ astro deployment token workspace-token update --workspace-token-name my-ws-token --deployment <deployment-id> --role DEPLOYMENT_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceTokenDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "workspace-token-name", "n", "", "The name of the WORKSPACE API token you want to update in a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Deployment role to grant to the "+
		"Workspace API token. Possible values are DEPLOYMENT_ADMIN or a custom role name.")
	return cmd
}

func addOrgTokenToDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	return setTokenDeploymentRole(cmd, args, out, tokenKindOrganization, tokenRoleAdd)
}

func updateOrgTokenToDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	return setTokenDeploymentRole(cmd, args, out, tokenKindOrganization, tokenRoleUpdate)
}

func addWorkspaceTokenDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	return setTokenDeploymentRole(cmd, args, out, tokenKindWorkspace, tokenRoleAdd)
}

func updateWorkspaceTokenDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	return setTokenDeploymentRole(cmd, args, out, tokenKindWorkspace, tokenRoleUpdate)
}

func newRemoveOrganizationTokenDeploymentRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [ORG_TOKEN_ID]",
		Short: "Remove an Organization API token's Deployment Role",
		Long:  "Remove an Organization API token's Deployment Role\n$astro deployment token organization-token remove [ORG_TOKEN_ID] --org-token-name [token name].",
		Example: `
  $ astro deployment token organization-token remove <org-token-id> --deployment <deployment-id>
  $ astro deployment token organization-token remove --org-token-name my-org-token --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeOrgTokenFromDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to remove from a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	return cmd
}

func newRemoveWorkspaceTokenDeploymentRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [WORKSPACE_TOKEN_ID]",
		Short: "Remove a Workspace API token's Deployment Role",
		Long:  "Remove a Workspace API token's Deployment Role\n$astro deployment token workspace-token remove [WORKSPACE_TOKEN_ID] --workspace-token-name [token name].",
		Example: `
  $ astro deployment token workspace-token remove <workspace-token-id> --deployment <deployment-id>
  $ astro deployment token workspace-token remove --workspace-token-name my-ws-token --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceTokenDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "workspace-token-name", "n", "", "The name of the WORKSPACE API token you want to remove from a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	return cmd
}

func removeOrgTokenFromDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		orgTokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return runDeploymentTokenRemove(format, out, tokenKindOrganization)
}

func removeWorkspaceTokenDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		workspaceTokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return runDeploymentTokenRemove(format, out, tokenKindWorkspace)
}

func newListOrganizationTokensInDeployment(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Organization API tokens in a deployment",
		Long:  "List all Organization API tokens in a deployment\n$astro deployment token organization-token list",
		Example: `
  $ astro deployment token organization-token list --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationTokensInDeployment(cmd, out)
		},
	}
	return cmd
}

func newListWorkspaceTokensInDeployment(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Workspace API tokens in a deployment",
		Long:  "List all Workspace API tokens in a deployment\n$astro deployment token workspace-token list",
		Example: `
  $ astro deployment token workspace-token list --deployment <deployment-id>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceTokensInDeployment(cmd, out)
		},
	}
	return cmd
}

func listOrganizationTokensInDeployment(cmd *cobra.Command, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it

	cmd.SilenceUsage = true
	return runDeploymentTokenList(format, out, deployment.DeploymentTokenTypeORGANIZATION)
}

func listWorkspaceTokensInDeployment(cmd *cobra.Command, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it

	cmd.SilenceUsage = true
	return runDeploymentTokenList(format, out, deployment.DeploymentTokenTypeWORKSPACE)
}

func listDeploymentToken(cmd *cobra.Command, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	return runDeploymentTokenList(format, out)
}

func createDeploymentToken(cmd *cobra.Command, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	if tokenName == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a name for the new Deployment API token: ", input.AnsweredBy("--name"))
		if err != nil {
			return err
		}
		tokenName = answer
	}

	if tokenRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a role for the new Deployment API token (Possible values are DEPLOYMENT_ADMIN or a custom role name): ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		tokenRole = answer
	}

	cmd.SilenceUsage = true
	return runDeploymentTokenCreate(format, out)
}

func updateDeploymentToken(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return runDeploymentTokenUpdate(format, out)
}

func rotateDeploymentToken(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}
	cmd.SilenceUsage = true
	return runDeploymentTokenRotate(format, out)
}

func deleteDeploymentToken(cmd *cobra.Command, args []string, out io.Writer) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	format, err := tokenFormat()
	if err != nil {
		return err
	}
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return runDeploymentTokenDelete(format, out)
}

func getOverrideUntil(until, forDuration string) (*time.Time, error) {
	if until != "" {
		untilParsed, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return nil, err
		}
		return &untilParsed, nil
	}
	if forDuration != "" {
		forDurationParsed, err := time.ParseDuration(forDuration)
		if err != nil {
			return nil, err
		}
		overrideUntil := time.Now().Add(forDurationParsed)
		return &overrideUntil, nil
	}
	return nil, nil
}

func fromCsv(s string) *[]string {
	ss := strings.Split(strings.TrimSpace(s), ",")
	return &ss
}
