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
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/spf13/cobra"
)

var (
	errInvalidOrganizationRoleKey      = errors.New("invalid organization role selection")
	orgSwitch                          = organization.Switch
	orgExportAuditLogs                 = organization.ExportAuditLogs
	wsSwitch                           = workspace.Switch
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
	shouldIncludeDefaultRoles          bool
	organizationListOutputFlags        output.Flags
	organizationUserListOutputFlags    output.Flags
	organizationTeamListOutputFlags    output.Flags
	forceTeam                          bool
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
		Short:   "List all Organizations you have access to",
		Long:    "List all Astro Organizations you have access to.",
		Example: `  astro organization list
  astro organization list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationList(cmd, out)
		},
	}
	organizationListOutputFlags.AddFlags(cmd)
	return cmd
}

func newOrganizationSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [organization name/id]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Organization",
		Long:    "Switch your active Organization and reset your Workspace context. After switching, your active Workspace is cleared unless you specify one with --workspace-id. Use --login-link to generate a login URL for switching on a different device.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  $ astro organization switch
  $ astro organization switch my-organization
  $ astro organization switch --login-link --workspace-id ws123456
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationSwitch(cmd, out, args)
		},
	}

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for organization switch")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "The Workspace's unique identifier")

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
		Long:    "Export Organization audit logs as a GZIP file. Includes all API and UI actions by Organization members for up to 90 days. Use --include to control how many days back to export (default: 1 day). Requires Organization Owner permissions.",
		Example: `
  $ astro organization audit-logs export
  $ astro organization audit-logs export --output-file audit-logs.gz --include 30
  $ astro organization audit-logs export --organization-name my-org --include 7
`,
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
		Example: `
  $ astro organization user invite user@company.com
  $ astro organization user invite user@company.com --role ORGANIZATION_BILLING_ADMIN
`,
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
		Long:    "List all users and their Organization-level roles.",
		Example: `  astro organization user list
  astro organization user list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listUsers(cmd, out)
		},
	}
	organizationUserListOutputFlags.AddFlags(cmd)
	return cmd
}

func newOrganizationUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [email]",
		Aliases: []string{"up"},
		Short:   "Update the role of a user your in Astro Organization",
		Long:    "Update the role of a user in your Astro Organization\n$astro user update [email] --role [" + allowedOrganizationRoleNames + "].",
		Example: `
  $ astro organization user update user@company.com --role ORGANIZATION_OWNER
  $ astro organization user update user@company.com --role ORGANIZATION_MEMBER
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return userUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateRole, "role", "r", "", "The new role for the "+
		"user. Possible values are "+allowedOrganizationRoleNamesProse)
	return cmd
}

func organizationList(cmd *cobra.Command, out io.Writer) error {
	format, err := organizationListOutputFlags.Resolve()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return organization.ListWithFormat(platformCoreClient, format, organizationListOutputFlags.Template, out)
}

func organizationSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	organizationNameOrID := ""

	if len(args) == 1 {
		organizationNameOrID = args[0]
	}

	if err := orgSwitch(organizationNameOrID, astroCoreClient, platformCoreClient, out, shouldDisplayLoginLink); err != nil {
		return err
	}

	if workspaceID != "" {
		return wsSwitch(workspaceID, astroCoreClient, out)
	}
	return nil
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
	format, err := organizationUserListOutputFlags.Resolve()
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return user.ListOrgUsersWithFormat(astroCoreClient, format, organizationUserListOutputFlags.Template, out)
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
		Example: `  astro organization team list
  astro organization team list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listTeams(cmd, out)
		},
	}
	organizationTeamListOutputFlags.AddFlags(cmd)
	return cmd
}

func listTeams(cmd *cobra.Command, out io.Writer) error {
	format, err := organizationTeamListOutputFlags.Resolve()
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return team.ListOrgTeamsWithFormat(astroCoreClient, format, organizationTeamListOutputFlags.Template, out)
}

func newTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [team-id]",
		Aliases: []string{"up"},
		Short:   "Update an Astro team",
		Long:    "Update a team's name, description, or Organization role. For IDP-managed teams, a confirmation prompt is shown unless --force is used. Team names are case-insensitively unique within an Organization.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  $ astro organization team update team123
  $ astro organization team update team123 --name "New Team Name" --role ORGANIZATION_MEMBER
  $ astro organization team update team123 --description "Updated description"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamUpdate(cmd, out, args)
		},
	}
	cmd.Flags().StringVarP(&teamName, "name", "n", "", "The Team's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().
		StringVarP(&teamDescription, "description", "d", "", "Description of the Team. If the description contains a space, specify the entire team description in quotes \"\"")
	cmd.Flags().StringVarP(&updateOrganizationRole, "role", "r", "", "The new role for the "+
		"team. Possible values are "+allowedOrganizationRoleNamesProse)
	cmd.Flags().BoolVarP(&forceTeam, "force", "f", false, "Force update: Skip confirmation for IDP-managed teams")
	return cmd
}

func teamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return team.UpdateTeam(id, teamName, teamDescription, updateOrganizationRole, forceTeam, out, astroCoreClient)
}

func newTeamCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Astro Team",
		Long:    "Create a team in your Organization. Teams let you assign Workspace and Deployment roles to groups of users at once. If your Organization uses an external identity provider (SCIM) for team sync, team creation through the CLI is blocked — manage teams in the IDP instead.",
		Example: `
  $ astro organization team create
  $ astro organization team create --name "My Team" --role ORGANIZATION_MEMBER
  $ astro organization team create --name "My Team" --description "Data engineering team" --role ORGANIZATION_OWNER
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamName, "name", "n", "", "The Team's name. If the name contains a space, specify the entire team within quotes \"\" ")
	cmd.Flags().StringVarP(&teamDescription, "description", "d", "", "Description of the Team. If the description contains a space, specify the entire team in quotes \"\"")
	cmd.Flags().StringVarP(&teamOrgRole, "role", "r", "", "The role for the token. Possible values are "+allowedOrganizationRoleNamesProse)

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
		Long:    "Permanently delete a team. All Workspace and Deployment role bindings for the team are removed, and members lose any access they had through the team (but keep directly assigned roles). This action cannot be undone.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  $ astro organization team delete
  $ astro organization team delete team123
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamDelete(cmd, out, args)
		},
	}
	cmd.Flags().BoolVarP(&forceTeam, "force", "f", false, "Force delete: Skip confirmation for IDP-managed teams")
	return cmd
}

func teamDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return team.Delete(id, forceTeam, out, astroCoreClient)
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

//nolint:dupl
func newTeamRemoveUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a user from an Astro Team",
		Long:  "Remove a user from a team. The user loses all Workspace and Deployment roles inherited through the team but keeps any directly assigned roles.",
		Example: `
  $ astro organization team user remove --team-id team123 --user-id user456
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeTeamUser(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's unique identifier \"\" ")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "The User's unique identifier \"\"")
	cmd.Flags().BoolVarP(&forceTeam, "force", "f", false, "Force remove: Skip confirmation for IDP-managed teams")
	return cmd
}

func removeTeamUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.RemoveUser(teamID, userID, forceTeam, out, astroCoreClient)
}

//nolint:dupl
func newTeamAddUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a user to an Astro Team",
		Long:  "Add a user to a team. The user immediately inherits all Workspace and Deployment roles assigned to the team.",
		Example: `
  $ astro organization team user add --team-id team123 --user-id user456
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addTeamUser(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's unique identifier \"\" ")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "The User's unique identifier \"\"")
	cmd.Flags().BoolVarP(&forceTeam, "force", "f", false, "Force add: Skip confirmation for IDP-managed teams")
	return cmd
}

func addTeamUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.AddUser(teamID, userID, forceTeam, out, astroCoreClient)
}

func newTeamListUsersCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists users in an Astro Team",
		Long:  "List all members of a team.",
		Example: `
  $ astro organization team user list
  $ astro organization team user list --team-id team123
`,
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
		Long:    "List all Organization-scoped API tokens. Organization tokens can hold roles at multiple levels (Organization, Workspace, Deployment) simultaneously.",
		Example: `
  $ astro organization token list
  $ astro organization token list --organization-id org123
`,
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
		Example: `
  $ astro organization token roles token123
`,
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
		Long:    "Create an Organization-scoped API token. The token value is displayed only once at creation and cannot be retrieved later — store it securely. Use --clean-output to print only the raw token value for scripts. Use --expiration to set a TTL in days (default: no expiration).",
		Example: `
  $ astro organization token create
  $ astro organization token create --name "CI/CD Token" --role ORGANIZATION_MEMBER
  $ astro organization token create --name "Deploy Token" --role ORGANIZATION_OWNER --expiration 365 --clean-output
`,
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
		Short:   "Update an Organization API token",
		Long:    "Update an Organization API token's name, description, or Organization-level role. Identify the token by its ID (positional argument) or current name (--name). This does not affect the token's Workspace or Deployment roles.",
		Example: `
  $ astro organization token update token123
  $ astro organization token update token123 --new-name "Updated Token" --role ORGANIZATION_OWNER
  $ astro organization token update --name "My Token" --new-name "Renamed Token" --description "Updated desc"
`,
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
		Example: `
  $ astro organization token rotate token123
  $ astro organization token rotate --name "My Token" --force
  $ astro organization token rotate token123 --clean-output
`,
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
		Long:    "Permanently revoke an Organization API token. All access the token grants — including any Workspace and Deployment roles — is immediately revoked.",
		Example: `
  $ astro organization token delete token123
  $ astro organization token delete --name "My Token" --force
`,
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
	return organization.ListTokenRoles(tokenID, astroCoreClient, astroCoreIamClient, out)
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
	return organization.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole, out, astroCoreClient, astroCoreIamClient)
}

//nolint:dupl
func rotateOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return organization.RotateToken(tokenID, name, cleanTokenOutput, forceRotate, out, astroCoreClient, astroCoreIamClient)
}

//nolint:dupl
func deleteOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return organization.DeleteToken(tokenID, name, forceDelete, out, astroCoreClient, astroCoreIamClient)
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
		Long:    "List all custom roles in your Organization. Custom roles define fine-grained permissions that can be assigned to users, teams, and tokens. Use --include-default-roles to also show the built-in system roles (Organization Member, Owner, etc.).",
		Example: `
  $ astro organization role list
  $ astro organization role list --include-default-roles
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRoles(cmd, out)
		},
	}
	cmd.Flags().BoolVarP(&shouldIncludeDefaultRoles, "include-default-roles", "i", false, "Should include default roles in response")

	return cmd
}

func listRoles(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return roleClient.ListOrgRoles(out, astroCoreClient, shouldIncludeDefaultRoles)
}
