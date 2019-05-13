package cmd

import (
	"github.com/astronomer/astro-cli/workspace"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceUpdateAttrs = []string{"label"}
	createDesc           string

	workspaceRootCmd = &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer workspaces",
		Long:    "Workspaces contain a group of Airflow Cluster Deployments. The creator of the workspace can invite other users into it",
	}

	workspaceListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List astronomer workspaces",
		Long:    "List astronomer workspaces",
		RunE:    workspaceList,
	}

	workspaceCreateCmd = &cobra.Command{
		Use:     "create WORKSPACE",
		Aliases: []string{"cr"},
		Short:   "Create an astronomer workspaces",
		Long:    "Create an astronomer workspaces",
		Args:    cobra.ExactArgs(1),
		RunE:    workspaceCreate,
	}

	workspaceDeleteCmd = &cobra.Command{
		Use:     "delete WORKSPACE",
		Aliases: []string{"de"},
		Short:   "Delete an astronomer workspace",
		Long:    "Delete an astronomer workspace",
		Args:    cobra.ExactArgs(1),
		RunE:    workspaceDelete,
	}

	workspaceSwitchCmd = &cobra.Command{
		Use:     "switch WORKSPACE",
		Aliases: []string{"sw"},
		Short:   "Switch to a different astronomer workspace",
		Long:    "Switch to a different astronomer workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE:    workspaceSwitch,
	}

	workspaceUpdateCmd = &cobra.Command{
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
		RunE: workspaceUpdate,
	}

	workspaceUserRootCmd = &cobra.Command{
		Use:   "user",
		Short: "Manage workspace user resources",
		Long:  "Users can be added or removed from workspaces",
	}

	workspaceUserAddCmd = &cobra.Command{
		Use:   "add EMAIL",
		Short: "Add a user to a workspace",
		Long:  "Add a user to a workspace",
		Args:  cobra.ExactArgs(1),
		RunE:  workspaceUserAdd,
	}

	workspaceUserUpdateCmd = &cobra.Command{
		Use:   "update user role",
		Short: "Update a user's role for a workspace",
		Long:  "Update a user's role for a workspace",
		Args:  cobra.ExactArgs(1),
		RunE:  workspaceUserUpdate,
	}

	workspaceUserRmCmd = &cobra.Command{
		Use:     "remove EMAIL",
		Aliases: []string{"rm"},
		Short:   "Remove a user from a workspace",
		Long:    "Remove a user from a workspace",
		Args:    cobra.ExactArgs(1),
		RunE:    workspaceUserRm,
	}

	workspaceUserListCmd = &cobra.Command{
		Use:   "list",
		Short: "List users in a workspace",
		Long:  "List users in a workspace",
		RunE:  workspaceUserList,
	}
)

func init() {
	// workspace root
	RootCmd.AddCommand(workspaceRootCmd)

	// workspace list
	workspaceRootCmd.AddCommand(workspaceListCmd)

	// workspace create
	workspaceRootCmd.AddCommand(workspaceCreateCmd)
	workspaceCreateCmd.Flags().StringVarP(&createDesc, "desc", "d", "", "description for your new workspace")

	// workspace delete
	workspaceRootCmd.AddCommand(workspaceDeleteCmd)

	// workspace switch
	workspaceRootCmd.AddCommand(workspaceSwitchCmd)

	// workspace update
	workspaceRootCmd.AddCommand(workspaceUpdateCmd)

	// workspace user root
	workspaceRootCmd.AddCommand(workspaceUserRootCmd)

	// workspace user add
	workspaceUserRootCmd.AddCommand(workspaceUserAddCmd)
	workspaceUserAddCmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
	workspaceUserAddCmd.PersistentFlags().StringVar(&role, "role", "WORKSPACE_VIEWER", "role assigned to user")

	// workspace user update
	workspaceUserRootCmd.AddCommand(workspaceUserUpdateCmd)
	workspaceUserUpdateCmd.PersistentFlags().StringVar(&role, "role", "WORKSPACE_VIEWER", "role assigned to user")

	// workspace user remove
	workspaceUserRootCmd.AddCommand(workspaceUserRmCmd)
	workspaceUserRmCmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")

	// workspace user list
	workspaceUserRootCmd.AddCommand(workspaceUserListCmd)
}

func workspaceCreate(cmd *cobra.Command, args []string) error {
	if len(createDesc) == 0 {
		createDesc = "N/A"
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Create(args[0], createDesc)
}

func workspaceList(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.List()
}

func workspaceDelete(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Delete(args[0])
}

func workspaceUpdate(cmd *cobra.Command, args []string) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Update(args[0], argsMap)
}

func workspaceUserAdd(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	if err := validateRole(role); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.Add(ws, args[0], role)
}

func workspaceUserUpdate(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateRole(role); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateRole(ws, args[0], role)
}

func workspaceUserRm(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Remove(ws, args[0])
}

func workspaceSwitch(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id)
}

func workspaceUserList(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}
	return workspace.ListRoles(ws)
}
