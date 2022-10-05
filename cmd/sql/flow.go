package sql

import (
	sql "github.com/astronomer/astro-cli/sql"
	"github.com/spf13/cobra"
)

var (
	flow = sql.Flow
)

func NewFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Run flow commands",
		Long:  "Forward flow subcommands to the flow python package",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return flow(cmd, args)
		},
	}

	return cmd
}
