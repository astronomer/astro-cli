package astro

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	roleClient "github.com/astronomer/astro-cli/internal/platform/astro/role"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	errInvalidOrganizationRoleKey      = errors.New("invalid organization role selection")
	orgSwitch                          = organization.Switch
	orgExportAuditLogs                 = organization.ExportAuditLogs
	wsSwitch                           = workspace.SwitchTo
	orgName                            string
	auditLogsOutputFilePath            string
	auditLogsOutput                    string
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
	organizationListOutput             string
	organizationClusterListOutput      string
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
		newOrganizationClusterRootCmd(out),
	)
	applyPreferredFlagsIn(cmd)
	return cmd
}

func newOrganizationListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Organizations you have access to",
		Long:    "List all Astro Organizations you have access to.",
		Example: `  astro organization list
  astro organization list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationList(cmd, out)
		},
	}
	cliout.AddOutputFlag(cmd, &organizationListOutput)
	return cmd
}

func newOrganizationSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [ORGANIZATION_NAME_OR_ID]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Organization",
		Long:    "Switch your active Organization and reset your Workspace context. After switching, your active Workspace is cleared unless you specify one with --workspace. Use --login-link to generate a login URL for switching on a different device.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  # Choose an Organization from a list
  astro organization switch

  # Switch to an Organization by its name or ID
  astro organization switch my-organization

  # Get a login link to switch on another device, and make a Workspace current
  astro organization switch --login-link --workspace <WORKSPACE_ID>

  # Switch, make a Workspace current, and print the result as JSON
  astro organization switch my-organization --workspace <WORKSPACE_ID> -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationSwitch(cmd, out, args)
		},
	}
	cliout.AddOutputFlag(cmd, &organizationSwitchOutput)

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for organization switch")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "The Workspace's unique identifier")
	addWorkspaceFlag(cmd.Flags(), "w", "Workspace to make current after the switch")

	return cmd
}

func newOrganizationAuditLogs(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "audit-logs",
		Aliases: []string{"al"},
		Short:   "Manage your Organization audit logs",
		Long:    "Manage your Organization audit logs.",
	}
	cmd.AddCommand(
		newOrganizationExportAuditLogs(out),
	)
	return cmd
}

func newOrganizationExportAuditLogs(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export",
		Aliases: []string{"e"},
		Short:   "Export your Organization audit logs in GZIP",
		Long:    "Export Organization audit logs as a GZIP file. Includes all API and UI actions by Organization members for up to 90 days. Use --include to control how many days back to export (default: 1 day). Requires Organization Owner permissions.",
		Example: `
  # Export the last day of audit logs
  astro organization audit-logs export

  # Export the last 30 days to a file
  astro organization audit-logs export --output-file audit-logs.gz --include 30

  # Export the last 7 days of another Organization
  astro organization audit-logs export --organization-name my-org --include 7

  # Export to a file, and print where it went as JSON
  astro organization audit-logs export --output-file audit-logs.gz -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationExportAuditLogs(cmd, out)
		},
	}
	cliout.AddOutputFlag(cmd, &auditLogsOutput)
	cmd.Flags().StringVarP(&orgName, "organization-name", "n", "", "Name of the Organization to manage audit logs for")
	// No shorthand: -o is --output, as on every other command.
	cmd.Flags().StringVar(&auditLogsOutputFilePath, "output-file", "", "Path to a file for storing exported audit logs. Defaults to a file in the current directory named for the Organization, the days and the date")
	cmd.Flags().IntVarP(&auditLogsEarliestParam, "include", "i", auditLogsEarliestParamDefaultValue,
		"Number of days in the past to start exporting logs from, from 1 to 90")
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
	cliout.AddOutputFlag(cmd, &organizationUserOutput)
	return cmd
}

func newOrganizationUserInviteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite [EMAIL]",
		Aliases: []string{"inv"},
		Short:   "Invite a user to your Astro Organization",
		Long:    "Invite a user to your Astro Organization. Without an email argument, you are prompted for one.",
		Example: `
  # Invite a user as an Organization member
  astro organization user invite user@company.com

  # Invite a user as a billing admin
  astro organization user invite user@company.com --role ORGANIZATION_BILLING_ADMIN
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return userInvite(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The role for the "+
		"user. Possible values are "+allowedOrganizationRoleNamesProse)
	return cmd
}

func newOrganizationUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the users in your Astro Organization",
		Long:    "List all users and their Organization-level roles.",
		Example: `  astro organization user list
  astro organization user list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listUsers(cmd, out)
		},
	}
	return cmd
}

func newOrganizationUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [EMAIL]",
		Aliases: []string{"up"},
		Short:   "Update the role of a user in your Astro Organization",
		Long:    "Update the role of a user in your Astro Organization. Without an email argument you choose the user from a list, and without --role you are prompted for the role.",
		Example: `
  # Make a user an Organization owner
  astro organization user update user@company.com --role ORGANIZATION_OWNER
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
	format, err := cliout.ParseFormat(organizationListOutput)
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return organization.ListWithFormat(astroV1Client, cliout.Renderer{Format: format, Out: out})
}

func organizationSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	format, err := cliout.ParseFormat(organizationSwitchOutput)
	if err != nil {
		return err
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	organizationNameOrID := ""

	if len(args) == 1 {
		organizationNameOrID = args[0]
	}

	asks := questionsTo(cmd, format, out)
	switched, err := orgSwitch(organizationNameOrID, astroV1Client, asks, shouldDisplayLoginLink)
	if err != nil {
		return err
	}
	res := &organization.SwitchResult{Organization: switched.Organization}
	r := cliout.Renderer{Format: format, Out: out}

	if workspaceID != "" {
		ws, err := wsSwitch(workspaceID, astroV1Client, asks)
		if err != nil {
			// The Organization did switch, and text has always said so
			// before the error. Under json the error object is all of it.
			if format == cliout.FormatText {
				if werr := cliout.WriteText(out, func(b *bufio.Writer) { fmt.Fprintln(b, switchedLine(switched.Changed)) }); werr != nil {
					return werr
				}
			}
			return err
		}
		res.Workspace = ws
		return emitOrganizationSwitch(r, res, switched.Changed, true)
	}
	// Which Workspace the switch left current is read back only for json:
	// text does not show it, and finding it costs a list. The switch is
	// already written by then, so a failed read does not fail the run: the
	// result says no Workspace is known to be current, and stderr says why.
	if format == cliout.FormatJSON {
		ws, err := workspace.Current(astroV1Client)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "The Organization was switched, but the current Workspace could not be read: %s\n", err)
		}
		res.Workspace = ws
	}
	return emitOrganizationSwitch(r, res, switched.Changed, false)
}

func organizationExportAuditLogs(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(auditLogsOutput)
	if err != nil {
		// -o was --output-file's shorthand here until it became --output,
		// so a script passing -o <path> lands here.
		return fmt.Errorf("%w; -o is the output format, and --output-file takes the path", err)
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	fmt.Fprintln(questionsTo(cmd, format, out), "This may take some time depending on how many days are being exported.")
	export, err := orgExportAuditLogs(astroV1Client,
		orgName, auditLogsOutputFilePath, auditLogsEarliestParam)
	if err != nil {
		return err
	}
	return emitAuditLogExport(cliout.Renderer{Format: format, Out: out}, export)
}

func userInvite(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	} else {
		// no email was provided so ask the user for it
		answer, err := input.Text("enter email address to invite a user: ", input.AnsweredBy("the email address as an argument"))
		if err != nil {
			return err
		}
		email = answer
	}

	cmd.SilenceUsage = true
	inv, err := user.CreateInvite(email, role, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &inv, fmt.Sprintf("invite for %s with role %s created", inv.Email, inv.Role))
}

func listUsers(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationUserOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return user.ListOrgUsersWithFormat(astroV1Client, cliout.Renderer{Format: format, Out: out})
}

func userUpdate(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if updateRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("enter a user Organization role("+allowedOrganizationRoleNames+") to update user: ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		updateRole = answer
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.UpdateUserRole(email, updateRole, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &u, fmt.Sprintf("The user %s role was successfully updated to %s", u.Email, u.OrgRole))
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
	cliout.AddOutputFlag(cmd, &organizationTeamOutput)
	return cmd
}

func newOrganizationTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the teams in your Astro Organization",
		Long:    "List all the teams in your Astro Organization",
		Example: `  astro organization team list
  astro organization team list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listTeams(cmd, out)
		},
	}
	return cmd
}

