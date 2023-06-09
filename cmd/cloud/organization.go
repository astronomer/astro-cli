package cloud

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/team"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	orgList                            = organization.List
	orgSwitch                          = organization.Switch
	orgExportAuditLogs                 = organization.ExportAuditLogs
	orgName                            string
	auditLogsOutputFilePath            string
	auditLogsEarliestParam             int
	auditLogsEarliestParamDefaultValue = 1
	shouldDisplayLoginLink             bool
	role                               string
	updateRole                         string
	teamDescription                    string
	teamName                           string
	teamID                             string
	userID                             string
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
		newOrganizationUserRootCmd(out),
		newOrganizationTeamRootCmd(out),
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
		Use:     "audit-logs",
		Aliases: []string{"al"},
		Short:   "Manage your organization audit logs.",
		Long:    "Manage your organization audit logs.",
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
	cmd.PersistentFlags().StringVarP(&orgName, "organization-name", "n", "", "Name of the organization to manage audit logs for.")
	err := cmd.MarkPersistentFlagRequired("organization-name")
	if err != nil {
		log.Fatalf("Error marking organization-name flag as required in astro organization audit-logs command: %s", err.Error())
	}
	cmd.Flags().StringVarP(&auditLogsOutputFilePath, "output-file", "o", "", "Path to a file for storing exported audit logs")
	cmd.Flags().IntVarP(&auditLogsEarliestParam, "include", "i", auditLogsEarliestParamDefaultValue,
		"Number of days in the past to start exporting logs from. Minimum: 1. Maximum: 90.")
	return cmd
}

func newOrganizationUserRootCmd(out io.Writer) *cobra.Command {
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
	cmd.Flags().StringVarP(&updateRole, "role", "r", "", "The new role for the "+
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
	fmt.Println("This may take some time depending on how many days are being exported.")
	return orgExportAuditLogs(astroClient, out, orgName, auditLogsEarliestParam)
}

func userInvite(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
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
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if updateRole == "" {
		// no role was provided so ask the user for it
		updateRole = input.Text("enter a user organization role(ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER) to update user: ")
	}

	cmd.SilenceUsage = true
	return user.UpdateUserRole(email, updateRole, out, astroCoreClient)
}

func newOrganizationTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te"},
		Short:   "Manage teams in your Astro Organization",
		Long:    "Manage teams in your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newTeamCreateCmd(out),
		newOrganizationTeamListCmd(out),
		newTeamUpdateCmd(out),
		newTeamDeleteCmd(out),
		newOrganizationTeamUserRootCmd(out),
	)
	return cmd
}

func newOrganizationTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the teams in your Astro Organization",
		Long:    "List all the teams in your Astro Organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listTeams(cmd, out)
		},
	}
	return cmd
}

func listTeams(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.ListOrgTeams(out, astroCoreClient)
}

func newTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [team-id]",
		Aliases: []string{"up"},
		Short:   "Update an Astro team",
		Long:    "Update an Astro team",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamUpdate(cmd, out, args)
		},
	}
	cmd.Flags().StringVarP(&teamName, "name", "n", "", "The Team's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&teamDescription, "description", "d", "", "Description of the Team. If the description contains a space, specify the entire team description in quotes \"\"")
	return cmd
}

func teamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return team.UpdateTeam(id, teamName, teamDescription, out, astroCoreClient)
}

func newTeamCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Astro Team",
		Long:    "Create an Astro Team",
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamName, "name", "n", "", "The Team's name. If the name contains a space, specify the entire team within quotes \"\" ")
	cmd.Flags().StringVarP(&teamDescription, "description", "d", "", "Description of the Team. If the description contains a space, specify the entire team in quotes \"\"")
	return cmd
}

func teamCreate(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.CreateTeam(teamName, teamDescription, out, astroCoreClient)
}

func newTeamDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [team-id]",
		Aliases: []string{"de"},
		Short:   "Delete an Astro Team",
		Long:    "Delete an Astro Team",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamDelete(cmd, out, args)
		},
	}
	return cmd
}

func teamDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return team.Delete(id, out, astroCoreClient)
}

func newOrganizationTeamUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users in your Astro Team",
		Long:  "Manage users in your Astro Team.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newTeamRemoveUserCmd(out),
		newTeamAddUserCmd(out),
		newTeamListUsersCmd(out),
	)
	return cmd
}

func newTeamRemoveUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a user from an Astro Team",
		Long:  "Remove a user from an Astro Team",
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeTeamUser(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's unique identifier \"\" ")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "The User's unique identifier \"\"")
	return cmd
}

func removeTeamUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.RemoveUser(teamID, userID, out, astroCoreClient)
}

func newTeamAddUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a user to an Astro Team",
		Long:  "Add a user to an Astro Team",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addTeamUser(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's unique identifier \"\" ")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "The User's unique identifier \"\"")
	return cmd
}

func addTeamUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.AddUser(teamID, userID, out, astroCoreClient)
}

func newTeamListUsersCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists users in an Astro Team",
		Long:  "Lists users in an Astro Team",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listUsersCmd(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's id \"\" ")
	return cmd
}

func listUsersCmd(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.ListTeamUsers(teamID, out, astroCoreClient)
}
