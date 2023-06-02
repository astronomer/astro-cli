package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/software/deployment"
	"github.com/spf13/cobra"
)

var (
	deploymentUserID       string
	deploymentUserEmail    string
	deploymentUserRole     string
	deploymentUserFullname string
	// examples
	deploymentUserListExample = `
# Search for deployment users
  $ astro deployment user list --deployment-id=<deployment-id> --email=EMAIL_ADDRESS --user-id=ID --name=NAME
`
	deploymentUserCreateExample = `
# Add a workspace user to a deployment with a particular role
  $ astro deployment user add --deployment-id=xxxxx --role=DEPLOYMENT_ROLE --email=<user-email-address>
`
	deploymentUserRemoveExample = `
# Remove user access to a deployment
	$ astro deployment user remove --deployment-id=xxxxx <user-email-address>
`
	deploymentUserUpdateExample = `
# Update a workspace user's deployment role
  $ astro deployment user update --deployment-id=xxxxx --role=DEPLOYMENT_ROLE <user-email-address>
`
)

func newDeploymentUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Deployment user resources",
		Long:  "Users can be added or removed from Deployment",
	}
	cmd.AddCommand(
		newDeploymentUserListCmd(out),
		newDeploymentUserAddCmd(out),
		newDeploymentUserRemoveCmd(out),
		newDeploymentUserUpdateCmd(out),
	)
	return cmd
}

func newDeploymentUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "Search for Deployment users",
		Long:    "Search for Deployment users",
		Example: deploymentUserListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserList(cmd, out)
		},
	}
	cmd.Flags().StringVar(&deploymentID, "deployment-id", "", "ID of the Deployment in which you wish to manage users")
	cmd.Flags().StringVarP(&deploymentUserID, "user-id", "u", "", "ID of the user to search for")
	cmd.Flags().StringVarP(&deploymentUserEmail, "email", "e", "", "Email of the user to search for")
	cmd.Flags().StringVarP(&deploymentUserFullname, "name", "n", "", "Full name of the user to search for")
	_ = cmd.MarkFlagRequired("deployment-id")

	return cmd
}

func newDeploymentUserAddCmd(out io.Writer) *cobra.Command { //nolint:dupl
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a user to a Deployment",
		Long:    "Add a user to a Deployment",
		Example: deploymentUserCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserAdd(cmd, out)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "ID of the Deployment in which you wish to add the user")
	cmd.PersistentFlags().StringVar(&deploymentUserRole, "role", houston.DeploymentViewerRole, "Role assigned to user, one of: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR, DEPLOYMENT_ADMIN")
	cmd.Flags().StringVarP(&deploymentUserEmail, "email", "e", "", "Email of the user to add to the Deployment")

	_ = cmd.MarkFlagRequired("deployment-id")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func newDeploymentUserRemoveCmd(out io.Writer) *cobra.Command { //nolint:dupl
	cmd := &cobra.Command{
		Use:     "remove [email]",
		Short:   "Remove a user from a Deployment",
		Long:    "Remove a user from a Deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserRemoveExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserRemove(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "ID of the Deployment where you want to remove the user")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func newDeploymentUserUpdateCmd(out io.Writer) *cobra.Command { //nolint:dupl
	cmd := &cobra.Command{
		Use:     "update [email]",
		Short:   "Update a user's role for a deployment",
		Long:    "Update a user's role for a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "ID of the Deployment where you want to update the user")
	cmd.PersistentFlags().StringVar(&deploymentUserRole, "role", houston.DeploymentViewerRole, "Role assigned to user, one of: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR, DEPLOYMENT_ADMIN")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func deploymentUserList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UserList(deploymentID, deploymentUserEmail, deploymentUserID, deploymentUserFullname, houstonClient, out)
}

func deploymentUserAdd(cmd *cobra.Command, out io.Writer) error {
	if err := validateDeploymentRole(deploymentUserRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.Add(deploymentID, deploymentUserEmail, deploymentUserRole, houstonClient, out)
}

func deploymentUserRemove(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.RemoveUser(deploymentID, args[0], houstonClient, out)
}

func deploymentUserUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	if err := validateDeploymentRole(deploymentUserRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return deployment.UpdateUser(deploymentID, args[0], deploymentUserRole, houstonClient, out)
}
