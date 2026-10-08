package apc

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
)

var (
	workspaceTeamRole string

	workspaceTeamAddExample = `
  # Add a team to a Workspace as an editor
  astro workspace team add --workspace-id <WORKSPACE_ID> --team-id <TEAM_ID> --role WORKSPACE_EDITOR
`
	workspaceTeamRemoveExample = `
  astro workspace team remove <TEAM_ID> --workspace-id <WORKSPACE_ID>
`
	workspaceTeamUpdateExample = `
  # Change a team's role in a Workspace
  astro workspace team update <TEAM_ID> --workspace-id <WORKSPACE_ID> --role WORKSPACE_EDITOR
`
	workspaceTeamsListExample = `
  astro workspace team list --workspace-id <WORKSPACE_ID>`
)

func newWorkspaceTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage Workspace Team resources",
		Long:    "A Team is a group of users imported from your Identity Provider, teams can be added to and removed from a deployment to manage group user access",
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "Workspace to associate team to")
	cmd.AddCommand(
		newWorkspaceTeamAddCmd(out),
		newWorkspaceTeamUpdateCmd(out),
		newWorkspaceTeamRemoveCmd(out),
		newWorkspaceTeamsListCmd(out),
	)
	return cmd
}

func newWorkspaceTeamAddCmd(out io.Writer) *cobra.Command { //nolint:dupl // the Deployment twin differs in its flags and its role
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a Team to a Workspace",
		Long:    "Add a Team to a Workspace",
		Example: workspaceTeamAddExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamAdd(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&teamID, "team-id", "", "Team ID to be assigned to workspace")
	_ = cmd.MarkFlagRequired("team-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.PersistentFlags().StringVar(&workspaceTeamRole, "role", houston.WorkspaceViewerRole, "Workspace role assigned to team")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <TEAM_ID>",
		Short:   "Update a Team inside a workspace",
		Long:    "Update a Team inside a workspace",
		Example: workspaceTeamUpdateExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamUpdate(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceTeamRole, "role", houston.WorkspaceViewerRole, "Workspace role assigned to team")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <TEAM_ID>",
		Aliases: []string{"rm"},
		Short:   "Remove a Team from a Workspace",
		Long:    "Remove a Team from a Workspace",
		Example: workspaceTeamRemoveExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamRm(cmd, out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceTeamsListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Teams inside an APC Workspace",
		Long:    "List Teams inside an APC Workspace",
		Example: workspaceTeamsListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamsList(cmd, out, args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func workspaceTeamAdd(cmd *cobra.Command, out io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceTeamRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	w, err := workspace.AddTeam(ws, teamID, workspaceTeamRole, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamAdded(r, w, teamID, workspaceTeamRole)
}

func workspaceTeamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceTeamRole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	change, err := workspace.UpdateTeamRole(ws, args[0], workspaceTeamRole, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamUpdated(r, &change)
}

func workspaceTeamRm(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	removal, err := workspace.RemoveTeam(ws, args[0], houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamRemoved(r, cliout.NotesTo(cmd, r.Format, out), &removal, args[0])
}

func workspaceTeamsList(cmd *cobra.Command, out io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	ts, err := workspace.ListTeamRoles(ws, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamList(r, ws, ts)
}
