package cmd

import (
	"io"
	"strings"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	sa "github.com/astronomer/astro-cli/serviceaccount"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	allDeployments bool
	executor       string
	deploymentId   string
	userId         string
	email          string
	fullName       string
	systemSA       bool
	category       string
	label          string
	cloudRole      string
	releaseName    string
	CreateExample  = `
# Create new deployment with Celery executor (default: celery without params).
  $ astro deployment create new-deployment-name --executor=celery

# Create new deployment with Local executor.
  $ astro deployment create new-deployment-name-local --executor=local

# Create new deployment with Kubernetes executor.
  $ astro deployment create new-deployment-name-k8s --executor=k8s
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
  # Get deployment service-account
  $ astro deployment service-account get <service-account-id> --deployment-id=<deployment-id>

  # or using deployment-id
  $ astro deployment service-account get --deployment-id=<deployment-id>
`
	deploymentSaDeleteExample = `
  $ astro deployment service-account delete <service-account-id> --deployment-id=<deployment-id>
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
		newDeploymentSaRootCmd(client, out),
		newDeploymentUserRootCmd(client, out),
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
	cmd.Flags().StringVarP(&executor, "executor", "e", "", "Add executor parameter: local or celery")
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUpdate(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&cloudRole, "cloud-role", "c", "", "Set cloud role to annotate service accounts in deployment")
	return cmd
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
	cmd.PersistentFlags().StringVar(&deploymentId, "deployment-id", "", "deployment assigned to user")
	cmd.Flags().StringVarP(&userId, "user-id", "u", "", "[ID]")
	cmd.Flags().StringVarP(&email, "email", "e", "", "[EMAIL]")
	cmd.Flags().StringVarP(&fullName, "name", "n", "", "[NAME]")

	return cmd
}

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
	cmd.PersistentFlags().StringVar(&deploymentId, "deployment-id", "", "deployment assigned to user")
	cmd.PersistentFlags().StringVar(&deploymentRole, "role", houston.DeploymentViewer, "role assigned to user")
	return cmd
}

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
	cmd.PersistentFlags().StringVar(&deploymentId, "deployment-id", "", "deployment to remove user access")
	return cmd
}

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
	cmd.PersistentFlags().StringVar(&deploymentId, "deployment-id", "", "deployment assigned to user")
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
	cmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	cmd.Flags().StringVarP(&userId, "user-id", "u", "", "[ID]")
	cmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	cmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	cmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")
	cmd.Flags().StringVarP(&role, "role", "r", "viewer", "ROLE")
	return cmd
}

func newDeploymentSaGetCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a service-account by entity type and entity id",
		Long:    "Get a service-account by entity type and entity id",
		Example: deploymentSaGetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentSaGet(cmd, client, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	cmd.MarkFlagRequired("deployment-id")
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
	cmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func deploymentCreate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var executorType string
	switch executor {
	case "local":
		executorType = "LocalExecutor"
	case "celery":
		executorType = "CeleryExecutor"
	case "kubernetes", "k8s":
		executorType = "KubernetesExecutor"
	default:
		return errors.New("please specify correct executor, one of: local, celery, kubernetes, k8s")
	}
	return deployment.Create(args[0], ws, releaseName, cloudRole, executorType, client, out)
}

func deploymentDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.Delete(args[0], client, out)
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

func deploymentUpdate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployment.Update(args[0], cloudRole, argsMap, client, out)
}

func deploymentUserList(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UserList(args[0], email, userId, fullName, client, out)
}

func deploymentUserAdd(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateDeploymentRole(deploymentRole); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.Add(deploymentId, args[0], deploymentRole, client, out)
}

func deploymentUserDelete(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.DeleteUser(deploymentId, args[0], client, out)
}

func deploymentUserUpdate(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	_, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateDeploymentRole(deploymentRole); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UpdateUser(deploymentId, args[0], deploymentRole, client, out)
}

func deploymentSaCreate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	if len(label) == 0 {
		return errors.New("must provide a service-account label with the --label (-l) flag")
	}

	if err := validateRole(role); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}
	fullRole := strings.Join([]string{"DEPLOYMENT", strings.ToUpper(role)}, "_")
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.CreateUsingDeploymentUUID(deploymentId, label, category, fullRole, client, out)
}

func deploymentSaGet(cmd *cobra.Command, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Get("DEPLOYMENT", deploymentId, client, out)
}

func deploymentSaDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.DeleteUsingDeploymentUUID(args[0], deploymentId, client, out)
}
