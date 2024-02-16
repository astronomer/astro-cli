package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/cloud/organization"
	roleClient "github.com/astronomer/astro-cli/cloud/role"
	"github.com/astronomer/astro-cli/cloud/team"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/spf13/cobra"
)

var (
	errInvalidOrganizationRoleKey      = errors.New("invalid organization role selection")
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
	organizationID                     string
	updateOrganizationRole             string
	teamOrgRole                        string
	validOrganizationRoles             []string
)

const (
	allowedOrganizationRoleNames      = "ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER"
	allowedOrganizationRoleNamesProse = "ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, and ORGANIZATION_OWNER"
)

func init() {
	validOrganizationRoles = []string{"ORGANIZATION_MEMBER", "ORGANIZATION_BILLING_ADMIN", "ORGANIZATION_OWNER"}
}

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
		newOrganizationTokenRootCmd(out),
		newOrganizationRoleRootCmd(out),
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
		Short:   "Manage your Organization audit logs.",
		Long:    "Manage your Organization audit logs.",
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
		Short:   "Export your Organization audit logs in GZIP. Requires Organization Owner permissions.",
		Long:    "Export your Organization audit logs in GZIP. Requires Organization Owner permissions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationExportAuditLogs(cmd)
		},
	}
	cmd.Flags().StringVarP(&orgName, "organization-name", "n", "", "Name of the Organization to manage audit logs for.")
	cmd.Flags().StringVarP(&auditLogsOutputFilePath, "output-file", "o", "", "Path to a file for storing exported audit logs")
	cmd.Flags().IntVarP(&auditLogsEarliestParam, "include", "i", auditLogsEarliestParamDefaultValue,
		"Number of days in the past to start exporting logs from. Minimum: 1. Maximum: 90.")
	return cmd
}

func newOrganizationUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us", "users"},
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
		Long:    "Invite a user to your Astro Organization\n$astro user invite [email] --role [" + allowedOrganizationRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userInvite(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The role for the "+
		"user. Possible values are"+allowedOrganizationRoleNamesProse)
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
		Long:    "Update the role of a user in your Astro Organization\n$astro user update [email] --role [" + allowedOrganizationRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateRole, "role", "r", "", "The new role for the "+
		"user. Possible values are "+allowedOrganizationRoleNamesProse)
	return cmd
}

func organizationList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return orgList(out, platformCoreClient)
}

func organizationSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	organizationNameOrID := ""

	if len(args) == 1 {
		organizationNameOrID = args[0]
	}

	return orgSwitch(organizationNameOrID, astroCoreClient, platformCoreClient, out, shouldDisplayLoginLink)
}

func organizationExportAuditLogs(cmd *cobra.Command) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	fmt.Println("This may take some time depending on how many days are being exported.")
	return orgExportAuditLogs(astroCoreClient, platformCoreClient,
		orgName, auditLogsOutputFilePath, auditLogsEarliestParam)
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
		updateRole = input.Text("enter a user Organization role(" + allowedOrganizationRoleNames + ") to update user: ")
	}

	cmd.SilenceUsage = true
	return user.UpdateUserRole(email, updateRole, out, astroCoreClient)
}

func newOrganizationTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
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
	cmd.Flags().
		StringVarP(&teamDescription, "description", "d", "", "Description of the Team. If the description contains a space, specify the entire team description in quotes \"\"")
	cmd.Flags().StringVarP(&updateOrganizationRole, "role", "r", "", "The new role for the "+
		"team. Possible values are "+allowedOrganizationRoleNamesProse)
	return cmd
}

func teamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return team.UpdateTeam(id, teamName, teamDescription, updateOrganizationRole, out, astroCoreClient)
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
	cmd.Flags().StringVarP(&teamOrgRole, "role", "r", "", "The role for the token. Possible values are"+allowedOrganizationRoleNamesProse)

	return cmd
}

func teamCreate(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	if teamOrgRole == "" {
		fmt.Println("select a Organization Role for the new team:")
		// no role was provided so ask the user for it
		var err error
		teamOrgRole, err = selectOrganizationRole()
		if err != nil {
			return err
		}
	}
	return team.CreateTeam(teamName, teamDescription, teamOrgRole, out, astroCoreClient)
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
		Use:     "user",
		Aliases: []string{"us", "users"},
		Short:   "Manage users in your Astro Team",
		Long:    "Manage users in your Astro Team.",
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

// org tokens

//nolint:dupl
func newOrganizationTokenRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "token",
		Aliases: []string{"to"},
		Short:   "Manage tokens in your Astro Organization",
		Long:    "Manage tokens in your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newOrganizationTokenListCmd(out),
		newOrganizationTokenListRolesCmd(out),
		newOrganizationTokenCreateCmd(out),
		newOrganizationTokenUpdateCmd(out),
		newOrganizationTokenRotateCmd(out),
		newOrganizationTokenDeleteCmd(out),
	)
	cmd.PersistentFlags().StringVar(&organizationID, "organization-id", "", "organization where you would like to manage tokens")
	return cmd
}

