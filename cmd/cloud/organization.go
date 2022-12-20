package cloud

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/config"
)

var (
	orgList                            = organization.List
	orgSwitch                          = organization.Switch
	orgExportAuditLogs                 = organization.ExportAuditLogs
	orgName                            string
	auditLogsOutputFilePath            string
	auditLogsEarliestParam             int
	auditLogsEarliestParamDefaultValue = 90
	shouldDisplayLoginLink             bool
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
	if config.CFG.AuditLogs.GetBool() {
		cmd.AddCommand(newOrganizationAuditLogs(out))
	}
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
		Use:     "audit-logs",
		Aliases: []string{"al"},
		Short:   "Manage your organization audit logs.",
		Long:    "Manage your organization audit logs.",
	}
	cmd.PersistentFlags().StringVarP(&orgName, "organization-name", "n", "", "Name of the organization to manage audit logs for.")
	err := cmd.MarkPersistentFlagRequired("organization-name")
	if err != nil {
		log.Fatalf("Error marking organization-name flag as required in astro organization audit-logs command: %s", err.Error())
	}
	cmd.AddCommand(
		newOrganizationExportAuditLogs(out),
	)
	return cmd
}

func newOrganizationExportAuditLogs(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export",
		Aliases: []string{"e"},
		Short:   "Export your organization audit logs in GZIP. Requires being an organization owner.",
		Long:    "Export your organization audit logs in GZIP. Requires being an organization owner.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationExportAuditLogs(cmd)
		},
	}
	cmd.Flags().StringVarP(&auditLogsOutputFilePath, "output-file", "o", "", "Path to a file for storing exported audit logs")
	cmd.Flags().IntVarP(
		&auditLogsEarliestParam, "earliest", "e", auditLogsEarliestParamDefaultValue,
		"Number of days in the past to start exporting logs from. Minimum: 1. Maximum: 90.")
	return cmd
}

func organizationList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return orgList(out, astroCoreClient)
}

func organizationSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	organizationNameOrID := ""

	if len(args) == 1 {
		organizationNameOrID = args[0]
	}

	return orgSwitch(organizationNameOrID, astroClient, astroCoreClient, out, shouldDisplayLoginLink)
}

func organizationExportAuditLogs(cmd *cobra.Command) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var outputFileName string
	if auditLogsOutputFilePath != "" {
		outputFileName = auditLogsOutputFilePath
	} else {
		outputFileName = fmt.Sprintf("audit-logs-%s.ndjson.gz", time.Now().Format("2006-01-02-150405"))
	}

	var filePerms fs.FileMode = 0o755
	// In CLI mode we should not need to close f
	f, err := os.OpenFile(outputFileName, os.O_RDWR|os.O_CREATE, filePerms)
	if err != nil {
		return err
	}
	out := bufio.NewWriter(f)

	return orgExportAuditLogs(astroClient, out, orgName, auditLogsEarliestParam)
}
