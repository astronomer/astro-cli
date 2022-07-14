package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/software/workspace"
	"github.com/spf13/cobra"
)

var (
	workspaceUserWsRole      string
	workspaceUserCreateEmail string
	paginated                bool
)

const paginationPageSize = 100

func newWorkspaceUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Workspace User resources",
		Long:  "Users can be added or removed from Workspaces",
	}
	cmd.AddCommand(
		newWorkspaceUserAddCmd(out),
		newWorkspaceUserUpdateCmd(out),
		newWorkspaceUserRemoveCmd(out),
		newWorkspaceUserListCmd(out),
	)

	cmd.PersistentFlags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the workspace, you can leave it empty if you want to use your current context's workspace ID")
	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a User to a Workspace",
		Long:  "Add a User to a Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserAdd(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceUserWsRole, "role", "r", houston.WorkspaceViewerRole, "Role assigned to user")
	cmd.Flags().StringVarP(&workspaceUserCreateEmail, "email", "e", "", "Email of the user you wish to add to this workspace.")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [user-email]",
		Short: "Update a User's Role for a Workspace",
		Long:  "Update a User's Role for a Workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserUpdate(cmd, out, args)
		},
	}
	cmd.Flags().StringVar(&workspaceUserWsRole, "role", houston.WorkspaceViewerRole, "Role assigned to user")
	return cmd
}

func newWorkspaceUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [user-email]",
		Aliases: []string{"rm"},
		Short:   "Remove a User from a Workspace",
		Long:    "Remove a User from a Workspace",
		Example: "astro workspace user remove test@astronomer.com",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserRemove(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List users inside an Astronomer Workspaces",
		Long:    "List users inside an Astronomer Workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserList(cmd, out)
		},
	}
	cmd.Flags().BoolVarP(&paginated, "paginated", "p", false, "Paginated user list")
	return cmd
}

func workspaceUserAdd(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceUserWsRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.Add(ws, workspaceUserCreateEmail, workspaceUserWsRole, houstonClient, out)
}

func workspaceUserUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceUserWsRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateRole(ws, args[0], workspaceUserWsRole, houstonClient, out)
}

func workspaceUserRemove(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	user, err := houstonClient.GetWorkspaceUserRole(ws, args[0])
	if err != nil {
		return err
	}

	return workspace.Remove(ws, user.ID, houstonClient, out)
}

func workspaceUserList(_ *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}
	if paginated {
		return workspace.PaginatedListRoles(ws, "", paginationPageSize, true, houstonClient, out)
	}
	return workspace.ListRoles(ws, houstonClient, out)
}
