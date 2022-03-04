package cmd

import (
	"io"

	team "github.com/astronomer/astro-cli/team"
	"github.com/spf13/cobra"
)

func newTeamCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage Astronomer Teams",
		Long:  "Teams represents a Team in or a group from an IDP in the Astronomer platform",
	}
	cmd.AddCommand(
		newTeamGetCmd(out),
	)
	return cmd
}

func newTeamGetCmd(out io.Writer) *cobra.Command {
	var (
		teamID       string
		usersEnabled bool
	)
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"g"},
		Short:   "Get a team in the astronomer platform",
		Long:    "Get a team in the astronomer platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return team.Get(teamID, usersEnabled, houstonClient, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "i", "", "Team Id")
	cmd.Flags().BoolVarP(&usersEnabled, "users", "u", false, "add team Users")
	return cmd
}
