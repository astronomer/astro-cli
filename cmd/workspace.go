package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"

	sa "github.com/astronomer/astro-cli/serviceaccount"
	"github.com/astronomer/astro-cli/workspace"

	"github.com/spf13/cobra"
)

var errUpdateWorkspaceInvalidArgs = errors.New("must specify a workspace ID and at least one attribute to update")

var (
	workspaceUpdateAttrs   = []string{"label"}
	createDesc             string
	workspaceDeleteExample = `
	$ astro workspace delete <workspace-id>
	`
	workspaceSaCreateExample = `
  # Create service-account
  $ astro workspace service-account create --workspace-id=<workspace-id> --label=my_label --role=ROLE
	`
	workspaceSaGetExample = `
	$ astro workspace service-account get --workspace-id=<workspace-id>
	`
	workspaceTeamAddExample = `
	$ astro workspace team add --workspace-id=<workspace-id> --team-id=<team-id> --role=ROLE
	`
	workspaceTeamRemoveExample = `
	$ astro workspace team remove cl0ck2snm0064jexu3ljfn9um --workspace-id ckwwb09aj0048u8xuts287erj
	`
	workspaceTeamUpdateExample = `
	$ astro workspace team update cl0ck2snm0064jexu3ljfn9um --workspace-id ckwwb09aj0048u8xuts287erj --role WORKSPACE_EDITOR
	`
	workspaceTeamsListExample = `
	$ astro workspace team list --workspace-id ckwwb09aj0048u8xuts287erj`
)

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer Workspaces",
		Long:    "Workspaces contain a group of Airflow Cluster Deployments. The creator of the workspace can invite other users into it",
	}
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceCreateCmd(out),
		newWorkspaceDeleteCmd(out),
		newWorkspaceSwitchCmd(out),
		newWorkspaceUpdateCmd(out),
		newWorkspaceUserRootCmd(out),
		newWorkspaceTeamRootCmd(out),
		newWorkspaceSaRootCmd(out),
	)
	return cmd
}

func newWorkspaceListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Astronomer Workspaces",
		Long:    "List Astronomer Workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, args, out)
		},
	}
	return cmd
}

func newWorkspaceCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create WORKSPACE",
		Aliases: []string{"cr"},
		Short:   "Create an Astronomer Workspace",
		Long:    "Create an Astronomer Workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&createDesc, "desc", "d", "", "description for your new workspace")
	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete WORKSPACE",
		Aliases: []string{"de"},
		Short:   "Delete an Astronomer Workspace",
		Long:    "Delete an Astronomer Workspace",
		Example: workspaceDeleteExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, args, out)
		},
	}
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch WORKSPACE",
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

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Astronomer Workspace",
		Long:    "Update a Workspace name, as well as users and roles assigned to a Workspace",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) <= 1 {
				return errUpdateWorkspaceInvalidArgs
			}
			return updateArgValidator(args[1:], workspaceUpdateAttrs)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, out, args)
		},
	}
	return cmd
}

// Workspace user commands

func newWorkspaceUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Workspace User resources",
		Long:  "Users can be added or removed from Workspaces",
	}
	cmd.AddCommand(
		newWorkspaceUserAddCmd(out),
		newWorkspaceUserUpdateCmd(out),
		newWorkspaceUserRmCmd(out),
		newWorkspaceUserListCmd(out),
	)
	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add EMAIL",
		Short: "Add a User to a Workspace",
		Long:  "Add a User to a Workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserAdd(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", houston.WorkspaceViewerRole, "role assigned to user")
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update user role",
		Short: "Update a User's Role for a Workspace",
		Long:  "Update a User's Role for a Workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", houston.WorkspaceViewerRole, "role assigned to user")
	return cmd
}

func newWorkspaceUserRmCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove EMAIL",
		Aliases: []string{"rm"},
		Short:   "Remove a User from a Workspace",
		Long:    "Remove a User from a Workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserRm(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Astronomer Workspaces",
		Long:    "List Astronomer Workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserList(cmd, out, args)
		},
	}
	return cmd
}

// Workspace teams commands

func newWorkspaceTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage Workspace Team resources",
		Long:  "Teams can be added or removed from Workspaces",
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace to associate team to")
	cmd.AddCommand(
		newWorkspaceTeamAddCmd(out),
		newWorkspaceTeamUpdateCmd(out),
		newWorkspaceTeamRemoveCmd(out),
		newWorkspaceTeamsListCmd(out),
	)
	return cmd
}

func newWorkspaceTeamAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a Team to a Workspace",
		Long:    "Add a Team to a Workspace",
		Example: workspaceTeamAddExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamAdd(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&teamID, "team-id", "", "team id to be assigned to workspace")
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", houston.WorkspaceViewerRole, "workspace role assigned to team")
	return cmd
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update a Team inside a workspace",
		Long:    "Update a Team inside a workspace",
		Example: workspaceTeamUpdateExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", houston.WorkspaceViewerRole, "workspace role assigned to team")
	return cmd
}

func newWorkspaceTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove TEAM",
		Aliases: []string{"rm"},
		Short:   "Remove a Team from a Workspace",
		Long:    "Remove a Team from a Workspace",
		Example: workspaceTeamRemoveExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamRm(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceTeamsListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Teams inside an Astronomer Workspace",
		Long:    "List Teams inside an Astronomer Workspace",
		Example: workspaceTeamsListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamsList(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceSaRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage Astronomer Service Accounts",
		Long:    "Service Accounts represent a revokable token with access to an Astronomer Cluster",
	}
	cmd.AddCommand(
		newWorkspaceSaCreateCmd(out),
		newWorkspaceSaGetCmd(out),
		newWorkspaceSaDeleteCmd(out),
	)
	return cmd
}

// nolint:dupl
func newWorkspaceSaCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a Service Account in an Astronomer Cluster",
		Long:    "Create a Service Account in an Astronomer Cluster",
		Example: workspaceSaCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaCreate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "[ID]")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "[ID]")
	cmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	cmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	cmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")
	cmd.Flags().StringVarP(&role, "role", "r", "viewer", "ROLE")
	return cmd
}

func newWorkspaceSaGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a Service Account by workspace",
		Long:    "Get a Service Account by workspace",
		Example: workspaceSaGetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaGet(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "[ID]")
	_ = cmd.MarkFlagRequired("workspace-id")
	return cmd
}

func newWorkspaceSaDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [SA-ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Service Account in the astronomer platform",
		Long:    "Delete a Service Account in the astronomer platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaDelete(cmd, args, out)
		},
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "[ID]")
	_ = cmd.MarkFlagRequired("workspace-id")
	return cmd
}

func workspaceCreate(cmd *cobra.Command, args []string, out io.Writer) error {
	if createDesc == "" {
		createDesc = "N/A"
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Create(args[0], createDesc, houstonClient, out)
}

func workspaceList(cmd *cobra.Command, _ []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(houstonClient, out)
}

func workspaceDelete(cmd *cobra.Command, args []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Delete(args[0], houstonClient, out)
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Update(args[0], houstonClient, out, argsMap)
}

func workspaceUserAdd(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.Add(ws, args[0], workspaceRole, houstonClient, out)
}

func workspaceUserUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateRole(ws, args[0], workspaceRole, houstonClient, out)
}

func workspaceUserRm(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Remove(ws, args[0], houstonClient, out)
}

func workspaceUserList(_ *cobra.Command, out io.Writer, _ []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}
	return workspace.ListRoles(ws, houstonClient, out)
}

func workspaceTeamAdd(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.AddTeam(ws, teamID, workspaceRole, houstonClient, out)
}

func workspaceTeamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateTeamRole(ws, args[0], workspaceRole, houstonClient, out)
}

func workspaceTeamRm(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.RemoveTeam(ws, args[0], houstonClient, out)
}

func workspaceTeamsList(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.ListTeamRoles(ws, houstonClient, out)
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id, houstonClient, out)
}

func workspaceSaCreate(cmd *cobra.Command, _ []string, out io.Writer) error {
	if label == "" {
		return errServiceAccountNotPresent
	}

	if err := validateRole(role); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}
	fullRole := strings.Join([]string{"WORKSPACE", strings.ToUpper(role)}, "_")
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.CreateUsingWorkspaceUUID(workspaceID, label, category, fullRole, houstonClient, out)
}

func workspaceSaGet(cmd *cobra.Command, _ []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.GetWorkspaceServiceAccounts(workspaceID, houstonClient, out)
}

func workspaceSaDelete(cmd *cobra.Command, args []string, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.DeleteUsingWorkspaceUUID(args[0], workspaceID, houstonClient, out)
}
