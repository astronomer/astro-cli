package apc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

const (
	celeryExecutorArg     = "celery"
	localExecutorArg      = "local"
	kubernetesExecutorArg = "kubernetes"
	k8sExecutorArg        = "k8s"

	cliDeploymentHardDeletePrompt              = "This permanently deletes all data in this Deployment, including the database. You cannot recover it. Delete the Deployment?"
	cliDeploymentUnadoptPrompt                 = "This removes the Deployment record from APC. The Airflow custom resource, its namespace, and its metadata database are left untouched. This cannot be undone from the CLI. Unadopt the Deployment?"
	deploymentTypeCmdMessage                   = "Dag Deployment mechanism: image, volume, git_sync, dag_deploy"
	CreateDeploymentWithTypeDagDeployPromptMsg = "\nDag-only deploys are experimental; use with caution. See the APC documentation at " + houston.DagDeployDocsLink + " for details. Create the Deployment?"
	UpdateDeploymentTypeToDagDeployPromptMsg   = "\nDag-only deploys are experimental; use with caution. Changing to a Dag-only Deployment erases all currently deployed Dags. Redeploy your Dags to keep them running. See the APC documentation at " + houston.DagDeployDocsLink + " for details. Change the Deployment type?"
	UpdateDeploymentTypeFromDagDeployPromptMsg = "\nChanging from a Dag-only Deployment erases all currently deployed Dags. Redeploy your Dags to keep them running. See the APC documentation at " + houston.DeployViaCLIDocsLink + " for details. Change the Deployment type?"
	SkipUserPromptMsgForCreateDeployment       = "Skip the confirmation prompt when creating a Deployment with type dag_deploy (experimental)"
	SkipUserPromptMsgForUpdateDeployment       = "Skip the confirmation prompt when changing a Deployment's type to or from dag_deploy (experimental)"
)

var (
	skipPrompt                  bool
	deploymentOutput            cliout.Format
	allDeployments              bool
	cancel                      bool
	executor                    string
	airflowVersion              string
	deploymentCreateName        string
	deploymentUpdateName        string
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
	clusterID               string
	deploymentMode          string
	createNamespace         string

	// errNoAppConfig is a create with no platform settings to read: the
	// lookup when the tree was built failed, and none was asked for since.
	errNoAppConfig = errors.New("could not read this platform's settings for a new Deployment; check your login and try again")

	hibernateUntil       string
	hibernateForDuration string
	hibernateRemove      bool

	adoptCRName                  string
	adoptNamespace               string
	adoptDeploymentName          string
	adoptDescription             string
	adoptUseApcLogging           bool
	adoptUseApcRegistry          bool
	adoptAcceptIncompatibilities bool

	deploymentAdoptExample = `  # Adopt an Airflow custom resource into the current Workspace
  astro deployment adopt --cluster-id=<CLUSTER_ID> --cr-name=<CR_NAME> --namespace=<CR_NAMESPACE>

  # Adopt it into another Workspace
  astro deployment adopt --cluster-id=<CLUSTER_ID> --cr-name=<CR_NAME> --namespace=<CR_NAMESPACE> \
    --workspace=<WORKSPACE_ID>`
	deploymentUnadoptExample = `  # Release an adopted Deployment
  astro deployment unadopt --deployment=<DEPLOYMENT_ID>`
	deploymentCreateExample = `  # Create a Deployment with the Celery executor
  astro deployment create --name=my-deployment

  # Create one with the Kubernetes executor and an Airflow version
  astro deployment create --name=my-deployment --executor=kubernetes --airflow-version=2.4.1

  # Create one on Astronomer Runtime
  astro deployment create --name=my-deployment --executor=kubernetes --runtime-version=6.0.1`
	deploymentCreateExampleAPCV1 = `  # Create a Deployment with the Celery executor
  astro deployment create --name=my-deployment --cluster-id=<CLUSTER_ID>

  # Create one with the Kubernetes executor and an Airflow version
  astro deployment create --name=my-deployment --executor=kubernetes --airflow-version=2.4.1 \
    --cluster-id=<CLUSTER_ID>

  # Create one on Astronomer Runtime
  astro deployment create --name=my-deployment --executor=kubernetes --runtime-version=6.0.1 \
    --cluster-id=<CLUSTER_ID>`
	createExampleOperatorMode = `

  # Create an operator-managed Deployment
  astro deployment create --name=my-deployment --executor=kubernetes --cluster-id=<CLUSTER_ID> \
    --mode=operator`

	createExampleDagDeployment = `

  # Create one that reads its Dags from an NFS volume
  astro deployment create --name=my-deployment --executor=kubernetes --airflow-version=2.4.1 \
    --dag-deployment-type=volume --nfs-location=<IP>:/<PATH>`
	createExampleDagDeploymentAPCV1 = `

  # Create one that reads its Dags from an NFS volume
  astro deployment create --name=my-deployment --executor=kubernetes --airflow-version=2.4.1 \
    --dag-deployment-type=volume --nfs-location=<IP>:/<PATH> --cluster-id=<CLUSTER_ID>`

	deploymentAirflowUpgradeExample = `  # Start upgrading a Deployment's Airflow
  astro deployment airflow upgrade --deployment=<DEPLOYMENT_ID> \
    --desired-airflow-version=<AIRFLOW_VERSION>

  # Cancel an upgrade that has not finished
  astro deployment airflow upgrade --deployment=<DEPLOYMENT_ID> --cancel`
	deploymentRuntimeUpgradeExample = `  # Start upgrading a Deployment's Runtime
  astro deployment runtime upgrade --deployment=<DEPLOYMENT_ID> \
    --desired-runtime-version=<RUNTIME_VERSION>

  # Cancel an upgrade that has not finished
  astro deployment runtime upgrade --deployment=<DEPLOYMENT_ID> --cancel`
	deploymentRuntimeMigrateExample = `  # Start migrating a Deployment to Astronomer Runtime
  astro deployment runtime migrate --deployment=<DEPLOYMENT_ID>

  # Cancel a migration that has not finished
  astro deployment runtime migrate --deployment=<DEPLOYMENT_ID> --cancel`
)

func newDeploymentRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Aliases: []string{"de", "deployments"},
		Short:   "Manage APC Deployments",
		Long:    "Deployments are individual Airflow clusters running on an installation of the APC platform.",
	}
	deleteCmd := newDeploymentDeleteCmd(out)
	cmd.AddCommand(
		newDeploymentCreateCmd(out),
		newDeploymentListCmd(out),
		newDeploymentUpdateCmd(out),
		deleteCmd,
		newDeploymentHibernateCmd(out),
		newDeploymentWakeUpCmd(out),
		newDeploymentAdoptCmd(out),
		newDeploymentUnadoptCmd(out),
		newLogsCmd(out),
		newDeploymentSaRootCmd(out),
		newDeploymentUserRootCmd(out),
		newDeploymentAirflowRootCmd(out),
		newDeploymentTeamRootCmd(out),
	)
	cmd.AddCommand(cliout.RemovedAlias("de", deleteCmd))

	if appConfig != nil && appConfig.Flags.AstroRuntimeEnabled {
		cmd.AddCommand(newDeploymentRuntimeRootCmd(out))
	}

	// --workspace is where create, list and adopt act, with 1.x's
	// --workspace-id as its alias. 1.x gave --workspace-id to every
	// deployment command, through this group, and no other one read it: those
	// keep it, hidden and read by nothing, so a 1.x script passing it keeps
	// working. hibernate and wake-up are newer than 1.x, so they take none.
	var adopt *cobra.Command
	for _, c := range cmd.Commands() {
		switch c.Name() {
		case "create":
			addWorkspaceFlag(c.Flags(), "Workspace to create the Deployment in (default: the current context's)", "w")
		case "list":
			addWorkspaceFlag(c.Flags(), "Workspace whose Deployments to list (default: the current context's)", "w")
		case "adopt":
			adopt = c
			addWorkspaceFlag(c.Flags(), "Workspace to adopt the Deployment into (default: the current context's)", "w")
		case "update", "delete", "unadopt", "logs", "service-account", "user", "team", "airflow", "runtime":
			fs := c.Flags()
			if c.HasSubCommands() {
				fs = c.PersistentFlags()
			}
			utils.AddOneXNoOp(fs, "workspace-id")
		}
	}

	applyPreferredFlagsIn(cmd)
	// BeforeArgs runs the hook installed last first, so this one, installed
	// after applyPreferredFlagsIn, runs ahead of it: see refuseOneXAdoptName.
	// TestDeploymentNameFlag's 1.x adopt lines fail if the order turns round.
	if adopt != nil {
		utils.BeforeArgs(adopt, refuseOneXAdoptName)
	}
	return cmd
}

// refuseOneXAdoptName refuses an adopt that gives --name but names no custom
// resource: 1.x's --name was the custom resource's, and in v2 it is the
// adopted Deployment's, as on create, so such a line is a 1.x one. It runs
// ahead of the reconciliation of --name with 1.x's --label, whose refusal of
// a 1.x line giving both would say to keep the custom resource's name as the
// Deployment's. A line giving --label alone names no custom resource either,
// and cobra's required-flag error says so.
func refuseOneXAdoptName(cmd *cobra.Command, _ []string) error {
	fs := cmd.Flags()
	if fs.Changed("cr-name") || !fs.Changed("name") {
		return nil
	}
	return cliout.Usage(errors.New("use --cr-name to name the custom resource; --name names the Deployment"))
}

