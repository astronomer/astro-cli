package cloud

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/organization"
)

var (
	shouldDisplayLoginLink bool
	orgList                 = organization.List
	orgSwitch               = organization.Switch
	orgExportAuditLogs      = organization.ExportAuditLogs
	auditLogsOutputFilePath string
	auditLogsEarliestParam  int
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
		newOrganizationExportAuditLogs(out),
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
		Use:     "switch [organization name/id]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Organization",
		Long:    "Switch to a different Organization",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationSwitch(cmd, out, args)
		},
	}

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for organization switch")
	return cmd
}

func newOrganizationExportAuditLogs(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export-audit-logs --output-file [output_file.json]",
		Aliases: []string{"sw"},
		Short:   "Export your organization audit logs. Requires being an organization owner.",
		Long:    "Export your organization audit logs. Requires being an organization owner.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationExportAuditLogs(cmd, out, args)
		},
	}
	cmd.PersistentFlags().StringVarP(&auditLogsOutputFilePath, "output-file", "o", "", "Path to a file for storing exported audit logs")
	cmd.PersistentFlags().IntVarP(
		&auditLogsEarliestParam, "earliest", "e", 90,
		"Number of days in the past to start exporting logs from. Minimum: 1. Maximum: 90. Defaults to 90 days.")
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

	organizationNameOrID := ""

	if len(args) == 1 {
		organizationNameOrID = args[0]
	}

	return orgSwitch(organizationNameOrID, astroClient, out, shouldDisplayLoginLink)
}

func organizationExportAuditLogs(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// TODO handle arg and create io.Writer if filename provided
	// TODO handle earliest param if provided
	return orgExportAuditLogs(astroClient, out)
}
