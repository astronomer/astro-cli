package apc

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

var (
	deploymentUserID       string
	deploymentUserEmail    string
	deploymentUserRole     string
	deploymentUserFullname string
	// examples
	deploymentUserListExample = `  # List a Deployment's users
  astro deployment user list --deployment=<DEPLOYMENT_ID>

  # Search them by email
  astro deployment user list --deployment=<DEPLOYMENT_ID> --email=<EMAIL>`
	deploymentUserCreateExample = `  # Give a Workspace user viewer access to a Deployment
  astro deployment user add --deployment=<DEPLOYMENT_ID> --email=<EMAIL>

  # Give them another role
  astro deployment user add --deployment=<DEPLOYMENT_ID> --email=<EMAIL> --role=DEPLOYMENT_EDITOR`
	deploymentUserRemoveExample = `  # Remove a user's access to a Deployment
  astro deployment user remove <EMAIL> --deployment=<DEPLOYMENT_ID>`
	deploymentUserUpdateExample = `  # Change a user's role in a Deployment
  astro deployment user update <EMAIL> --deployment=<DEPLOYMENT_ID> --role=DEPLOYMENT_ADMIN`
)

func newDeploymentUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us", "users"},
		Short:   "Manage Deployment user resources",
		Long:    "Users can be added or removed from Deployment",
	}
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment whose users to manage: a link name from the project's manifest, or a Deployment id", true, "")
	_ = cmd.MarkPersistentFlagRequired("deployment") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
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
	cmd.Flags().StringVarP(&deploymentUserID, "user-id", "u", "", "ID of the user to search for")
	cmd.Flags().StringVarP(&deploymentUserEmail, "email", "e", "", "Email of the user to search for")
	cmd.Flags().StringVarP(&deploymentUserFullname, "name", "n", "", "Full name of the user to search for")
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a user to a Deployment",
		Long:    "Add a user to a Deployment",
		Example: deploymentUserCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserAdd(cmd, out)
		},
	}
	cmd.PersistentFlags().StringVarP(&deploymentUserRole, "role", "r", houston.DeploymentViewerRole, "Role assigned to user, one of: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR, DEPLOYMENT_ADMIN")
	cmd.Flags().StringVarP(&deploymentUserEmail, "email", "e", "", "Email of the user to add to the Deployment")

	_ = cmd.MarkFlagRequired("email") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name

	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <EMAIL>",
		Short:   "Remove a user from a Deployment",
		Long:    "Remove a user from a Deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserRemoveExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserRemove(cmd, out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func newDeploymentUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <EMAIL>",
		Short:   "Update a user's role for a deployment",
		Long:    "Update a user's role for a deployment",
		Args:    cobra.ExactArgs(1),
		Example: deploymentUserUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUserUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVarP(&deploymentUserRole, "role", "r", houston.DeploymentViewerRole, "Role assigned to user, one of: DEPLOYMENT_VIEWER, DEPLOYMENT_EDITOR, DEPLOYMENT_ADMIN")
	addAccessOutputFlag(cmd)
	return cmd
}

func deploymentUserList(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	users, err := deployment.UserList(deploymentID, deploymentUserEmail, deploymentUserID, deploymentUserFullname, houstonClient)
	if errors.Is(err, deployment.ErrNoDeploymentUsers) {
		return renderNoDeploymentUsers(r)
	}
	if err != nil {
		return err
	}
	return renderDeploymentUserList(r, users)
}

func deploymentUserAdd(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := validateDeploymentRole(deploymentUserRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	u, err := deployment.Add(deploymentID, deploymentUserEmail, deploymentUserRole, houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentUserChange(r, deploymentID, deploymentUserEmail, &u, true)
}

func deploymentUserRemove(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	u, err := deployment.RemoveUser(deploymentID, args[0], houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentUserRemoval(r, deploymentID, args[0], &u)
}

func deploymentUserUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if err := validateDeploymentRole(deploymentUserRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	u, err := deployment.UpdateUser(deploymentID, args[0], deploymentUserRole, houstonClient)
	if err != nil {
		return err
	}
	return renderDeploymentUserChange(r, deploymentID, args[0], &u, false)
}
