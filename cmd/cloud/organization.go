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
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/input"
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
	role                               string
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
		neOrganizationwUserRootCmd(out),
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

func neOrganizationwUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us"},
		Short:   "Manage users in your Astro Organization",
		Long:    "Manage users in your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newOrganizationUserInviteCmd(out),
		newOrganizationUserListCmd(out),
		newOrganizationUserUpdateCmd(out),
	)
	return cmd
}

func newOrganizationUserInviteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite [email]",
		Aliases: []string{"inv"},
		Short:   "Invite a user to your Astro Organization",
		Long: "Invite a user to your Astro Organization\n$astro user invite [email] --role [ORGANIZATION_MEMBER, " +
			"ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userInvite(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The role for the "+
		"user. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	return cmd
}

func newOrganizationUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the users in your Astro Organization",
		Long:    "List all the users in your Astro Organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listUsers(cmd, out)
		},
	}
	return cmd
}

func newOrganizationUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [email]",
		Aliases: []string{"up"},
		Short:   "Update a the role of a user your in Astro Organization",
		Long: "Update the role of a user in your Astro Organization\n$astro user update [email] --role [ORGANIZATION_MEMBER, " +
			"ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The new role for the "+
		"user. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
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

func userInvite(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		email = args[0]
	} else {
		// no email was provided so ask the user for it
		email = input.Text("enter email address to invite a user: ")
	}

	cmd.SilenceUsage = true
	return user.CreateInvite(email, role, out, astroCoreClient)
}

func listUsers(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return user.ListOrgUsers(out, astroCoreClient)
}

func userUpdate(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		email = args[0]
	}

	cmd.SilenceUsage = true
	return user.UpdateUserRole(email, role, out, astroCoreClient)
}