func newDeploymentCreateCmd(out io.Writer) *cobra.Command {
	example := deploymentCreateExample
	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "1.0.0"}) {
		example = deploymentCreateExampleAPCV1
	}
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an APC Deployment",
		Long:    "Create an APC Deployment.",
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentCreate(cmd, out)
		},
	}

	addYesFlag(cmd, SkipUserPromptMsgForCreateDeployment)

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
		cmd.Flags().StringVar(&dagDeploymentType, "dag-deployment-type", "", deploymentTypeCmdMessage)
	}

	if nfsMountDAGDeploymentEnabled {
		if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "1.0.0"}) {
			cmd.Example += createExampleDagDeploymentAPCV1
		} else {
			cmd.Example += createExampleDagDeployment
		}
		cmd.Flags().StringVar(&nfsLocation, "nfs-location", "", "NFS volume to mount, as <IP>:/<path>, without the nfs:// prefix the CLI adds")
	}

	if gitSyncDAGDeploymentEnabled {
		addGitSyncDeploymentFlags(cmd)
	}

	if triggererEnabled {
		cmd.Flags().IntVar(&createTriggererReplicas, "triggerer-replicas", 0, "Number of triggerer replicas, 0 to 2")
	}

	if runtimeEnabled {
		cmd.Flags().StringVar(&runtimeVersion, "runtime-version", "", "Astro Runtime version, for example 4.2.2 or 5.0.0")
	}

	// No -n: 1.x's -n here was --nfs-location, so a 1.x line passing the mount
	// with it is refused (removedFlags), not read as the Deployment's name.
	addDeploymentNameFlag(cmd, &deploymentCreateName, "Name of the Deployment", "")
	cmd.Flags().StringVar(&executor, "executor", celeryExecutorArg, "Executor: local, celery, or kubernetes (or LocalExecutor, CeleryExecutor, KubernetesExecutor)")
	cmd.Flags().StringVar(&airflowVersion, "airflow-version", "", "Airflow version, for example 1.10.5 or 1.10.7")
	cmd.Flags().StringVar(&releaseName, "release-name", "", "Custom release name, if the platform allows one")
	cmd.Flags().StringVar(&cloudRole, "cloud-role", "", "Cloud role to annotate service accounts with in the Deployment")
	cmd.Flags().StringVar(&createNamespace, "namespace", "", "Kubernetes namespace for the Deployment, where the platform asks for one: one it offers, or a name of your own (lowercase letters, digits and '-', at most 63)")

	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "1.0.0"}) {
		cmd.Flags().StringVar(&clusterID, "cluster-id", "", "Cluster to create the Deployment in")
		_ = cmd.MarkFlagRequired("cluster-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	}

	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "2.1.0"}) {
		cmd.Flags().StringVar(&deploymentMode, "mode", "", "Deployment mode, one of: helm (default), operator")
		cmd.Example += createExampleOperatorMode
	}
	_ = cmd.MarkFlagRequired("name") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentDeleteCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete a Deployment",
		Long:    "Delete a Deployment. Name it by its ID or with --deployment, which also takes a link name from the project's manifest.",
		Example: `  # Delete a Deployment and all of its data, after you confirm
  astro deployment delete <DEPLOYMENT_ID>

  # Delete the one a link of this project names
  astro deployment delete --deployment <LINK_NAME>

  # Delete it without the confirmation, from a script
  astro deployment delete <DEPLOYMENT_ID> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentDelete(cmd, out)
		},
	}, cliout.Optional("DEPLOYMENT_ID"))
	addDeploymentArgFlag(cmd)
	addYesFlag(cmd, "Delete without asking for confirmation")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentHibernateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "hibernate",
		Aliases: []string{"hb"},
		Short:   "Hibernate an APC Deployment",
		Long: "Hibernate an APC Deployment. Overrides its hibernation schedule, if it has one, and holds it hibernating until you wake it up, " +
			"for a duration, or until a time. Use --remove-override to remove the override and resume the regular hibernation schedule. " +
			"Needs Astro Private Cloud " + deployment.HibernationSince + " or later, with hibernation enabled for the Deployment.",
		Example: `  # Hibernate a Deployment until you wake it up
  astro deployment hibernate <DEPLOYMENT_ID>

  # Hibernate it for two and a half hours
  astro deployment hibernate <DEPLOYMENT_ID> --for 2h30m

  # Go back to its regular hibernation schedule
  astro deployment hibernate <DEPLOYMENT_ID> --remove-override

  # Hibernate the one a link of this project names
  astro deployment hibernate --deployment <LINK_NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentHibernate(cmd, true, out)
		},
	}, cliout.Optional("DEPLOYMENT_ID"))
	addHibernationFlags(cmd, "hibernation", "hibernating")
	return cmd
}

func newDeploymentWakeUpCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "wake-up",
		Aliases: []string{"wu"},
		Short:   "Wake up an APC Deployment",
		Long: "Wake up an APC Deployment from hibernation. Overrides its hibernation schedule, if it has one, and holds it awake until you hibernate it, " +
			"for a duration, or until a time. Use --remove-override to remove the override and resume the regular hibernation schedule. " +
			"Needs Astro Private Cloud " + deployment.HibernationSince + " or later, with hibernation enabled for the Deployment.",
		Example: `  # Wake up a hibernating Deployment until you hibernate it
  astro deployment wake-up <DEPLOYMENT_ID>

  # Keep it awake for four hours
  astro deployment wake-up <DEPLOYMENT_ID> --for 4h

  # Go back to its regular hibernation schedule
  astro deployment wake-up <DEPLOYMENT_ID> --remove-override

  # Wake up the one a link of this project names
  astro deployment wake-up --deployment <LINK_NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentHibernate(cmd, false, out)
		},
	}, cliout.Optional("DEPLOYMENT_ID"))
	addHibernationFlags(cmd, "awake period", "waking up")
	return cmd
}

// addHibernationFlags gives hibernate or wake-up its flags: period names
// what --until and --for end, and doing what --yes skips the question for.
func addHibernationFlags(cmd *cobra.Command, period, doing string) {
	addDeploymentArgFlagWithoutLetter(cmd) // 1.x gave -d to --for here
	cmd.Flags().StringVar(&hibernateUntil, "until", "", "End the "+period+" at this date and time, in RFC 3339. Example value: 2026-01-01T00:00:00Z")
	cmd.Flags().StringVar(&hibernateForDuration, "for", "", "End the "+period+" after this long. Example value: 1h30m")
	cmd.Flags().BoolVar(&hibernateRemove, "remove-override", false, "Remove any existing override and resume the regular hibernation schedule")
	addYesFlag(cmd, "Don't ask for confirmation before "+doing+" the Deployment")
	cmd.MarkFlagsMutuallyExclusive("until", "for", "remove-override")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
}

func newDeploymentAdoptCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "adopt",
		Short:   "Adopt an existing operator-managed Airflow custom resource into APC",
		Long:    "Adopt an existing operator-managed Airflow custom resource into APC. The custom resource, its namespace, and its metadata database are left untouched; adopting only creates the corresponding Deployment record in APC.",
		Example: deploymentAdoptExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAdopt(out, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster-id", "", "ID of the cluster the Airflow custom resource is running on")
	// --name is the Deployment's, as on create, with 1.x's --label its alias.
	// 1.x's --name was the custom resource's, which is --cr-name now, and
	// required: a 1.x line names none, and refuseOneXAdoptName says why.
	cmd.Flags().StringVar(&adoptCRName, "cr-name", "", "Name (metadata.name) of the existing Airflow custom resource (CR)")
	cmd.Flags().StringVar(&adoptNamespace, "namespace", "", "Kubernetes namespace of the existing Airflow custom resource (CR)")
	addDeploymentNameFlag(cmd, &adoptDeploymentName, "Name of the adopted Deployment (default: the --cr-name value)", "n")
	cmd.Flags().StringVar(&adoptDescription, "description", "", "Description for the adopted Deployment")
	cmd.Flags().BoolVar(&adoptUseApcLogging, "use-apc-logging", false, "Route the adopted Deployment's logs through APC logging")
	cmd.Flags().BoolVar(&adoptUseApcRegistry, "use-apc-registry", false, "Use the APC in-cluster registry for the adopted Deployment; its images must be synced beforehand")
	cmd.Flags().BoolVar(&adoptAcceptIncompatibilities, "accept-incompatibilities", true, "Adopt even if the custom resource has fields with no APC representation")
	_ = cmd.MarkFlagRequired("cluster-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	_ = cmd.MarkFlagRequired("cr-name")    //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	_ = cmd.MarkFlagRequired("namespace")  //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentUnadoptCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "unadopt",
		Short:   "Release an adopted Deployment back to operator-only management",
		Long:    "Release an adopted Deployment back to operator-only management. The Airflow custom resource, its namespace, and its metadata database are left untouched; only the APC Deployment record is removed.",
		Example: deploymentUnadoptExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUnadopt(out, cmd.ErrOrStderr())
		},
	}
	addDeploymentFlag(cmd.Flags(), "Adopted Deployment to release: a link name from the project's manifest, or a Deployment id", "d", true)
	_ = cmd.MarkFlagRequired("deployment") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	addYesFlag(cmd, "Release the Deployment without asking for confirmation")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Deployments",
		Long:    "List the Deployments in the current Workspace, or in all Workspaces with --all.",
		Example: `  # List the Deployments in the current Workspace
  astro deployment list

  # List Deployments across all Workspaces
  astro deployment list --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentList(out, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVarP(&allDeployments, "all", "a", false, "Show Deployments across all Workspaces")
	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "1.0.0"}) {
		cmd.Flags().StringVar(&clusterID, "cluster-id", "", "Show Deployments from the specified cluster")
	}
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentUpdateCmd(out io.Writer) *cobra.Command {
	example := `  # Change a Deployment's executor
  astro deployment update <DEPLOYMENT_ID> --executor=celery

  # Change its name
  astro deployment update <DEPLOYMENT_ID> --name=my-deployment

  # Change the name of the one a link of this project names
  astro deployment update --deployment <LINK_NAME> --name=my-deployment`
	updateExampleDagDeployment := `

  # Read its Dags from an NFS volume
  astro deployment update <DEPLOYMENT_ID> --dag-deployment-type=volume --nfs-location=<IP>:/<PATH>`
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update a Deployment",
		Long:    "Update a Deployment. Name it by its ID or with --deployment, which also takes a link name from the project's manifest.",
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUpdate(cmd, dagDeploymentType, nfsLocation, out)
		},
	}, cliout.Optional("DEPLOYMENT_ID"))
	addDeploymentArgFlagWithoutLetter(cmd) // 1.x gave -d to --description here

	addYesFlag(cmd, SkipUserPromptMsgForUpdateDeployment)

	var nfsMountDAGDeploymentEnabled, triggererEnabled, gitSyncDAGDeploymentEnabled, dagOnlyDeployEnabled bool
	if appConfig != nil {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		triggererEnabled = appConfig.Flags.TriggererEnabled
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
		dagOnlyDeployEnabled = appConfig.Flags.DagOnlyDeployment
	}

	cmd.Flags().StringVar(&executorUpdate, "executor", "", "Executor: local, celery, or kubernetes (or LocalExecutor, CeleryExecutor, KubernetesExecutor)")

	// let's hide under feature flag
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		cmd.Flags().StringVar(&dagDeploymentType, "dag-deployment-type", "", deploymentTypeCmdMessage)
	}

	if nfsMountDAGDeploymentEnabled {
		cmd.Example += updateExampleDagDeployment
		cmd.Flags().StringVar(&nfsLocation, "nfs-location", "", "NFS volume to mount, as <IP>:/<path>, without the nfs:// prefix the CLI adds")
	}

	if triggererEnabled {
		cmd.Flags().IntVar(&updateTriggererReplicas, "triggerer-replicas", -1, "Number of triggerer replicas, 0 to 2")
	}

	//noline:dupl
	if gitSyncDAGDeploymentEnabled {
		addGitSyncDeploymentFlags(cmd)
	}

	cmd.Flags().StringVar(&deploymentUpdateDescription, "description", "", "New description for the Deployment")
	// -n, as on Astro: see create.
	addDeploymentNameFlag(cmd, &deploymentUpdateName, "The new name for the deployment", "")
	cmd.Flags().StringVar(&cloudRole, "cloud-role", "", "Cloud role to annotate service accounts with in the Deployment")
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func addGitSyncDeploymentFlags(cmd *cobra.Command) {
	const defaultSyncInterval = 60

	cmd.Flags().StringVar(&gitRevision, "git-revision", "", "Git revision (tag or hash) to check out")
	cmd.Flags().StringVar(&gitRepoURL, "git-repository-url", "", "Git repository URL")
	cmd.Flags().StringVar(&gitBranchName, "git-branch-name", "", "Git branch to sync from")
	cmd.Flags().StringVar(&gitDAGDir, "dag-directory-path", "", "Directory in the repository where Dags are stored")
	cmd.Flags().IntVar(&gitSyncInterval, "sync-interval", defaultSyncInterval, "Seconds between git-sync polls for updates")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to the SSH public key file used to clone the repository")
	cmd.Flags().StringVar(&knowHosts, "known-hosts", "", "Path to the known hosts file used to clone the repository")
}

func newDeploymentAirflowRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow",
		Aliases: []string{"ai"},
		Short:   "Manage a Deployment's Airflow version",
		Long:    "Manage the Airflow version of a Deployment.",
	}
	cmd.AddCommand(
		newDeploymentAirflowUpgradeCmd(out),
	)
	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentAirflowUpgradeCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"up"},
		Short:   "Upgrade the Airflow version of a Deployment",
		Long:    "Upgrade the Airflow version of a Deployment.",
		Example: deploymentAirflowUpgradeExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowUpgrade(out, cmd.ErrOrStderr())
		},
	}
	addDeploymentFlag(cmd.Flags(), "Deployment to upgrade: a link name from the project's manifest, or a Deployment id", "d", true)
	cmd.Flags().StringVar(&desiredAirflowVersion, "desired-airflow-version", "", "Airflow version to upgrade to")
	cmd.Flags().BoolVar(&cancel, "cancel", false, "Cancel the first step of the Airflow upgrade")
	_ = cmd.MarkFlagRequired("deployment") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentRuntimeRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "runtime",
		Aliases: []string{"r"},
		Short:   "Manage a Deployment's Astro Runtime",
		Long:    "Manage the Astro Runtime version of a Deployment.",
	}
	cmd.AddCommand(
		newDeploymentRuntimeUpgradeCmd(out),
		newDeploymentRuntimeMigrateCmd(out),
	)
	return cmd
}

