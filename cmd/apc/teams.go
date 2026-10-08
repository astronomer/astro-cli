package apc

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/teams"
	"github.com/astronomer/astro-cli/pkg/logger"
)

func newTeamCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage APC Teams",
		Long:  "A team represents a group of users from an IDP in the APC platform",
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
		Use:     "get <TEAM_ID>",
		Aliases: []string{"g"},
		Short:   "Get a team in the APC platform",
		Long:    "Get a team in the APC platform",
		Example: `  # Show a team
  astro team get <TEAM_ID>

  # Show a team with its users and roles
  astro team get <TEAM_ID> --all`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := cliout.Renderer{Format: accessOutput, Out: out}
			cmd.SilenceUsage = true
			d, err := teams.Get(args[0], usersEnabled || all, houstonClient)
			if err != nil {
				return err
			}
			return renderTeamDetail(r, &d, rolesEnabled || all)
		},
	}
	cmd.Flags().BoolVarP(&usersEnabled, "users", "u", false, "Get user details of the team")
	cmd.Flags().BoolVarP(&rolesEnabled, "roles", "r", false, "Get role details of the team")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Use all of the filters")
	addAccessOutputFlag(cmd)
	return cmd
}

func newTeamListCmd(out io.Writer) *cobra.Command {
	var paginated bool
	var pageSize int
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List all teams in the APC platform",
		Long:    "List all teams in the APC platform",
		Example: `  # List every team
  astro team list

  # List teams a page at a time
  astro team list --paginated --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listTeam(cmd, out, paginated, pageSize)
		},
	}
	cmd.Flags().BoolVarP(&paginated, "paginated", "p", false, "Paginated team list")
	cmd.Flags().IntVarP(&pageSize, "page-size", "s", 0, "Page size of the team list if paginated is set to true")
	addAccessOutputFlag(cmd)
	return cmd
}

func newTeamUpdateCmd(out io.Writer) *cobra.Command {
	var teamRole string
	cmd := &cobra.Command{
		Use:     "update <TEAM_ID>",
		Aliases: []string{"u"},
		Short:   "Update a team in the APC platform",
		Long:    "Update a team in the APC platform",
		Example: `  # Give a team the system editor role
  astro team update <TEAM_ID> --role SYSTEM_EDITOR`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := cliout.Renderer{Format: accessOutput, Out: out}
			cmd.SilenceUsage = true
			change, err := teams.Update(args[0], teamRole, houstonClient)
			if err != nil {
				return err
			}
			return renderTeamUpdated(r, &change)
		},
	}
	cmd.Flags().StringVarP(&teamRole, "role", "r", "", "Role assigned to the team, one of: SYSTEM_VIEWER, SYSTEM_EDITOR, SYSTEM_ADMIN, NONE")
	_ = cmd.MarkFlagRequired("role") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	addAccessOutputFlag(cmd)
	return cmd
}

func listTeam(cmd *cobra.Command, out io.Writer, paginated bool, pageSize int) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	if paginated && r.Format == cliout.FormatJSON {
		return cliout.Usage(errListPaginatedUnderJSON)
	}
	cmd.SilenceUsage = true
	// Under json the interactive setting does not apply: the whole list is
	// the result.
	if r.Format != cliout.FormatJSON && (config.CFG.Interactive.GetBool() || paginated) {
		configPageSize := config.CFG.PageSize.GetInt()
		if pageSize <= 0 && teams.ListTeamLimit > 0 {
			pageSize = configPageSize
		}

		if !(pageSize > 0 && pageSize <= teams.ListTeamLimit) {
			logger.Warnf("Page size cannot be more than %d, reducing the page size to %d", teams.ListTeamLimit, teams.ListTeamLimit)
			pageSize = teams.ListTeamLimit
		}

		return teams.PaginatedList(houstonClient, pageSize, 0, "", func(ts []houston.Team) error {
			return systemTeamsTable(ts).Print(out)
		})
	}
	ts, err := teams.List(houstonClient)
	if err != nil {
		return err
	}
	return renderTeamList(r, ts)
}
