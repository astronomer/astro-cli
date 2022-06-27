package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/software/workspace"

	"github.com/spf13/cobra"
)

var (
	workspaceTeamRole string

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
	_ = cmd.MarkFlagRequired("team-id")
	cmd.PersistentFlags().StringVar(&workspaceTeamRole, "role", houston.WorkspaceViewerRole, "workspace role assigned to team")
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
	cmd.PersistentFlags().StringVar(&workspaceTeamRole, "role", houston.WorkspaceViewerRole, "workspace role assigned to team")
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

func workspaceTeamAdd(cmd *cobra.Command, out io.Writer, _ []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceTeamRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.AddTeam(ws, teamID, workspaceTeamRole, houstonClient, out)
}

func workspaceTeamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceTeamRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateTeamRole(ws, args[0], workspaceTeamRole, houstonClient, out)
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

func workspaceTeamsList(cmd *cobra.Command, out io.Writer, _ []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.ListTeamRoles(ws, houstonClient, out)
}