func newDeploymentRuntimeUpgradeCmd(out io.Writer) *cobra.Command { //nolint:dupl // the duplication is acceptable here
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"up"},
		Short:   "Upgrade the Runtime version of a Deployment",
		Long:    "Upgrade the Astro Runtime version of a Deployment.",
		Example: deploymentRuntimeUpgradeExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return deploymentRuntimeUpgrade(out, cmd.ErrOrStderr())
		},
	}
	addDeploymentFlag(cmd.Flags(), "Deployment to upgrade: a link name from the project's manifest, or a Deployment id", "d", true)
	cmd.Flags().StringVar(&desiredRuntimeVersion, "desired-runtime-version", "", "Runtime version to upgrade the Deployment to")
	cmd.Flags().BoolVar(&cancel, "cancel", false, "Cancel the first step of the Runtime upgrade")
	_ = cmd.MarkFlagRequired("deployment") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

func newDeploymentRuntimeMigrateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "migrate",
		Aliases: []string{"m"},
		Short:   "Migrate a Deployment to an Astro Runtime image",
		Long:    "Migrate a Deployment to an Astro Runtime image.",
		Example: deploymentRuntimeMigrateExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return deploymentRuntimeMigrate(out, cmd.ErrOrStderr())
		},
	}
	addDeploymentFlag(cmd.Flags(), "Deployment to migrate: a link name from the project's manifest, or a Deployment id", "d", true)
	cmd.Flags().BoolVar(&cancel, "cancel", false, "Cancel the first step of the Runtime migration")
	_ = cmd.MarkFlagRequired("deployment") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &deploymentOutput)
	return cmd
}

// addDeploymentNameFlag registers the Deployment's name on cmd as --name, with
// the shorthand given ("n", or "" on create and update, where 1.x's -n was
// --nfs-location), read into to, and 1.x's --label as its hidden alias.
func addDeploymentNameFlag(cmd *cobra.Command, to *string, usage, shorthand string) {
	*to = ""
	cmd.Flags().StringVarP(to, "name", shorthand, "", usage)
	utils.AddAlias(cmd.Flags(), "label", "name")
}

func deploymentCreate(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: cmd.ErrOrStderr()}

	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("could not find a valid Workspace: %w", err)
	}

	// The settings that decide create's questions (namespace management, DAG
	// deploy mechanisms) are resolved per cluster and workspace, so a
	// --namespace is judged against those, not the defaults the tree was built
	// with. A failed lookup used to be dropped, leaving no settings to read.
	if clusterID != "" || createNamespace != "" {
		appConfig, err = houston.Call(houstonClient.GetAppConfig)(houston.GetAppConfigRequest{ClusterID: clusterID, WorkspaceUUID: ws})
		if err != nil {
			return fmt.Errorf("could not get the platform config: %w", err)
		}
	}
	if appConfig == nil {
		return errNoAppConfig
	}

	executorType, err := validateExecutorArg(executor)
	if err != nil {
		return err
	}

	if err := validateDeploymentModeArg(deploymentMode); err != nil {
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

	// If it's a dag_deploy type, confirm it with the person first.
	if dagDeploymentType == houston.DagOnlyDeploymentType {
		if err := confirm(CreateDeploymentWithTypeDagDeployPromptMsg); err != nil {
			return err
		}
	}

	// we should validate only in case when this feature has been enabled
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		err = validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL, false)
		if err != nil {
			return err
		}
	}
	req := &deployment.CreateDeploymentRequest{
		Label:             deploymentCreateName,
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
		ClusterID:         clusterID,
		Mode:              deploymentMode,
		Namespace:         createNamespace,
	}
	d, err := deployment.Create(req, houstonClient, appConfig)
	if err != nil {
		return err
	}
	return emitCreated(r, d, executorType)
}

// hibernationNow is the clock hibernate and wake-up read: when --until is
// checked against, and when a --for override starts. Tests replace it.
var hibernationNow = time.Now

// deploymentHibernate sets, or with --remove-override removes, the override
// of a Deployment's hibernation schedule: hibernate holds it hibernating, and
// wake-up (hibernate false) awake.
func deploymentHibernate(cmd *cobra.Command, hibernate bool, out io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: cmd.ErrOrStderr()}
	if err := requireDeployment(cmd); err != nil {
		return err
	}
	end, err := hibernationEnd(hibernateUntil, hibernateForDuration)
	if err != nil {
		return err
	}

	id := deploymentID
	if err := deployment.CheckHibernation(id, houstonClient); err != nil {
		return err
	}
	verb, action := "wake up", "Wake up"
	if hibernate {
		verb, action = "hibernate", "Hibernate"
	}
	question := fmt.Sprintf("\n%s this Deployment?", action)
	if hibernateRemove {
		question = "\nRemove the hibernation override on this Deployment?"
	}
	if err := confirm(question); err != nil {
		return err
	}

	var d *houston.Deployment
	if hibernateRemove {
		d, err = deployment.RemoveHibernationOverride(id, houstonClient)
	} else {
		// A --for counts from now, not from before the question was
		// answered.
		d, err = deployment.SetHibernationOverride(id, hibernate, end(), houstonClient)
	}
	if err != nil {
		return err
	}
	return emitHibernation(r, newHibernationJSON(id, d), verb, hibernateRemove)
}