//nolint:dupl
func newOrganizationTokenListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the API tokens in an Astro Organization",
		Long:    "List all the API tokens in an Astro Organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationToken(cmd, out)
		},
	}
	return cmd
}

//nolint:dupl
func newOrganizationTokenListRolesCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roles [TOKEN_ID]",
		Short: "List roles for an organization API token",
		Long:  "List roles for an organization API token\n$astro organization token roles [TOKEN_ID] ",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationTokenRoles(cmd, args, out)
		},
	}
	return cmd
}

//nolint:dupl
func newOrganizationTokenCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an API token in an Astro Organization",
		Long:    "Create an API token in an Astro Organization\n$astro organization token create --name [token name] --role [" + allowedOrganizationRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createOrganizationToken(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&tokenName, "name", "n", "", "The token's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Description of the token. If the description contains a space, specify the entire description within quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The role for the token. Possible values are "+allowedOrganizationRoleNamesProse)
	cmd.Flags().IntVarP(&tokenExpiration, "expiration", "e", 0, "Expiration of the token in days. If the flag isn't used the token won't have an expiration. Must be between 1 and 3650 days. ")
	return cmd
}

//nolint:dupl
func newOrganizationTokenUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TOKEN_ID]",
		Aliases: []string{"up"},
		Short:   "Update a Organization or Organaization API token",
		Long:    "Update a Organization or Organaization API token that has a role in an Astro Organization\n$astro organization token update [TOKEN_ID] --name [new token name] --role [" + allowedOrganizationRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateOrganizationToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The current name of the token. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenName, "new-name", "n", "", "The token's new name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "updated description of the token. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The new role for the token. Possible values are "+allowedOrganizationRoleNamesProse)
	return cmd
}

//nolint:dupl
func newOrganizationTokenRotateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate [TOKEN_ID]",
		Aliases: []string{"ro"},
		Short:   "Rotate a Organization API token",
		Long:    "Rotate a Organization API token. You can only rotate Organization API tokens. You cannot rotate Workspace API tokens with this command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rotateOrganizationToken(cmd, args, out)
		},
	}
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be rotated. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceRotate, "force", "f", false, "Rotate the Organization API token without showing a warning")

	return cmd
}

//nolint:dupl
func newOrganizationTokenDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [TOKEN_ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Organization API token or remove an Organization API token from a Organization",
		Long:    "Delete a Organization API token or remove an Organization API token from a Organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteOrganizationToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be deleted. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Delete or remove the API token without showing a warning")

	return cmd
}

//nolint:dupl
func listOrganizationToken(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return organization.ListTokens(astroCoreClient, out)
}

//nolint:dupl
func listOrganizationTokenRoles(cmd *cobra.Command, args []string, out io.Writer) error {
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}
	cmd.SilenceUsage = true
	return organization.ListTokenRoles(tokenID, astroCoreClient, out)
}

//nolint:dupl
func createOrganizationToken(cmd *cobra.Command, out io.Writer) error {
	if tokenName == "" {
		// no role was provided so ask the user for it
		tokenName = input.Text("Enter a name for the new Organization API token: ")
	}
	if tokenRole == "" {
		fmt.Println("select a Organization Role for the new API token:")
		// no role was provided so ask the user for it
		var err error
		tokenRole, err = selectOrganizationRole()
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true

	return organization.CreateToken(tokenName, tokenDescription, tokenRole, tokenExpiration, cleanTokenOutput, out, astroCoreClient)
}

//nolint:dupl
func updateOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return organization.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole, out, astroCoreClient)
}

//nolint:dupl
func rotateOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return organization.RotateToken(tokenID, name, cleanTokenOutput, forceRotate, out, astroCoreClient)
}

//nolint:dupl
func deleteOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return organization.DeleteToken(tokenID, name, forceDelete, out, astroCoreClient)
}

//nolint:dupl
func selectOrganizationRole() (string, error) {
	tokenRolesMap := map[string]string{}
	tab := &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "ROLE"},
	}
	for i := range validOrganizationRoles {
		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			validOrganizationRoles[i],
		}, false)
		tokenRolesMap[strconv.Itoa(index)] = validOrganizationRoles[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := tokenRolesMap[choice]
	if !ok {
		return "", errInvalidOrganizationRoleKey
	}
	return selected, nil
}

func newOrganizationRoleRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "role",
		Aliases: []string{"ro", "roles"},
		Short:   "Manage roles in your Astro Organization",
		Long:    "Manage roles in your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newOrganizationRoleListCmd(out),
	)
	return cmd
}

func newOrganizationRoleListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the roles in your Astro Organization",
		Long:    "List all the roles in your Astro Organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRoles(cmd, out)
		},
	}
	return cmd
}

func listRoles(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return roleClient.ListOrgRoles(out, astroCoreClient)
}
