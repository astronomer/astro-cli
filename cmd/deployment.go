package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/input"
	sa "github.com/astronomer/astro-cli/serviceaccount"

	"github.com/spf13/cobra"
)

var (
	errUpdateDeploymentInvalidArgs = errors.New("must specify a deployment ID and at least one attribute to update")
	errServiceAccountNotPresent    = errors.New("must provide a service-account label with the --label (-l) flag")
)

var (
	allDeployments        bool
	cancel                bool
	hardDelete            bool
	executor              string
	deploymentID          string
	desiredAirflowVersion string
	email                 string
	fullName              string
	userID                string
	systemSA              bool
	category              string
	label                 string
	cloudRole             string
	releaseName           string
	nfsLocation           string
	dagDeploymentType     string
	triggererReplicas     int
	gitRevision           string
	gitRepoURL            string
	gitBranchName         string
	gitDAGDir             string
	gitSyncInterval       int
	sshKey                string
	knowHosts             string
	CreateExample         = `
# Create new deployment with Celery executor (default: celery without params).
  $ astro deployment create new-deployment-name --executor=celery

# Create new deployment with Local executor.
  $ astro deployment create new-deployment-name-local --executor=local

# Create new deployment with Kubernetes executor.
  $ astro deployment create new-deployment-name-k8s --executor=k8s

# Create new deployment with Kubernetes executor.
  $ astro deployment create my-new-deployment --executor=k8s --airflow-version=1.10.10
`
	createExampleDagDeployment = `
# Create new deployment with Kubernetes executor and dag deployment type volume and nfs location.
  $ astro deployment create my-new-deployment --executor=k8s --airflow-version=2.0.0 --dag-deployment-type=volume --nfs-location=test:/test
`
	deploymentUserListExample = `
# Search for deployment users
  $ astro deployment user list <deployment-id> --email=EMAIL_ADDRESS --user-id=ID --name=NAME
`
	deploymentUserCreateExample = `
# Add a workspace user to a deployment with a particular role
  $ astro deployment user add --deployment-id=xxxxx --role=DEPLOYMENT_ROLE <user-email-address>
`
	deploymentUserDeleteExample = `
# Delete user access to a deployment
	$ astro deployment user delete --deployment-id=xxxxx <user-email-address>
`
	deploymentUserUpdateExample = `
# Update a workspace user's deployment role
  $ astro deployment user update --deployment-id=xxxxx --role=DEPLOYMENT_ROLE <user-email-address>
`
	deploymentSaCreateExample = `
# Create service-account
  $ astro deployment service-account create --deployment-id=xxxxx --label=my_label --role=ROLE
`
	deploymentSaGetExample = `
  # Get deployment service-accounts
  $ astro deployment service-account get --deployment-id=<deployment-id>
`
	deploymentSaDeleteExample = `
  $ astro deployment service-account delete <service-account-id> --deployment-id=<deployment-id>
`
	deploymentAirflowUpgradeExample = `
  $ astro deployment airflow upgrade --deployment-id=<deployment-id> --desired-airflow-version=<desired-airflow-version>

# Abort the initial airflow upgrade step:
  $ astro deployment airflow upgrade --cancel --deployment-id=<deployment-id>
`

	deploymentUpdateAttrs = []string{"label"}
)

func newDeploymentRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de"},
		Short:   "Manage Astronomer Deployments",
		Long:    "Deployments are individual Airflow clusters running on an installation of the Astronomer platform.",
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	cmd.AddCommand(
		newDeploymentCreateCmd(client, out),
		newDeploymentListCmd(client, out),
		newDeploymentUpdateCmd(client, out),
		newDeploymentDeleteCmd(client, out),
		newLogsCmd(),
		newDeploymentSaRootCmd(client, out),
		newDeploymentUserRootCmd(client, out),
		newDeploymentAirflowRootCmd(client, out),
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentCreate(cmd, args, client, out)
		},
	}

	var nfsMountDAGDeploymentEnabled, triggererEnabled, gitSyncDAGDeploymentEnabled bool
	appConfig, err := deployment.AppConfig(client)
	if err != nil {
		fmt.Println("Error checking feature flag", err)
	} else {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		triggererEnabled = appConfig.Flags.TriggererEnabled
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
	}

	// let's hide under feature flag
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled {
		cmd.Flags().StringVarP(&dagDeploymentType, "dag-deployment-type", "t", "", "DAG Deployment mechanism: image, volume, git_sync")
	}

	if nfsMountDAGDeploymentEnabled {
		cmd.Example += createExampleDagDeployment
		cmd.Flags().StringVarP(&nfsLocation, "nfs-location", "n", "", "NFS Volume Mount, specified as: <IP>:/<path>. Input is automatically prepended with 'nfs://' - do not include.")
	}

	if gitSyncDAGDeploymentEnabled {
		addGitSyncDeploymentFlags(cmd)
	}

	if triggererEnabled {
		cmd.Flags().IntVarP(&triggererReplicas, "triggerer-replicas", "", 0, "Number of replicas to use for triggerer airflow component, valid 0-2")
	}

	cmd.Flags().StringVarP(&executor, "executor", "e", "", "Add executor parameter: local, celery, or kubernetes")
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "a", "", "Add desired airflow version parameter: e.g: 1.10.5 or 1.10.7")
	cmd.Flags().StringVarP(&releaseName, "release-name", "r", "", "Set custom release-name if possible")
	cmd.Flags().StringVarP(&cloudRole, "cloud-role", "c", "", "Set cloud role to annotate service accounts in deployment")
	return cmd
}

func newDeploymentDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete DEPLOYMENT",
		Aliases: []string{"de"},
		Short:   "Delete an airflow deployment",
		Long:    "Delete an airflow deployment",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentDelete(cmd, args, client, out)
		},
	}
	if deployment.CheckHardDeleteDeployment(client) {
		cmd.Flags().BoolVar(&hardDelete, "hard", false, "Deletes all infrastructure and records for this Deployment")
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
	example := `
# update executor for given deployment
$ astro deployment update UUID --executor=celery`
	updateExampleDagDeployment := `

# update dag deployment strategy
$ astro deployment update UUID --dag-deployment-type=volume --nfs-location=test:/test`
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update airflow deployments",
		Long:    "Update airflow deployments",
		Example: example,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errUpdateDeploymentInvalidArgs
			}
			return updateArgValidator(args, deploymentUpdateAttrs)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUpdate(cmd, args, dagDeploymentType, nfsLocation, client, out)
		},
	}

	var nfsMountDAGDeploymentEnabled, triggererEnabled, gitSyncDAGDeploymentEnabled bool
	appConfig, err := deployment.AppConfig(client)
	if err != nil {
		fmt.Println("Error checking feature flag", err)
	} else {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		triggererEnabled = appConfig.Flags.TriggererEnabled
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
	}

	cmd.Flags().StringVarP(&executor, "executor", "e", "", "Add executor parameter: local, celery, or kubernetes")

	// let's hide under feature flag
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled {
		cmd.Flags().StringVarP(&dagDeploymentType, "dag-deployment-type", "t", "", "DAG Deployment mechanism: image, volume, git_sync")
	}

	if nfsMountDAGDeploymentEnabled {
		cmd.Example += updateExampleDagDeployment
		cmd.Flags().StringVarP(&nfsLocation, "nfs-location", "n", "", "NFS Volume Mount, specified as: <IP>:/<path>. Input is automatically prepended with 'nfs://' - do not include.")
	}

	if triggererEnabled {
		cmd.Flags().IntVarP(&triggererReplicas, "triggerer-replicas", "", 0, "Number of replicas to use for triggerer airflow component, valid 0-2")
	}

	//noline:dupl
	if gitSyncDAGDeploymentEnabled {
		addGitSyncDeploymentFlags(cmd)
	}

	cmd.Flags().StringVarP(&cloudRole, "cloud-role", "c", "", "Set cloud role to annotate service accounts in deployment")
	return cmd
}

func addGitSyncDeploymentFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&gitRevision, "git-revision", "v", "", "Git revision (tag or hash) to check out")
	cmd.Flags().StringVarP(&gitRepoURL, "git-repository-url", "u", "", "The repository URL of the git repo")
	cmd.Flags().StringVarP(&gitBranchName, "git-branch-name", "b", "", "The Branch name of the git repo we will be syncing from")
	cmd.Flags().StringVarP(&gitDAGDir, "dag-directory-path", "p", "", "The directory where dags are stored in repo")
	cmd.Flags().IntVarP(&gitSyncInterval, "sync-interval", "s", 1, "The interval in seconds in which git-sync will be polling git for updates")
	cmd.Flags().StringVarP(&sshKey, "ssh-key", "", "", "Path to the ssh public key file to use to clone your git repo")
	cmd.Flags().StringVarP(&knowHosts, "known-hosts", "", "", "Path to the known hosts file to use to clone your git repo")
}

func newDeploymentUserRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage deployment user resources",
		Long:  "Users can be added or removed from deployment",
	}
	cmd.AddCommand(
		newDeploymentUserListCmd(client, out),
		newDeploymentUserAddCmd(client, out),
		newDeploymentUserDeleteCmd(client, out),
		newDeploymentUserUpdateCmd(client, out),
	)
	return cmd
}

func newDeploymentUserListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list USERS",
		Short:   "Search for deployment user's",
		Long:    "Search for deployment user's",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserList(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment assigned to user")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "[ID]")
	cmd.Flags().StringVarP(&email, "email", "e", "", "[EMAIL]")
	cmd.Flags().StringVarP(&fullName, "name", "n", "", "[NAME]")

	return cmd
}

// nolint:dupl
func newDeploymentUserAddCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add EMAIL",
		Short:   "Add a user to a deployment",
		Long:    "Add a user to a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserAdd(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment assigned to user")
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewer, "role assigned to user")
	return cmd
}

// nolint:dupl
func newDeploymentUserDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete EMAIL",
		Short:   "Delete a user from a deployment",
		Long:    "Delete a user from a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserDeleteExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserDelete(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment to remove user access")
	return cmd
}

// nolint:dupl
func newDeploymentUserUpdateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update EMAIL",
		Short:   "Update a user's role for a deployment",
		Long:    "Update a user's role for a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserUpdate(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "deployment assigned to user")
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewer, "role assigned to user")
	return cmd
}

func newDeploymentSaRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage astronomer service accounts",
		Long:    "Service-accounts represent a revokable token with access to the Astronomer platform",
	}
	cmd.AddCommand(
		newDeploymentSaCreateCmd(client, out),
		newDeploymentSaGetCmd(client, out),
		newDeploymentSaDeleteCmd(client, out),
	)
	return cmd
}

// nolint:dupl
func newDeploymentSaCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a service-account in the astronomer platform",
		Long:    "Create a service-account in the astronomer platform",
		Example: deploymentSaCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaCreate(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "[ID]")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "[ID]")
	cmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	cmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	cmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")
	cmd.Flags().StringVarP(&role, "role", "r", "viewer", "ROLE")
	return cmd
}

func newDeploymentSaGetCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a service-account by deployment id",
		Long:    "Get a service-account by deployment id",
		Example: deploymentSaGetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaGet(cmd, client, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "[ID]")
	err := cmd.MarkFlagRequired("deployment-id")
	if err != nil {
		fmt.Println("error adding deployment-id flag: ", err.Error())
	}
	return cmd
}

func newDeploymentSaDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [SA-ID]",
		Aliases: []string{"de"},
		Short:   "Delete a service-account in the astronomer platform",
		Long:    "Delete a service-account in the astronomer platform",
		Example: deploymentSaDeleteExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaDelete(cmd, args, client, out)
		},
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "[ID]")
	err := cmd.MarkFlagRequired("deployment-id")
	if err != nil {
		fmt.Println("error adding deployment-id flag: ", err.Error())
	}
	return cmd
}

func newDeploymentAirflowRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow",
		Aliases: []string{"ai"},
		Short:   "Manage airflow deployments",
		Long:    "Manage airflow deployments",
	}
	cmd.AddCommand(
		newDeploymentAirflowUpgradeCmd(client, out),
	)
	return cmd
}

func newDeploymentAirflowUpgradeCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"up"},
		Short:   "Upgrade Airflow version",
		Long:    "Upgrade Airflow version",
		Example: deploymentAirflowUpgradeExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowUpgrade(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "[ID]")
	cmd.Flags().StringVarP(&desiredAirflowVersion, "desired-airflow-version", "v", "", "[DESIRED_AIRFLOW_VERSION]")
	cmd.Flags().BoolVarP(&cancel, "cancel", "c", false, "Abort the initial airflow upgrade step")
	err := cmd.MarkFlagRequired("deployment-id")
	if err != nil {
		fmt.Println("error adding deployment-id flag: ", err.Error())
	}
	return cmd
}

func deploymentCreate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
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

	var nfsMountDAGDeploymentEnabled, gitSyncDAGDeploymentEnabled bool
	appConfig, err := deployment.AppConfig(client)
	if err != nil {
		fmt.Println("Error checking feature flag", err)
	} else {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
	}

	// we should validate only in case when this feature has been enabled
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled {
		err = validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL, false)
		if err != nil {
			return err
		}
	}

	return deployment.Create(args[0], ws, releaseName, cloudRole, executorType, airflowVersion, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knowHosts, gitSyncInterval, triggererReplicas, client, out)
}

func deploymentDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if hardDelete {
		i, _ := input.Confirm(
			fmt.Sprintf(messages.CLIDeploymentHardDeletePrompt))

		if !i {
			fmt.Println("Exit: This command was not executed and your Deployment was not hard deleted.\n If you want to delete your Deployment but not permanently, try\n $ astro deployment delete without the --hard flag.")
			return nil
		}
	}
	return deployment.Delete(args[0], hardDelete, client, out)
}

func deploymentList(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
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

	return deployment.List(ws, allDeployments, client, out)
}

func deploymentUpdate(cmd *cobra.Command, args []string, dagDeploymentType, nfsLocation string, client *houston.Client, out io.Writer) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var nfsMountDAGDeploymentEnabled, gitSyncDAGDeploymentEnabled bool
	appConfig, err := deployment.AppConfig(client)
	if err != nil {
		fmt.Println("Error checking feature flag", err)
	} else {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
	}

	// we should validate only in case when this feature has been enabled
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled {
		err = validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL, true)
		if err != nil {
			return err
		}
	}

	var executorType string
	if executor != "" {
		executorType, err = validateExecutorArg(executor)
		if err != nil {
			return nil
		}
	}

	return deployment.Update(args[0], cloudRole, argsMap, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knowHosts, executorType, gitSyncInterval, triggererReplicas, client, out)
}

func deploymentUserList(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UserList(args[0], email, userID, fullName, client, out)
}

func deploymentUserAdd(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.Add(deploymentID, args[0], deploymentRole, client, out)
}

func deploymentUserDelete(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.DeleteUser(deploymentID, args[0], client, out)
}

func deploymentUserUpdate(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateDeploymentRole(deploymentRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UpdateUser(deploymentID, args[0], deploymentRole, client, out)
}

func deploymentSaCreate(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
	if label == "" {
		return errServiceAccountNotPresent
	}

	if err := validateRole(role); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}
	fullRole := strings.Join([]string{"DEPLOYMENT", strings.ToUpper(role)}, "_")
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.CreateUsingDeploymentUUID(deploymentID, label, category, fullRole, client, out)
}

func deploymentSaGet(cmd *cobra.Command, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.GetDeploymentServiceAccounts(deploymentID, client, out)
}

func deploymentSaDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.DeleteUsingDeploymentUUID(args[0], deploymentID, client, out)
}

func deploymentAirflowUpgrade(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	if cancel {
		return deployment.AirflowUpgradeCancel(deploymentID, client, out)
	}
	return deployment.AirflowUpgrade(deploymentID, desiredAirflowVersion, client, out)
}