// hibernationEnd checks --until and --for, and returns what gives the end of
// the override they set: nil for one that lasts until it is removed. A --for
// end is computed when that is called, so it counts from the request, not
// from the check. A --until not in the future, or a --for that is not
// positive, is a usage error: Houston would refuse the first, and the second
// would set an override that has already ended.
func hibernationEnd(until, forDuration string) (func() *time.Time, error) {
	switch {
	case until != "":
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return nil, cliout.Usage(fmt.Errorf("--until takes a date and time in RFC 3339, such as 2026-01-01T00:00:00Z: %w", err))
		}
		if !t.After(hibernationNow()) {
			return nil, cliout.Usage(fmt.Errorf("--until %s is not in the future", until))
		}
		return func() *time.Time { return &t }, nil
	case forDuration != "":
		d, err := time.ParseDuration(forDuration)
		if err != nil {
			return nil, cliout.Usage(fmt.Errorf("--for takes a duration, such as 1h30m: %w", err))
		}
		if d <= 0 {
			return nil, cliout.Usage(fmt.Errorf("--for %s is not a positive duration", forDuration))
		}
		return func() *time.Time {
			t := hibernationNow().Add(d)
			return &t
		}, nil
	}
	return func() *time.Time { return nil }, nil
}

func deploymentDelete(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: cmd.ErrOrStderr()}
	if err := requireDeployment(cmd); err != nil {
		return err
	}
	// Deletions are always hard deletes now (PLX-575): all data associated with
	// the Deployment, including the database, is permanently removed. Confirm
	// before proceeding, unless --yes already did.
	prompt := dangerPrompt(cliDeploymentHardDeletePrompt)
	if deploymentLink != "" {
		// The run named a link: say which Deployment that is before
		// asking to delete it.
		prompt = fmt.Sprintf("\nLink %s names Deployment %s.", deploymentLink, deploymentID) + prompt
	}
	if err := confirm(prompt); err != nil {
		return err
	}
	d, err := deployment.Delete(deploymentID, true, houstonClient)
	if err != nil {
		return err
	}
	return r.Emit(newRemovalJSON(deploymentID, d, "deleted"), cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, "\n Deleted Deployment")
	}))
}

func deploymentAdopt(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: errOut}

	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("could not find a valid Workspace: %w", err)
	}

	req := &houston.AdoptDeploymentRequest{
		WorkspaceID:             ws,
		ClusterID:               clusterID,
		CRNamespace:             adoptNamespace,
		CRName:                  adoptCRName,
		Label:                   adoptDeploymentName,
		Description:             adoptDescription,
		UseApcLogging:           adoptUseApcLogging,
		UseApcRegistry:          adoptUseApcRegistry,
		AcceptIncompatibilities: adoptAcceptIncompatibilities,
	}
	d, err := deployment.Adopt(req, houstonClient)
	if err != nil {
		return err
	}
	return emitAdopted(r, d)
}

// dangerPrompt leads a confirmation of something that cannot be undone with
// the red WARNING: label, on its own line. The question is asked on stderr,
// so the label is colored by stderr's terminal.
func dangerPrompt(text string) string {
	return "\n" + cliout.DangerLabel(os.Stderr) + " " + text
}

func deploymentUnadopt(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: errOut}

	if err := confirm(dangerPrompt(cliDeploymentUnadoptPrompt)); err != nil {
		return err
	}
	d, err := deployment.Unadopt(deploymentID, houstonClient)
	if err != nil {
		return err
	}
	return r.Emit(newRemovalJSON(deploymentID, d, "unadopted"), cliout.Text(func(b *bufio.Writer) {
		// Houston returns the record it removed; without one, the id given
		// is what is known.
		if d == nil {
			fmt.Fprintf(b, "\n Unadopted Deployment %s\n", deploymentID)
			return
		}
		fmt.Fprintf(b, "\n Unadopted Deployment %s (%s)\n", d.ReleaseName, d.ID)
	}))
}

func deploymentList(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: errOut}

	// --all lists every workspace's, so it needs no workspace, and a context
	// with none can still list them.
	ws := ""
	if !allDeployments {
		var err error
		if ws, err = coalesceWorkspace(); err != nil {
			return fmt.Errorf("could not find a valid Workspace: %w", err)
		}
	}

	ds, err := deployment.List(ws, allDeployments, houstonClient, clusterID)
	if err != nil {
		return err
	}
	return emitDeploymentList(r, ds)
}

