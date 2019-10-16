package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/workspace"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceUpdateAttrs = []string{"label"}
	createDesc           string
)

func newWorkspaceCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer workspaces",
		Long:    "Workspaces contain a group of Airflow Cluster Deployments. The creator of the workspace can invite other users into it",
		RunE: func(_ *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(
		newWorkspaceListCmd(client, out),
		newWorkspaceCreateCmd(client, out),
		newWorkspaceDeleteCmd(client, out),
		newWorkspaceSwitchCmd(client, out),
		newWorkspaceUpdateCmd(client, out),
		newWorkspaceUserRootCmd(client, out),
	)
	return cmd
}

func newWorkspaceListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List astronomer workspaces",
		Long:    "List astronomer workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, args, client, out)
		},
	}
	return cmd
}

func newWorkspaceCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create WORKSPACE",
		Aliases: []string{"cr"},
		Short:   "Create an astronomer workspaces",
		Long:    "Create an astronomer workspaces",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&createDesc, "desc", "d", "", "description for your new workspace")
	return cmd
}

func newWorkspaceDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete WORKSPACE",
		Aliases: []string{"de"},
		Short:   "Delete an astronomer workspace",
		Long:    "Delete an astronomer workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, args, client, out)
		},
	}
	return cmd
}

func newWorkspaceSwitchCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch WORKSPACE",
		Aliases: []string{"sw"},
		Short:   "Switch to a different astronomer workspace",
		Long:    "Switch to a different astronomer workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, client, out, args)
		},
	}
	return cmd
}

func newWorkspaceUpdateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Astronomer workspace",
		Long:    "Update a workspace name, as well as users and roles assigned to a workspace",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) <= 1 {
				return errors.New("must specify a workspace ID and at least one attribute to update.")
			}
			return updateArgValidator(args[1:], workspaceUpdateAttrs)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, client, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage workspace user resources",
		Long:  "Users can be added or removed from workspaces",
	}
	cmd.AddCommand(
		newWorkspaceUserAddCmd(client, out),
		newWorkspaceUserUpdateCmd(client, out),
		newWorkspaceUserRmCmd(client, out),
		newWorkspaceUserListCmd(client, out),
	)
	return cmd
}

func newWorkspaceUserAddCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add EMAIL",
		Short: "Add a user to a workspace",
		Long:  "Add a user to a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserAdd(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", "WORKSPACE_VIEWER", "role assigned to user")
	return cmd
}

func newWorkspaceUserUpdateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update user role",
		Short: "Update a user's role for a workspace",
		Long:  "Update a user's role for a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserUpdate(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", "WORKSPACE_VIEWER", "role assigned to user")
	return cmd
}

func newWorkspaceUserRmCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove EMAIL",
		Aliases: []string{"rm"},
		Short:   "Remove a user from a workspace",
		Long:    "Remove a user from a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserRm(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
	return cmd
}

func newWorkspaceUserListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove EMAIL",
		Aliases: []string{"rm"},
		Short:   "Remove a user from a workspace",
		Long:    "Remove a user from a workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserList(cmd, client, out, args)
		},
	}
	return cmd
}

func workspaceCreate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	if len(createDesc) == 0 {
		createDesc = "N/A"
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Create(args[0], createDesc, client, out)
}

func workspaceList(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(client, out)
}

func workspaceDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Delete(args[0], client, out)
}

func workspaceUpdate(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Update(args[0], client, out, argsMap)
}

func workspaceUserAdd(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.Add(ws, args[0], role, client, out)
}

func workspaceUserUpdate(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateRole(ws, args[0], role, client, out)
}

func workspaceUserRm(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Remove(ws, args[0], client, out)
}

func workspaceSwitch(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id, client, out)
}

func workspaceUserList(_ *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}
	return workspace.ListRoles(ws, client, out)
}