func listTeams(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return team.ListOrgTeamsWithFormat(astroV1Client, cliout.Renderer{Format: format, Out: out})
}

func newTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TEAM_ID]",
		Aliases: []string{"up"},
		Short:   "Update an Astro team",
		Long:    "Update a team's name, description, or Organization role. For IDP-managed teams, a confirmation prompt is shown unless --yes is used. Team names are case-insensitively unique within an Organization.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  # Rename a team and change its Organization role
  astro organization team update <TEAM_ID> --name "New Team Name" --role ORGANIZATION_MEMBER

  # Change a team's description
  astro organization team update <TEAM_ID> --description "Updated description"
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
	cmd.Flags().BoolVarP(&forceTeam, "yes", "y", false, "Don't ask for confirmation on an IDP-managed team")
	return cmd
}

func teamUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	} else if err := mayPick("a team", teamIDAnswer); err != nil {
		return err
	}

	upd, err := team.UpdateTeam(id, teamName, teamDescription, updateOrganizationRole, forceTeam, astroV1Client)
	return renderTeamUpdate(format, out, upd, err)
}

func newTeamCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Astro Team",
		Long:    "Create a team in your Organization. Teams let you assign Workspace and Deployment roles to groups of users at once. If your Organization uses an external identity provider (SCIM) for team sync, team creation through the CLI is blocked — manage teams in the IDP instead.",
		Example: `
  # Create a team, choosing its name and role when prompted
  astro organization team create

  # Create a team with a name, a description and a role
  astro organization team create --name "My Team" --description "Data engineering team" --role ORGANIZATION_MEMBER
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamName, "name", "n", "", "The Team's name. If the name contains a space, specify the entire team within quotes \"\" ")
	cmd.Flags().StringVarP(&teamDescription, "description", "d", "", "Description of the Team. If the description contains a space, specify the entire team in quotes \"\"")
	cmd.Flags().StringVarP(&teamOrgRole, "role", "r", "", "The role for the new team. Possible values are "+allowedOrganizationRoleNamesProse)

	return cmd
}

func teamCreate(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	if teamOrgRole == "" {
		// Refused before the line introducing the choice, so a run that
		// cannot ask prints nothing.
		if err := mayPickRole(); err != nil {
			return err
		}
		fmt.Println("select a Organization Role for the new team:")
		// no role was provided so ask the user for it
		teamOrgRole, err = selectOrganizationRole()
		if err != nil {
			return err
		}
	}
	t, err := team.CreateTeam(teamName, teamDescription, teamOrgRole, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &t, fmt.Sprintf("Astro Team %s was successfully created", t.Name))
}

func newTeamDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [TEAM_ID]",
		Aliases: []string{"de"},
		Short:   "Delete an Astro Team",
		Long:    "Permanently delete a team. All Workspace and Deployment role bindings for the team are removed, and members lose any access they had through the team (but keep directly assigned roles). This action cannot be undone.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  # Delete a team by its ID
  astro organization team delete <TEAM_ID>

  # Choose the team to delete from a list
  astro organization team delete
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return teamDelete(cmd, out, args)
		},
	}
	cmd.Flags().BoolVarP(&forceTeam, "yes", "y", false, "Don't ask for confirmation on an IDP-managed team")
	return cmd
}

func teamDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	} else if err := mayPick("a team", teamIDAnswer); err != nil {
		return err
	}

	r, err := team.Delete(id, forceTeam, astroV1Client)
	if err != nil || r == nil {
		return err
	}
	return renderLines(format, out, r, fmt.Sprintf("Astro Team %s was successfully deleted", r.Name))
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

//nolint:dupl // the duplication is acceptable here
func newTeamRemoveUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a user from an Astro Team",
		Long:  "Remove a user from a team. The user loses all Workspace and Deployment roles inherited through the team but keeps any directly assigned roles.",
		Example: `
  astro organization team user remove --team-id <TEAM_ID> --user-id <USER_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeTeamUser(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's unique identifier \"\" ")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "The User's unique identifier \"\"")
	cmd.Flags().BoolVarP(&forceTeam, "yes", "y", false, "Don't ask for confirmation on an IDP-managed team")
	return cmd
}

