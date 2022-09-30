package cloud

import (
	"bufio"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/organization"
)

var (
	shouldDisplayLoginLink bool
	orgList                            = organization.List
	orgSwitch                          = organization.Switch
	orgExportAuditLogs                 = organization.ExportAuditLogs
	orgName                            string
	auditLogsOutputFilePath            string
	auditLogsEarliestParam             int
	auditLogsEarliestParamDefaultValue = 90
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
		newOrganizationAuditLogs(out),
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

func newOrganizationAuditLogs(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "audit-logs --organization-name [organization_name]",
		Aliases: []string{"al"},
		Short:   "Manage your organization audit logs.",
		Long:    "Manage your organization audit logs.",
	}
	cmd.AddCommand(
		newOrganizationExportAuditLogs(out),
	)
	cmd.PersistentFlags().StringVarP(&orgName, "organization-name", "n", "", "Name of the organization to manage audit logs for.")
	err := cmd.MarkPersistentFlagRequired("organization-name")
	if err != nil {
		log.Fatalf("Error marking organization-name flag as required in astro organization audit-logs command: %s", err.Error())
	}
	return cmd
}

func newOrganizationExportAuditLogs(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export --output-file [output_file.json]",
		Aliases: []string{"e"},
		Short:   "Export your organization audit logs. Requires being an organization owner.",
		Long:    "Export your organization audit logs. Requires being an organization owner.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationExportAuditLogs(cmd, out)
		},
	}
	cmd.LocalFlags().StringVarP(&auditLogsOutputFilePath, "output-file", "o", "", "Path to a file for storing exported audit logs")
	cmd.LocalFlags().IntVarP(
		&auditLogsEarliestParam, "earliest", "e", auditLogsEarliestParamDefaultValue,
		"Number of days in the past to start exporting logs from. Minimum: 1. Maximum: 90.")
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

func organizationExportAuditLogs(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if auditLogsOutputFilePath != "" {
		var filePerms fs.FileMode = 0o755
		// In CLI mode we should not need to close f
		f, err := os.OpenFile(auditLogsOutputFilePath, os.O_RDWR|os.O_CREATE, filePerms)
		if err != nil {
			return err
		}
		out = bufio.NewWriter(f)
	}

	return orgExportAuditLogs(astroClient, out, orgName, auditLogsEarliestParam)
}
