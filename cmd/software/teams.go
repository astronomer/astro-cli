package software

import (
	"io"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
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
	var usersEnabled, rolesEnabled, all bool
	cmd := &cobra.Command{
		Use:     "get [TEAM ID]",
		Aliases: []string{"g"},
		Short:   "Get a team in the Astronomer Platform",
		Long:    "Get a team in the Astronomer Platform",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return teams.Get(args[0], usersEnabled, rolesEnabled, all, houstonClient, out)
		},
	}
	cmd.Flags().BoolVarP(&usersEnabled, "users", "u", false, "Get user details of the team")
	cmd.Flags().BoolVarP(&rolesEnabled, "roles", "r", false, "Get role details of the team")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Use all of the filters")
	return cmd
}

func newTeamListCmd(out io.Writer) *cobra.Command {
	var paginated bool
	var pageSize int
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List all teams in the Astronomer Platform",
		Long:    "List all teams in the Astronomer Platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return listTeam(cmd, out, paginated, pageSize)
		},
	}
	cmd.Flags().BoolVarP(&paginated, "paginated", "p", false, "Paginated team list")
	cmd.Flags().IntVarP(&pageSize, "page-size", "s", 0, "Page size of the team list if paginated is set to true")
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

func listTeam(_ *cobra.Command, out io.Writer, paginated bool, pageSize int) error {
	if config.CFG.Interactive.GetBool() || paginated {
		configPageSize := config.CFG.PageSize.GetInt()
		if pageSize <= 0 && teams.ListTeamLimit > 0 {
			pageSize = configPageSize
		}

		if !(pageSize > 0 && pageSize <= teams.ListTeamLimit) {
			logger.Warnf("Page size cannot be more than %d, reducing the page size to %d", teams.ListTeamLimit, teams.ListTeamLimit)
			pageSize = teams.ListTeamLimit
		}

		return teams.PaginatedList(houstonClient, out, pageSize, 0, "")
	}
	return teams.List(houstonClient, out)
}
