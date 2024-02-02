package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/software/deployment"
	"github.com/spf13/cobra"
)

const (
	celeryExecutorArg     = "celery"
	localExecutorArg      = "local"
	kubernetesExecutorArg = "kubernetes"
	k8sExecutorArg        = "k8s"

	cliDeploymentHardDeletePrompt = "\nWarning: This action permanently deletes all data associated with this Deployment, including the database. You will not be able to recover it. Proceed with hard delete?"
	deploymentTypeCmdMessage      = "DAG Deployment mechanism: image, volume, git_sync, dag_only"
)

var (
	allDeployments              bool
	cancel                      bool
	hardDelete                  bool
	executor                    string
	airflowVersion              string
	deploymentCreateLabel       string
	deploymentUpdateLabel       string
	deploymentUpdateDescription string
	// have to use two different executor flags for create and update commands otherwise both commands override this value
	executorUpdate          string
	deploymentID            string
	desiredAirflowVersion   string
	cloudRole               string
	releaseName             string
	nfsLocation             string
	dagDeploymentType       string
	createTriggererReplicas int
	updateTriggererReplicas int
	gitRevision             string
	gitRepoURL              string
	gitBranchName           string
	gitDAGDir               string
	gitSyncInterval         int
	sshKey                  string
	knowHosts               string
	runtimeVersion          string
	desiredRuntimeVersion   string
	deploymentCreateExample = `
# Create new deployment with Celery executor (default: celery without params).
$ astro deployment create --label=new-deployment-name --executor=celery

# Create new deployment with Local executor.
$ astro deployment create --label=new-deployment-name-local --executor=local

# Create new deployment with Kubernetes executor.
$ astro deployment create --label=new-deployment-name-k8s --executor=k8s --airflow-version=2.4.1

# Create new deployment with Astronomer Runtime.
$ astro deployment create --label=my-new-deployment --executor=k8s --runtime-version=6.0.1
`
	createExampleDagDeployment = `
# Create new deployment with Kubernetes executor and dag deployment type volume and nfs location.
$ astro deployment create --label=my-new-deployment --executor=k8s --airflow-version=2.4.1 --dag-deployment-type=volume --nfs-location=test:/test
`
	deploymentAirflowUpgradeExample = `
  $ astro deployment airflow upgrade --deployment-id=<deployment-id> --desired-airflow-version=<desired-airflow-version>

# Abort the initial airflow upgrade step:
  $ astro deployment airflow upgrade --cancel --deployment-id=<deployment-id>
`
	deploymentRuntimeUpgradeExample = `
$ astro deployment runtime upgrade --deployment-id=<deployment-id> --desired-runtime-version=<desired-runtime-version>
# Abort the initial runtime upgrade step:
$ astro deployment runtime upgrade --deployment-id=<deployment-id> --cancel
`
	deploymentRuntimeMigrateExample = `
$ astro deployment runtime migrate --deployment-id=<deployment-id>
# Abort the initial runtime migrate step:
$ astro deployment runtime migrate --deployment-id=<deployment-id> --cancel
`
)

func newDeploymentRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de", "deployments"},
		Short:   "Manage Astronomer Deployments",
		Long:    "Deployments are individual Airflow clusters running on an installation of the Astronomer platform.",
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "ID of the workspace in which you want to manage deployments, you can leave it empty if you want to use your current context's workspace ID")
	cmd.AddCommand(
		newDeploymentCreateCmd(out),
		newDeploymentListCmd(out),
		newDeploymentUpdateCmd(out),
		newDeploymentDeleteCmd(out),
		newLogsCmd(out),
		newDeploymentSaRootCmd(out),
		newDeploymentUserRootCmd(out),
		newDeploymentAirflowRootCmd(out),
		newDeploymentTeamRootCmd(out),
	)

	if appConfig != nil && appConfig.Flags.AstroRuntimeEnabled {
		cmd.AddCommand(newDeploymentRuntimeRootCmd(out))
	}

	return cmd
}

func newDeploymentCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a new Astronomer Deployment",
		Long:    "Create a new Astronomer Deployment",
		Example: deploymentCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentCreate(cmd, out)
		},
	}

	var nfsMountDAGDeploymentEnabled, triggererEnabled, gitSyncDAGDeploymentEnabled, runtimeEnabled, dagOnlyDeployEnabled bool
	if appConfig != nil {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		triggererEnabled = appConfig.Flags.TriggererEnabled
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
		runtimeEnabled = appConfig.Flags.AstroRuntimeEnabled
		dagOnlyDeployEnabled = appConfig.Flags.DagOnlyDeployment
	}

	// let's hide under feature flag
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		cmd.Flags().StringVarP(&dagDeploymentType, "dag-deployment-type", "t", "", deploymentTypeCmdMessage)
	}

	if nfsMountDAGDeploymentEnabled {
		cmd.Example += createExampleDagDeployment
		cmd.Flags().StringVarP(&nfsLocation, "nfs-location", "n", "", "NFS Volume Mount, specified as: <IP>:/<path>. Input is automatically prepended with 'nfs://' - do not include.")
	}

	if gitSyncDAGDeploymentEnabled {
		addGitSyncDeploymentFlags(cmd)
	}

	if triggererEnabled {
		cmd.Flags().IntVarP(&createTriggererReplicas, "triggerer-replicas", "", 0, "Number of replicas to use for triggerer airflow component, valid 0-2")
	}

	if runtimeEnabled {
		cmd.Flags().StringVarP(&runtimeVersion, "runtime-version", "", "", "Add desired Astronomer Runtime version: e.g: 4.2.2 or 5.0.0")
	}

	cmd.Flags().StringVarP(&deploymentCreateLabel, "label", "l", "", "Label of your deployment")
	cmd.Flags().StringVarP(&executor, "executor", "e", celeryExecutorArg, "The executor used in your Airflow deployment, one of: local, celery, or kubernetes")
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "a", "", "Add desired Airflow version parameter: e.g: 1.10.5 or 1.10.7")
	cmd.Flags().StringVarP(&releaseName, "release-name", "r", "", "Set custom release-name if possible")
	cmd.Flags().StringVarP(&cloudRole, "cloud-role", "c", "", "Set cloud role to annotate service accounts in deployment")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}

func newDeploymentDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [deployment ID]",
		Aliases: []string{"de"},
		Short:   "Delete an Airflow Deployment",
		Long:    "Delete an Airflow Deployment",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentDelete(cmd, args, out)
		},
	}
	if appConfig != nil && appConfig.Flags.HardDeleteDeployment {
		cmd.Flags().BoolVar(&hardDelete, "hard", false, "Deletes all infrastructure and records for this Deployment")
	}
	return cmd
}

func newDeploymentListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Airflow Deployment",
		Long:    "List Airflow Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentList(cmd, out)
		},
	}
	cmd.Flags().BoolVarP(&allDeployments, "all", "a", false, "Show Deployments across all Workspaces")
	return cmd
}

func newDeploymentUpdateCmd(out io.Writer) *cobra.Command {
	example := `
# update executor for given deployment
$ astro deployment update [deployment ID] --executor=celery`
	updateExampleDagDeployment := `

# update dag deployment strategy
$ astro deployment update [deployment ID] --dag-deployment-type=volume --nfs-location=test:/test`
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update Airflow Deployments",
		Long:    "Update Airflow Deployments",
		Example: example,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUpdate(cmd, args, dagDeploymentType, nfsLocation, out)
		},
	}

	var nfsMountDAGDeploymentEnabled, triggererEnabled, gitSyncDAGDeploymentEnabled, dagOnlyDeployEnabled bool
	if appConfig != nil {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		triggererEnabled = appConfig.Flags.TriggererEnabled
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
		dagOnlyDeployEnabled = appConfig.Flags.DagOnlyDeployment
	}

	cmd.Flags().StringVarP(&executorUpdate, "executor", "e", "", "Add executor parameter: local, celery, or kubernetes")

	// let's hide under feature flag
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		cmd.Flags().StringVarP(&dagDeploymentType, "dag-deployment-type", "t", "", deploymentTypeCmdMessage)
	}

	if nfsMountDAGDeploymentEnabled {
		cmd.Example += updateExampleDagDeployment
		cmd.Flags().StringVarP(&nfsLocation, "nfs-location", "n", "", "NFS Volume Mount, specified as: <IP>:/<path>. Input is automatically prepended with 'nfs://' - do not include.")
	}

	if triggererEnabled {
		cmd.Flags().IntVarP(&updateTriggererReplicas, "triggerer-replicas", "", -1, "Number of replicas to use for triggerer Airflow component, valid 0-2")
	}

	//noline:dupl
	if gitSyncDAGDeploymentEnabled {
		addGitSyncDeploymentFlags(cmd)
	}

	cmd.Flags().StringVarP(&deploymentUpdateDescription, "description", "d", "", "Set description to update in deployment")
	cmd.Flags().StringVarP(&deploymentUpdateLabel, "label", "l", "", "Set label to update in deployment")
	cmd.Flags().StringVarP(&cloudRole, "cloud-role", "c", "", "Set cloud role to annotate service accounts in deployment")
	return cmd
}

