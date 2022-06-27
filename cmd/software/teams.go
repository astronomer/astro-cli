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
	)
	return cmd
}

func newTeamGetCmd(out io.Writer) *cobra.Command {
	var usersEnabled bool
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"g"},
		Short:   "Get a team in the Astronomer Platform",
		Long:    "Get a team in the Astronomer Platform",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return teams.Get(args[0], usersEnabled, houstonClient, out)
		},
	}
	cmd.Flags().BoolVarP(&usersEnabled, "users", "u", false, "add team Users")
	return cmd
}
