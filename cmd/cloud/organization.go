package cloud

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/organization"
)

var (
	orgList   = organization.List
	orgSwitch = organization.Switch
)

func newOrganizationCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "organization",
		Aliases: []string{"org"},
		Short:   "Manage Astronomer Organizations",
		Long:    "Manage your Astro Organizations. These commands are for users in more than one Organization",
	}
	cmd.AddCommand(
		newOrganizationListCmd(out),
		newOrganizationSwitchCmd(out),
	)
	return cmd
}

func newOrganizationListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Organizations you have access too",
		Long:    "List all Organizations you have access too",
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationList(cmd, out)
		},
	}
	return cmd
}

func newOrganizationSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [organization_id]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Organization",
		Long:    "Switch to a different Organization",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationSwitch(cmd, out, args)
		},
	}
	return cmd
}

func organizationList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return orgList(out)
}

func organizationSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	organizationName := ""

	if len(args) == 1 {
		organizationName = args[0]
	}

	return orgSwitch(organizationName, astroClient, out)
}
