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
  astro workspace team add <TEAM_ID> --workspace <WORKSPACE_ID> --role WORKSPACE_EDITOR
`
	workspaceTeamRemoveExample = `
  astro workspace team remove <TEAM_ID> --workspace <WORKSPACE_ID>
`
	workspaceTeamUpdateExample = `
  # Change a team's role in a Workspace
  astro workspace team update <TEAM_ID> --workspace <WORKSPACE_ID> --role WORKSPACE_EDITOR
`
	workspaceTeamsListExample = `
  astro workspace team list --workspace <WORKSPACE_ID>`
)

func newWorkspaceTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage Workspace Team resources",
		Long:    "A Team is a group of users imported from your Identity Provider. Add a Team to or remove it from a Deployment to manage access for the whole group.",
	}
	addWorkspaceFlag(cmd.PersistentFlags(), "Workspace whose teams to manage (default: the current context's)", "w")
	cmd.AddCommand(
		newWorkspaceTeamAddCmd(out),
		newWorkspaceTeamUpdateCmd(out),
		newWorkspaceTeamRemoveCmd(out),
		newWorkspaceTeamsListCmd(out),
	)
	return cmd
}

func newWorkspaceTeamAddCmd(out io.Writer) *cobra.Command { //nolint:dupl // the Deployment twin differs in its flags and its role
	cmd := cliout.CheckArgs(cliout.Args(&cobra.Command{
		Use:     "add",
		Short:   "Add a Team to a Workspace",
		Long:    "Add a Team to a Workspace, named by its id as the argument or with --team-id",
		Example: workspaceTeamAddExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamAdd(out, cmd.ErrOrStderr(), args)
		},
	}, cliout.Optional("TEAM_ID")), argOrFlag("team-id", "team"))
	cmd.PersistentFlags().StringVar(&teamID, "team-id", "", "Team ID to be assigned to Workspace")
	// A persistent flag is not among the command's own flags, where
	// MarkFlagRequired looks, so marking it so failed, unseen, and a run
	// without one asked Houston about the team "".
	_ = cmd.MarkPersistentFlagRequired("team-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cmd.PersistentFlags().StringVar(&workspaceTeamRole, "role", houston.WorkspaceViewerRole, "Workspace role assigned to team")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
		Short:   "Update a Team inside a Workspace",
		Long:    "Update a Team inside a Workspace",
		Example: workspaceTeamUpdateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamUpdate(out, cmd.ErrOrStderr(), args)
		},
	}, cliout.Required("TEAM_ID"))
	cmd.PersistentFlags().StringVar(&workspaceTeamRole, "role", houston.WorkspaceViewerRole, "Workspace role assigned to team")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a Team from a Workspace",
		Long:    "Remove a Team from a Workspace",
		Example: workspaceTeamRemoveExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceTeamRm(cmd, out, args)
		},
	}, cliout.Required("TEAM_ID"))
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
			return workspaceTeamsList(out, cmd.ErrOrStderr(), args)
		},
	}
	addAccessOutputFlag(cmd)
	return cmd
}

func workspaceTeamAdd(out, errOut io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: errOut}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("could not find a valid Workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceTeamRole); err != nil {
		return fmt.Errorf("could not find a valid role: %w", err)
	}

	w, err := workspace.AddTeam(ws, teamID, workspaceTeamRole, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamAdded(r, w, teamID, workspaceTeamRole)
}

func workspaceTeamUpdate(out, errOut io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: errOut}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("could not find a valid Workspace: %w", err)
	}

	if err := validateWorkspaceRole(workspaceTeamRole); err != nil {
		return fmt.Errorf("could not find a valid role: %w", err)
	}

	change, err := workspace.UpdateTeamRole(ws, args[0], workspaceTeamRole, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamUpdated(r, &change)
}

func workspaceTeamRm(cmd *cobra.Command, out io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: cmd.ErrOrStderr()}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("could not find a valid Workspace: %w", err)
	}

	removal, err := workspace.RemoveTeam(ws, args[0], houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamRemoved(r, cliout.NotesTo(cmd, r.Format, out), &removal, args[0])
}

func workspaceTeamsList(out, errOut io.Writer, _ []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: errOut}
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("could not find a valid Workspace: %w", err)
	}

	ts, err := workspace.ListTeamRoles(ws, houstonClient)
	if err != nil {
		return err
	}
	return renderWorkspaceTeamList(r, ws, ts)
}
