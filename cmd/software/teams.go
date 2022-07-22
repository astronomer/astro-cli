package software

import (
	"io"

	"github.com/astronomer/astro-cli/software/teams"
	"github.com/spf13/cobra"
)

func newTeamCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage Astronomer Teams",
		Long:  "Teams represents a team or a group from an IDP in the Astronomer Platform",
	}
	cmd.AddCommand(
		newTeamGetCmd(out),
		newTeamListCmd(out),
		newTeamUpdateCmd(out),
	)
	return cmd
}

func newTeamGetCmd(out io.Writer) *cobra.Command {
	var usersEnabled bool
	cmd := &cobra.Command{
		Use:     "get [TEAM ID]",
		Aliases: []string{"g"},
		Short:   "Get a team in the Astronomer Platform",
		Long:    "Get a team in the Astronomer Platform",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return teams.Get(args[0], usersEnabled, houstonClient, out)
		},
	}
	cmd.Flags().BoolVarP(&usersEnabled, "users", "u", false, "Get user details of the team")
	return cmd
}

func newTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List all teams in the Astronomer Platform",
		Long:    "List all teams in the Astronomer Platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return teams.List(houstonClient, out)
		},
	}
	return cmd
}

func newTeamUpdateCmd(out io.Writer) *cobra.Command {
	var teamRole string
	cmd := &cobra.Command{
		Use:     "update [TEAM ID]",
		Aliases: []string{"u"},
		Short:   "Update a team in the Astronomer Platform",
		Long:    "Update a team in the Astronomer Platform",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return teams.Update(args[0], teamRole, houstonClient, out)
		},
	}
	cmd.Flags().StringVarP(&teamRole, "role", "r", "", "Role assigned to the team, one of: SYSTEM_VIEWER, SYSTEM_EDITOR, SYSTEM_ADMIN, NONE")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}