func removeTeamUser(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	if err := mayPickTeamAndUser("a team member"); err != nil {
		return err
	}
	m, err := team.RemoveUser(teamID, userID, forceTeam, astroV1Client)
	if err != nil {
		return err
	}
	return renderMembership(format, out, m)
}

// mayPickTeamAndUser refuses, under --output json, a team user command given
// no --team-id or no --user-id, before it fetches anything. userWhat is the
// user picker's question: a user of the Organization to add, or a member of
// the team to remove.
func mayPickTeamAndUser(userWhat string) error {
	if teamID == "" {
		if err := mayPick("a team", teamIDFlag); err != nil {
			return err
		}
	}
	if userID == "" {
		return mayPick(userWhat, userIDFlag)
	}
	return nil
}

//nolint:dupl // the duplication is acceptable here
func newTeamAddUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a user to an Astro Team",
		Long:  "Add a user to a team. The user immediately inherits all Workspace and Deployment roles assigned to the team.",
		Example: `
  astro organization team user add --team-id <TEAM_ID> --user-id <USER_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addTeamUser(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's unique identifier \"\" ")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "The User's unique identifier \"\"")
	cmd.Flags().BoolVarP(&forceTeam, "yes", "y", false, "Don't ask for confirmation on an IDP-managed team")
	return cmd
}

func addTeamUser(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	if err := mayPickTeamAndUser("a user"); err != nil {
		return err
	}
	m, err := team.AddUser(teamID, userID, forceTeam, astroV1Client)
	if err != nil {
		return err
	}
	return renderMembership(format, out, m)
}

func newTeamListUsersCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists users in an Astro Team",
		Long:  "List all members of a team.",
		Example: `
  # List the members of a team
  astro organization team user list --team-id <TEAM_ID>

  # Choose the team from a list
  astro organization team user list
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listUsersCmd(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&teamID, "team-id", "t", "", "The Team's id \"\" ")
	return cmd
}

func listUsersCmd(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationTeamOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	if teamID == "" {
		if err := mayPick("a team", teamIDFlag); err != nil {
			return err
		}
	}
	list, err := team.ListTeamUsers(teamID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTeamMembers(format, out, &list)
}

// org tokens

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
	cmd.PersistentFlags().StringVar(&organizationID, "organization-id", "", "Organization where you would like to manage tokens")
	cliout.AddOutputFlag(cmd, &organizationTokenOutput)
	return cmd
}

func newOrganizationTokenListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the API tokens in an Astro Organization",
		Long:    "List all Organization-scoped API tokens. Organization tokens can hold roles at multiple levels (Organization, Workspace, Deployment) simultaneously.",
		Example: `
  # List the tokens in the current Organization
  astro organization token list

  # List the tokens in another Organization
  astro organization token list --organization-id <ORGANIZATION_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationToken(cmd, out)
		},
	}
	return cmd
}

func newOrganizationTokenListRolesCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roles [TOKEN_ID]",
		Short: "List roles for an organization API token",
		Long:  "List roles for an organization API token",
		Example: `
  astro organization token roles <TOKEN_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationTokenRoles(cmd, args, out)
		},
	}
	return cmd
}

func newOrganizationTokenCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an API token in an Astro Organization",
		Long:    "Create an Organization-scoped API token. The token value is displayed only once at creation and cannot be retrieved later — store it securely. Use --clean-output to print only the raw token value for scripts. Use --expiration to set a TTL in days (default: no expiration).",
		Example: `
  # Create a token, choosing its name and role when prompted
  astro organization token create

  # Create a member token
  astro organization token create --name "CI/CD Token" --role ORGANIZATION_MEMBER

  # Create an owner token that expires in a year, printing only the token
  astro organization token create --name "Deploy Token" --role ORGANIZATION_OWNER --expiration 365 --clean-output
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createOrganizationToken(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&tokenName, "name", "n", "", "The token's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Description of the token. If the description contains a space, specify the entire description within quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The role for the token. Possible values are "+allowedOrganizationRoleNamesProse)
	cmd.Flags().IntVarP(&tokenExpiration, "expiration", "e", 0, "Expiration of the token in days, from 1 to 3650. Without it, the token does not expire")
	return cmd
}

func newOrganizationTokenUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TOKEN_ID]",
		Aliases: []string{"up"},
		Short:   "Update an Organization API token",
		Long:    "Update an Organization API token's name, description, or Organization-level role. Identify the token by its ID (positional argument) or current name (--name). This does not affect the token's Workspace or Deployment roles.",
		Example: `
  # Rename a token and change its Organization role
  astro organization token update <TOKEN_ID> --new-name "Updated Token" --role ORGANIZATION_OWNER

  # Find a token by its name and change its description
  astro organization token update --name "My Token" --description "Updated description"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateOrganizationToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The current name of the token. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenName, "new-name", "n", "", "The token's new name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Updated description of the token. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The new role for the token. Possible values are "+allowedOrganizationRoleNamesProse+". Without it, the token keeps its role")
	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newOrganizationTokenRotateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate [TOKEN_ID]",
		Aliases: []string{"ro"},
		Short:   "Rotate a Organization API token",
		Long:    "Rotate a Organization API token. You can only rotate Organization API tokens. You cannot rotate Workspace API tokens with this command",
		Example: `
  # Rotate a token by its ID
  astro organization token rotate <TOKEN_ID>

  # Rotate a token by its name without confirming, printing only the new token
  astro organization token rotate --name "My Token" --yes --clean-output
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rotateOrganizationToken(cmd, args, out)
		},
	}
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be rotated. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceRotate, "yes", "y", false, "Don't ask for confirmation before rotating the Organization API token")

	return cmd
}

func newOrganizationTokenDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [TOKEN_ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Organization API token or remove an Organization API token from a Organization",
		Long:    "Permanently revoke an Organization API token. All access the token grants — including any Workspace and Deployment roles — is immediately revoked.",
		Example: `
  # Delete a token by its ID
  astro organization token delete <TOKEN_ID>

  # Delete a token by its name without confirming
  astro organization token delete --name "My Token" --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteOrganizationToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be deleted. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Don't ask for confirmation before deleting or removing the API token")

	return cmd
}

func selectOrganizationRole() (string, error) {
	list := picker.List{
		Header:  []string{"ROLE"},
		Ask:     []input.Option{input.About("a role"), input.AnsweredBy("--role")},
		Invalid: errInvalidOrganizationRoleKey,
	}
	for _, role := range validOrganizationRoles {
		list.AddRow(false, role)
	}
	i, err := list.Pick(os.Stdout, os.Stdin)
	if err != nil {
		return "", err
	}
	return validOrganizationRoles[i], nil
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
	cliout.AddOutputFlag(cmd, &organizationRoleOutput)
	return cmd
}

func newOrganizationRoleListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the roles in your Astro Organization",
		Long:    "List all custom roles in your Organization. Custom roles define fine-grained permissions that can be assigned to users, teams, and tokens. Use --include-default-roles to also show the built-in system roles (Organization Member, Owner, etc.).",
		Example: `
  # List the custom roles
  astro organization role list

  # Include the built-in roles
  astro organization role list --include-default-roles

  # List the roles as JSON
  astro organization role list -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRoles(cmd, out)
		},
	}
	cmd.Flags().BoolVarP(&shouldIncludeDefaultRoles, "include-default-roles", "i", false, "Should include default roles in response")

	return cmd
}

func listRoles(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationRoleOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	roles, err := roleClient.ListData(astroV1Client, shouldIncludeDefaultRoles)
	if err != nil {
		return err
	}
	return emitRoles(cliout.Renderer{Format: format, Out: out}, roles)
}

func newOrganizationClusterRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cluster",
		Aliases: []string{"cl", "clusters"},
		Short:   "Manage clusters in your Astro Organization",
		Long:    "Manage clusters in your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newOrganizationClusterListCmd(out),
	)
	return cmd
}

func newOrganizationClusterListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the clusters in your Astro Organization",
		Long:    "List all the clusters in your Astro Organization. Only Dedicated and Hybrid clusters are listed. Standard Deployments run on clusters that Astronomer manages, so an Organization with only Standard Deployments has no clusters to list.",
		Example: `  astro organization cluster list
  astro organization cluster list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listClusters(cmd, out)
		},
	}
	cliout.AddOutputFlag(cmd, &organizationClusterListOutput)
	return cmd
}

func listClusters(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(organizationClusterListOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return organization.ListClustersWithFormat(astroV1Client, cliout.Renderer{Format: format, Out: out})
}