func addGitSyncDeploymentFlags(cmd *cobra.Command) {
	const defaultSyncInterval = 60

	cmd.Flags().StringVarP(&gitRevision, "git-revision", "v", "", "Git revision (tag or hash) to check out")
	cmd.Flags().StringVarP(&gitRepoURL, "git-repository-url", "u", "", "The repository URL of the git repo")
	cmd.Flags().StringVarP(&gitBranchName, "git-branch-name", "b", "", "The Branch name of the git repo we will be syncing from")
	cmd.Flags().StringVarP(&gitDAGDir, "dag-directory-path", "p", "", "The directory where dags are stored in repo")
	cmd.Flags().IntVarP(&gitSyncInterval, "sync-interval", "s", defaultSyncInterval, "The interval in seconds in which git-sync will be polling git for updates")
	cmd.Flags().StringVarP(&sshKey, "ssh-key", "", "", "Path to the ssh public key file to use to clone your git repo")
	cmd.Flags().StringVarP(&knowHosts, "known-hosts", "", "", "Path to the known hosts file to use to clone your git repo")
}

func newDeploymentAirflowRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow",
		Aliases: []string{"ai"},
		Short:   "Manage airflow deployments",
		Long:    "Manage airflow deployments",
	}
	cmd.AddCommand(
		newDeploymentAirflowUpgradeCmd(out),
	)
	return cmd
}

//nolint:dupl
func newDeploymentAirflowUpgradeCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"up"},
		Short:   "Upgrade Airflow version",
		Long:    "Upgrade Airflow version",
		Example: deploymentAirflowUpgradeExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowUpgrade(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the deployment to upgrade")
	cmd.Flags().StringVarP(&desiredAirflowVersion, "desired-airflow-version", "v", "", "Desired Airflow version to upgrade to")
	cmd.Flags().BoolVarP(&cancel, "cancel", "c", false, "Abort the initial airflow upgrade step")
	err := cmd.MarkFlagRequired("deployment-id")
	if err != nil {
		fmt.Println("error adding deployment-id flag: ", err.Error())
	}
	return cmd
}

func newDeploymentRuntimeRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "runtime",
		Aliases: []string{"r"},
		Short:   "Manage runtime deployments",
		Long:    "Manage runtime deployments",
	}
	cmd.AddCommand(
		newDeploymentRuntimeUpgradeCmd(out),
		newDeploymentRuntimeMigrateCmd(out),
	)
	return cmd
}

func newDeploymentRuntimeUpgradeCmd(out io.Writer) *cobra.Command { //nolint:dupl
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"up"},
		Short:   "Upgrade Runtime image version",
		Long:    "Upgrade Runtime image version",
		Example: deploymentRuntimeUpgradeExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return deploymentRuntimeUpgrade(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the deployment to upgrade")
	cmd.Flags().StringVarP(&desiredRuntimeVersion, "desired-runtime-version", "v", "", "Desired Runtime version you wish to upgrade your deployment to")
	cmd.Flags().BoolVarP(&cancel, "cancel", "c", false, "Abort the initial runtime upgrade step")
	err := cmd.MarkFlagRequired("deployment-id")
	if err != nil {
		fmt.Println("error adding deployment-id flag: ", err.Error())
	}
	return cmd
}

func newDeploymentRuntimeMigrateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "migrate",
		Aliases: []string{"m"},
		Short:   "Migrate to runtime image based deployment",
		Long:    "Migrate to runtime image based deployment",
		Example: deploymentRuntimeMigrateExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return deploymentRuntimeMigrate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the deployment to migrate")
	cmd.Flags().BoolVarP(&cancel, "cancel", "c", false, "Abort the initial runtime migrate step")
	err := cmd.MarkFlagRequired("deployment-id")
	if err != nil {
		fmt.Println("error adding deployment-id flag: ", err.Error())
	}
	return cmd
}

func deploymentCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	executorType, err := validateExecutorArg(executor)
	if err != nil {
		return err
	}

	var nfsMountDAGDeploymentEnabled, gitSyncDAGDeploymentEnabled, dagOnlyDeployEnabled bool
	if appConfig != nil {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
		dagOnlyDeployEnabled = appConfig.Flags.DagOnlyDeployment
	}

	if !cmd.Flags().Changed("triggerer-replicas") {
		createTriggererReplicas = -1
	}

	// we should validate only in case when this feature has been enabled
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		err = validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL, false)
		if err != nil {
			return err
		}
	}
	req := &deployment.CreateDeploymentRequest{
		Label:             deploymentCreateLabel,
		WS:                ws,
		ReleaseName:       releaseName,
		CloudRole:         cloudRole,
		Executor:          executorType,
		AirflowVersion:    airflowVersion,
		RuntimeVersion:    runtimeVersion,
		DAGDeploymentType: dagDeploymentType,
		NFSLocation:       nfsLocation,
		GitRepoURL:        gitRepoURL,
		GitRevision:       gitRevision,
		GitBranchName:     gitBranchName,
		GitDAGDir:         gitDAGDir,
		SSHKey:            sshKey,
		KnownHosts:        knowHosts,
		GitSyncInterval:   gitSyncInterval,
		TriggererReplicas: createTriggererReplicas,
	}
	return deployment.Create(req, houstonClient, out)
}

func deploymentDelete(cmd *cobra.Command, args []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if hardDelete {
		i, _ := input.Confirm(cliDeploymentHardDeletePrompt)

		if !i {
			fmt.Println("Exit: This command was not executed and your Deployment was not hard deleted.\n If you want to delete your Deployment but not permanently, try\n $ astro deployment delete without the --hard flag.")
			return nil
		}
	}
	return deployment.Delete(args[0], hardDelete, houstonClient, out)
}

func deploymentList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Don't validate workspace if viewing all deployments
	if allDeployments {
		ws = ""
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.List(ws, allDeployments, houstonClient, out)
}

func deploymentUpdate(cmd *cobra.Command, args []string, dagDeploymentType, nfsLocation string, out io.Writer) error {
	argsMap := map[string]string{}
	if deploymentUpdateDescription != "" {
		argsMap["description"] = deploymentUpdateDescription
	}
	if deploymentUpdateLabel != "" {
		argsMap["label"] = deploymentUpdateLabel
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var nfsMountDAGDeploymentEnabled, gitSyncDAGDeploymentEnabled, dagOnlyDeployEnabled bool
	if appConfig != nil {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
		dagOnlyDeployEnabled = appConfig.Flags.DagOnlyDeployment
	}

	// we should validate only in case when this feature has been enabled
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		err := validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL, true)
		if err != nil {
			return err
		}
	}

	var executorType string
	var err error
	if executorUpdate != "" {
		executorType, err = validateExecutorArg(executorUpdate)
		if err != nil {
			return nil
		}
	}

	return deployment.Update(args[0], cloudRole, argsMap, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knowHosts, executorType, gitSyncInterval, updateTriggererReplicas, houstonClient, out)
}

func deploymentAirflowUpgrade(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if cancel {
		return deployment.AirflowUpgradeCancel(deploymentID, houstonClient, out)
	}
	return deployment.AirflowUpgrade(deploymentID, desiredAirflowVersion, houstonClient, out)
}

func deploymentRuntimeUpgrade(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if cancel {
		return deployment.RuntimeUpgradeCancel(deploymentID, houstonClient, out)
	}
	return deployment.RuntimeUpgrade(deploymentID, desiredRuntimeVersion, houstonClient, out)
}

func deploymentRuntimeMigrate(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if cancel {
		return deployment.RuntimeMigrateCancel(deploymentID, houstonClient, out)
	}
	return deployment.RuntimeMigrate(deploymentID, houstonClient, out)
}
