package apc

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	workspaceUserWsRole      string
	workspaceUserCreateEmail string
	paginated                bool
	pageSize                 int
)

const defaultWorkspaceUserPageSize = 100

func newWorkspaceUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us", "users"},
		Short:   "Manage Workspace User resources",
		Long:    "Users can be added or removed from Workspaces",
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
		Example: `  # Add a user to the current Workspace as a viewer
  astro workspace user add --email user@company.com

  # Add a user to another Workspace as an editor
  astro workspace user add --email user@company.com --role WORKSPACE_EDITOR \
    --workspace-id <WORKSPACE_ID>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserAdd(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceUserWsRole, "role", "r", houston.WorkspaceViewerRole, "Role assigned to user")
	cmd.Flags().StringVarP(&workspaceUserCreateEmail, "email", "e", "", "Email of the user you wish to add to this workspace")
	_ = cmd.MarkFlagRequired("email") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <EMAIL>",
		Short: "Update a User's Role for a Workspace",
		Long:  "Update a User's Role for a Workspace",
		Example: `  # Make a user an admin of the current Workspace
  astro workspace user update user@company.com --role WORKSPACE_ADMIN`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserUpdate(cmd, out, args)
		},
	}
	cmd.Flags().StringVar(&workspaceUserWsRole, "role", houston.WorkspaceViewerRole, "Role assigned to user")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <EMAIL>",
		Aliases: []string{"rm"},
		Short:   "Remove a User from a Workspace",
		Long:    "Remove a User from a Workspace",
		Example: `  # Remove a user from the current Workspace
  astro workspace user remove user@company.com`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserRemove(cmd, out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List users inside an APC Workspace",
		Long:    "List users inside an APC Workspace",
		Example: `  # List the users in the current Workspace
  astro workspace user list

  # List the users in another Workspace
  astro workspace user list --workspace-id <WORKSPACE_ID>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserList(cmd, out)
		},
	}
	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		cmd.Flags().BoolVarP(&paginated, "paginated", "p", false, "Paginated workspace user list")
		cmd.Flags().IntVarP(&pageSize, "page-size", "s", 0, "Page size of the workspace user list if paginated is set to true")
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func workspaceUserAdd(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceUserWsRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	w, added, err := workspace.Add(ws, workspaceUserCreateEmail, workspaceUserWsRole, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceUserAdded(r, w, workspaceUserCreateEmail, &added)
}

func workspaceUserUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceUserWsRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	change, err := workspace.UpdateRole(ws, args[0], workspaceUserWsRole, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceUserUpdated(r, args[0], &change)
}

func workspaceUserRemove(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// A user with no Workspace role there is refused before anything is
	// sent: workspaceUser answers for any active user with the email,
	// whatever the Workspace.
	user, err := workspace.UserRoleIn(ws, args[0], houstonClient)
	if err != nil {
		return err
	}

	w, err := workspace.Remove(ws, user.ID, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceUserRemoved(r, w, &user)
}

// errListPaginatedUnderJSON refuses --paginated under --output json: it pages
// through a list by asking which page to show next, and json asks nothing.
var errListPaginatedUnderJSON = errors.New("--paginated pages through the list by asking which page to show next, so it cannot be used with --output json; leave it out to publish the whole list")

func workspaceUserList(_ *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if paginated && r.Format == cliout.FormatJSON {
		return cliout.Usage(errListPaginatedUnderJSON)
	}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}
	configPageSize := config.CFG.PageSize.GetInt()

	// not calling paginated workspace roles if houston version is before 0.30.0, since that doesn't support pagination.
	// Under json the interactive setting does not apply: the whole list is the result.
	if r.Format != cliout.FormatJSON && (config.CFG.Interactive.GetBool() || paginated) && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		if pageSize <= 0 && configPageSize > 0 {
			pageSize = configPageSize
		}

		if !(pageSize > 0 && pageSize <= defaultWorkspaceUserPageSize) {
			logger.Warnf("Page size cannot be more than %d, reducing the page size to %d", defaultWorkspaceUserPageSize, defaultWorkspaceUserPageSize)
			pageSize = defaultWorkspaceUserPageSize
		}

		return workspace.PaginatedListRoles(ws, "", pageSize, 0, houstonClient, func(users []workspace.UserRole) error {
			return workspaceUserListTable(users).Print(out)
		})
	}
	users, err := workspace.ListRoles(ws, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceUserList(r, users)
}