// dagDeploymentQuestion is what an update asks before it moves a deployment
// onto or off dag_deploy, and "" for any other change, including one that
// names no type.
func dagDeploymentQuestion(current, next string) string {
	switch {
	case current != houston.DagOnlyDeploymentType && next == houston.DagOnlyDeploymentType:
		return UpdateDeploymentTypeToDagDeployPromptMsg
	case current == houston.DagOnlyDeploymentType && next != "" && next != houston.DagOnlyDeploymentType:
		return UpdateDeploymentTypeFromDagDeployPromptMsg
	}
	return ""
}

func deploymentUpdate(cmd *cobra.Command, dagDeploymentType, nfsLocation string, out io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: cmd.ErrOrStderr()}
	if err := requireDeployment(cmd); err != nil {
		return err
	}

	argsMap := map[string]string{}
	if deploymentUpdateDescription != "" {
		argsMap["description"] = deploymentUpdateDescription
	}
	if deploymentUpdateName != "" {
		argsMap["label"] = deploymentUpdateName
	}

	var nfsMountDAGDeploymentEnabled, gitSyncDAGDeploymentEnabled, dagOnlyDeployEnabled bool
	deploymentInfo, err := houston.Call(houstonClient.GetDeployment)(deploymentID)
	if err != nil {
		return fmt.Errorf("could not get Deployment info: %w", err)
	}
	appConfig, err = houston.Call(houstonClient.GetAppConfig)(houston.GetAppConfigRequest{
		ClusterID:      deploymentInfo.ClusterID,
		WorkspaceUUID:  deploymentInfo.Workspace.ID,
		DeploymentUUID: deploymentID,
	})
	if err != nil {
		return fmt.Errorf("could not get the platform config: %w", err)
	}
	if appConfig != nil {
		nfsMountDAGDeploymentEnabled = appConfig.Flags.NfsMountDagDeployment
		gitSyncDAGDeploymentEnabled = appConfig.Flags.GitSyncEnabled
		dagOnlyDeployEnabled = appConfig.Flags.DagOnlyDeployment
	}

	// we should validate only in case when this feature has been enabled
	if nfsMountDAGDeploymentEnabled || gitSyncDAGDeploymentEnabled || dagOnlyDeployEnabled {
		err = validateDagDeploymentArgs(dagDeploymentType, nfsLocation, gitRepoURL, true)
		if err != nil {
			return err
		}
	}

	var executorType string
	if executorUpdate != "" {
		executorType, err = validateExecutorArg(executorUpdate)
		if err != nil {
			return err
		}
	}

	// The settings the flags name (the SSH key and known hosts files, the
	// repository) are read once, here, and what is read is what is sent. A
	// move onto or off dag_deploy is confirmed first, once what was asked for
	// is known to be valid, and a run that may not ask is refused before any
	// file is read.
	var dagConfig map[string]interface{}
	build := func() (err error) {
		dagConfig, err = deployment.BuildDagDeploymentConfig(dagDeploymentType, nfsLocation, sshKey, knowHosts, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, gitSyncInterval)
		return err
	}
	if question := dagDeploymentQuestion(deploymentInfo.DagDeployment.Type, dagDeploymentType); question != "" {
		err = confirmAfter(question, build)
	} else {
		err = build()
	}
	if err != nil {
		return err
	}

	d, err := deployment.Update(deploymentID, cloudRole, argsMap, dagConfig, executorType, updateTriggererReplicas, houstonClient, appConfig)
	if err != nil {
		return err
	}
	return emitUpdated(r, d)
}

func deploymentAirflowUpgrade(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: errOut}
	var (
		change *deployment.VersionChange
		err    error
	)
	if cancel {
		change, err = deployment.AirflowUpgradeCancel(deploymentID, houstonClient)
	} else {
		change, err = deployment.AirflowUpgrade(deploymentID, desiredAirflowVersion, houstonClient)
	}
	if err != nil {
		return err
	}
	return emitVersionChange(r, airflowUpgrade, change)
}

func deploymentRuntimeUpgrade(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: errOut}
	var (
		change *deployment.VersionChange
		err    error
	)
	if cancel {
		change, err = deployment.RuntimeUpgradeCancel(deploymentID, houstonClient)
	} else {
		change, err = deployment.RuntimeUpgrade(deploymentID, desiredRuntimeVersion, houstonClient)
	}
	if err != nil {
		return err
	}
	return emitVersionChange(r, runtimeUpgrade, change)
}

func deploymentRuntimeMigrate(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: deploymentOutput, Out: out, Err: errOut}
	var (
		change *deployment.VersionChange
		err    error
	)
	if cancel {
		change, err = deployment.RuntimeMigrateCancel(deploymentID, houstonClient)
	} else {
		change, err = deployment.RuntimeMigrate(deploymentID, houstonClient)
	}
	if err != nil {
		return err
	}
	return emitVersionChange(r, runtimeMigrate, change)
}
