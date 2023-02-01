package cloud

import (
	"io"
	"strings"

	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/pkg/input"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceID string
	addWorkspaceRole string
	workspaceRole string
)

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer Workspaces",
		Long:    "Create and manage Workspaces on Astro. Workspaces can contain multiple Deployments and can be shared across users.",
	}
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceSwitchCmd(out),
		newWorkspaceUserRootCmd(out),
	)
	return cmd
}

func newWorkspaceListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astronomer Workspaces in your organization",
		Long:    "List all Astronomer Workspaces in your organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [workspace_id]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Astronomer Workspace",
		Long:    "Switch to a different Astronomer Workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us"},
		Short:   "Manage users in your Astro Workspace",
		Long:    "Manage users in your Astro Workspace.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newWorkspaceUserListCmd(out),
		newWorkspaceUserUpdateCmd(out),
		newWorkspaceUserRemoveCmd(out),
		newWorkspaceUserAddCmd(out),
	)
	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [email]",
		Short: "Add a user to an Astro Workspace with a specific role",
		Long: "Add a user to an Astro Workspace with a specific role\n$astro workspace user add [email] --role [WORKSPACE_MEMBER, " +
			"WORKSPACE_OPERATOR, WORKSPACE_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&addWorkspaceRole, "role", "r", "WORKSPACE_MEMBER", "The role for the "+
		"new user. Possible values are WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
	return cmd
}

func newWorkspaceUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the users in an Astro Workspace",
		Long:    "List all the users in an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceUser(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [email]",
		Aliases: []string{"up"},
		Short:   "Update a the role of a user in an Astro Workspace",
		Long: "Update the role of a user in an Astro Workspace\n$astro workspace user update [email] --role [WORKSPACE_MEMBER, " +
			"WORKSPACE_OPERATOR, WORKSPACE_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceRole, "role", "r", "", "The new role for the "+
		"user. Possible values are WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
	return cmd
}

func newWorkspaceUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a user from an Astro Workspace",
		Long:    "Remove a user from an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceUser(cmd, args, out)
		},
	}
	return cmd
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(astroClient, out)
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id, astroClient, out)
}

func addWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return user.AddWorkspaceUser(email, addWorkspaceRole, "", out, astroCoreClient)
}

func listWorkspaceUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return user.ListWorkspaceUsers(out, astroCoreClient, "")
}

func updateWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if workspaceRole == "" {
		// no role was provided so ask the user for it
		workspaceRole = input.Text("Enter a user workspace role(WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER) to update user: ")
	}

	cmd.SilenceUsage = true
	return user.UpdateWorkspaceUserRole(email, workspaceRole, "", out, astroCoreClient)
}

func removeWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return user.RemoveWorkspaceUser(email, "", out, astroCoreClient)
}

func coalesceWorkspace() (string, error) {
	wsFlag := workspaceID
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, "failed to get current workspace")
	}

	if wsFlag != "" {
		return wsFlag, nil
	}

	if wsCfg != "" {
		return wsCfg, nil
	}

	return "", errors.New("no valid workspace source found")
}
