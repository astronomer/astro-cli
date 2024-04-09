package cloud

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/team"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/cloud/workspace"
	workspacetoken "github.com/astronomer/astro-cli/cloud/workspace-token"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	errInvalidWorkspaceRoleKey = errors.New("invalid workspace role selection")
	workspaceID                string
	addWorkspaceRole           string
	updateWorkspaceRole        string
	workspaceName              string
	workspaceDescription       string
	enforceCD                  string
	tokenName                  string
	tokenDescription           string
	tokenRole                  string
	orgTokenName               string
	tokenID                    string
	orgTokenID                 string
	workspaceTokenID           string
	cleanTokenOutput           bool
	forceRotate                bool
	tokenExpiration            int
	validWorkspaceRoles        []string
)

const (
	allowedWorkspaceRoleNames      = "WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR, WORKSPACE_OWNER"
	allowedWorkspaceRoleNamesProse = "WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR, and WORKSPACE_OWNER"
)

func init() {
	validWorkspaceRoles = []string{"WORKSPACE_MEMBER", "WORKSPACE_AUTHOR", "WORKSPACE_OPERATOR", "WORKSPACE_OWNER"}
}

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astro Workspaces",
		Long:    "Create and manage Workspaces on Astro. Workspaces can contain multiple Deployments and can be shared across users.",
	}
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceSwitchCmd(out),
		newWorkspaceCreateCmd(out),
		newWorkspaceUpdateCmd(out),
		newWorkspaceDeleteCmd(out),
		newWorkspaceUserRootCmd(out),
		newWorkspaceTokenRootCmd(out),
		newWorkspaceTeamRootCmd(out),
	)
	return cmd
}

func newWorkspaceListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astro Workspaces in your organization",
		Long:    "List all Astro Workspaces in your organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [workspace_id]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Astro Workspace",
		Long:    "Switch to a different Astro Workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Astro Workspace",
		Long:    "Create an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceName, "name", "n", "", "The Workspace's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&workspaceDescription, "description", "d", "", "Description of the Workspace. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&enforceCD, "enforce-cicd", "e", "OFF", "Provide this flag either ON/OFF. ON means deploys to deployments must use an API Key or Token. This essentially forces Deploys to happen through CI/CD")
	return cmd
}

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [workspace_id]",
		Aliases: []string{"up"},
		Short:   "Update an Astro Workspace",
		Long:    "Update an Astro Workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, out, args)
		},
	}
	cmd.Flags().StringVarP(&workspaceName, "name", "n", "", "The Workspace's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&workspaceDescription, "description", "d", "", "Description of the Workspace. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&enforceCD, "enforce-cicd", "e", "OFF", "Provide this flag either ON/OFF. ON means deploys to deployments must use an API Key or Token. This essentially forces Deploys to happen through CI/CD")
	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [workspace_id]",
		Aliases: []string{"de"},
		Short:   "Delete an Astro Workspace",
		Long:    "Delete an Astro Workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us", "users"},
		Short:   "Manage users in your Astro Workspace",
		Long:    "Manage users in your Astro Workspace.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newWorkspaceUserListCmd(out),
		newWorkspaceUserUpdateCmd(out),
		newWorkspaceUserRemoveCmd(out),
		newWorkspaceUserAddCmd(out),
	)
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace where you'd like to manage users")

	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [email]",
		Short: "Add a user to an Astro Workspace with a specific role",
		Long:  "Add a user to an Astro Workspace with a specific role\n$astro workspace user add [email] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&addWorkspaceRole, "role", "r", "WORKSPACE_MEMBER", "The role for the "+
		"new user. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newWorkspaceUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the users in an Astro Workspace",
		Long:    "List all the users in an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceUser(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [email]",
		Aliases: []string{"up"},
		Short:   "Update a the role of a user in an Astro Workspace",
		Long:    "Update the role of a user in an Astro Workspace\n$astro workspace user update [email] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateWorkspaceRole, "role", "r", "", "The new role for the "+
		"user. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newWorkspaceUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a user from an Astro Workspace",
		Long:    "Remove a user from an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceUser(cmd, args, out)
		},
	}
	return cmd
}

//nolint:dupl
func newWorkspaceTokenRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "token",
		Aliases: []string{"to"},
		Short:   "Manage tokens in your Astro Workspace",
		Long:    "Manage tokens in your Astro Workspace.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newWorkspaceTokenListCmd(out),
		newWorkspaceTokenCreateCmd(out),
		newWorkspaceTokenUpdateCmd(out),
		newWorkspaceTokenRotateCmd(out),
		newWorkspaceTokenDeleteCmd(out),
		newWorkspaceTokenAddOrgTokenCmd(out),
		newWorkspaceOrgTokenManageCmd(out),
	)
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace where you would like to manage tokens")
	return cmd
}

//nolint:dupl
func newWorkspaceTokenListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the API tokens in an Astro Workspace",
		Long:    "List all the API tokens in an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceToken(cmd, out)
		},
	}
	return cmd
}

//nolint:dupl
func newWorkspaceTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te", "teams"},
		Short:   "Manage teams in your Astro Workspace",
		Long:    "Manage teams in your Astro Workspace.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newWorkspaceTeamListCmd(out),
		newWorkspaceTeamUpdateCmd(out),
		newWorkspaceTeamRemoveCmd(out),
		newWorkspaceTeamAddCmd(out),
	)
	return cmd
}

//nolint:dupl
func newWorkspaceTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the teams in an Astro Workspace",
		Long:    "List all the teams in an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceTeam(cmd, out)
		},
	}
	return cmd
}

//nolint:dupl
func newWorkspaceTokenCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an API token in an Astro Workspace",
		Long:    "Create an API token in an Astro Workspace\n$astro workspace token create --name [token name] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createWorkspaceToken(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&tokenName, "name", "n", "", "The token's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Description of the token. If the description contains a space, specify the entire description within quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The role for the "+
		"token. Possible values are "+allowedWorkspaceRoleNamesProse)
	cmd.Flags().IntVarP(&tokenExpiration, "expiration", "e", 0, "Expiration of the token in days. If the flag isn't used the token won't have an expiration. Must be between 1 and 3650 days. ")
	return cmd
}

//nolint:dupl
func newWorkspaceTokenUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TOKEN_ID]",
		Aliases: []string{"up"},
		Short:   "Update a Workspace or Organaization API token",
		Long:    "Update a Workspace or Organaization API token that has a role in an Astro Workspace\n$astro workspace token update [TOKEN_ID] --name [new token name] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The current name of the token. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenName, "new-name", "n", "", "The token's new name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "updated description of the token. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The new role for the "+
		"token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

//nolint:dupl
func newWorkspaceTokenRotateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate [TOKEN_ID]",
		Aliases: []string{"ro"},
		Short:   "Rotate a Workspace API token",
		Long:    "Rotate a Workspace API token. You can only rotate Workspace API tokens. You cannot rotate Organization API tokens with this command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rotateWorkspaceToken(cmd, args, out)
		},
	}
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be rotated. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceRotate, "force", "f", false, "Rotate the Workspace API token without showing a warning")

	return cmd
}

//nolint:dupl
func newWorkspaceTokenDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [TOKEN_ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Workspace API token or remove an Organization API token from a Workspace",
		Long:    "Delete a Workspace API token or remove an Organization API token from a Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteWorkspaceToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be deleted. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Delete or remove the API token without showing a warning")

	return cmd
}

//nolint:dupl
func newWorkspaceTokenAddOrgTokenCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [ORG_TOKEN_ID]",
		Short: "Add an Organization API token to an Astro Workspace",
		Long:  "Add an Organization API token to an Astro Workspace\n$astro workspace token add [ORG_TOKEN_NAME] --name [new token name] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenToWorkspace(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to add to a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Workspace role to grant to the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

//nolint:dupl
func newWorkspaceOrgTokenManageCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organization-token",
		Short: "Manage organization tokens in a workspace",
		Long:  "Manage organization tokens in a workspace",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newAddOrganizationTokenWorkspaceRole(out),
		newUpdateOrganizationTokenWorkspaceRole(out),
		newRemoveOrganizationTokenWorkspaceRole(out),
		newListOrganizationTokensInWorkspace(out),
	)
	return cmd
}

func newAddOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [ORG_TOKEN_ID]",
		Short: "Add an Organization API token to a Workspace",
		Long:  "Add an Organization API token to a Workspace\n$astro workspace token organization-token add [ORG_TOKEN_ID] --name [token name] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenWorkspaceRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to add to a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Workspace role to grant to the "+
		"Organization API token. Possible values are"+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newUpdateOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [ORG_TOKEN_ID]",
		Short: "Update an Organization API token's Workspace Role",
		Long:  "Update an Organization API token's Workspace Role\n$astro workspace token organization-token update [ORG_TOKEN_ID] --name [token name] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateOrgTokenWorkspaceRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to update in a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Workspace role to update the "+
		"Organization API token. Possible values are"+allowedWorkspaceRoleNamesProse)
	return cmd
}

func addOrgTokenWorkspaceRole(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		orgTokenID = strings.ToLower(args[0])
	}
	if tokenRole == "" {
		// no role was provided so ask the user for it
		tokenRole = input.Text("Enter a role for the API token. Possible values are " + allowedWorkspaceRoleNamesProse + ": ")
	}
	cmd.SilenceUsage = true

	return workspacetoken.UpsertOrgTokenWorkspaceRole(orgTokenID, orgTokenName, tokenRole, workspaceID, "create", out, astroCoreClient, astroCoreIamClient)
}

func updateOrgTokenWorkspaceRole(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		orgTokenID = strings.ToLower(args[0])
	}
	if tokenRole == "" {
		// no role was provided so ask the user for it
		tokenRole = input.Text("Enter a role for the new Workspace API token. Possible values are " + allowedWorkspaceRoleNamesProse + ": ")
	}
	cmd.SilenceUsage = true

	return workspacetoken.UpsertOrgTokenWorkspaceRole(orgTokenID, orgTokenName, tokenRole, workspaceID, "update", out, astroCoreClient, astroCoreIamClient)
}

func newRemoveOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [ORG_TOKEN_ID]",
		Short: "Remove an Workspace API token's Deployment Role",
		Long:  "Remove an Workspace API token's Deployment Role\n$astro workspace token organization-token remove [ORG_TOKEN_ID] --name [token name].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceTokenFromDeploymentRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Workspace API token you want to remove from a Deployment. If the name contains a space, specify the entire name within quotes \"\" ")
	return cmd
}

func removeWorkspaceTokenFromDeploymentRole(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		orgTokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return workspacetoken.RemoveOrgTokenWorkspaceRole(orgTokenID, orgTokenName, workspaceID, out, astroCoreClient, astroCoreIamClient)
}

func newListOrganizationTokensInWorkspace(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Organization API tokens in a workspace",
		Long:  "List all Organization API tokens in a workspace\n$astro workspace token organization-token list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationTokensInWorkspace(cmd, out)
		},
	}
	return cmd
}

func listOrganizationTokensInWorkspace(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	tokenTypes := []astrocore.ListWorkspaceApiTokensParamsTokenTypes{
		"ORGANIZATION",
	}
	return workspacetoken.ListTokens(astroCoreClient, deploymentID, &tokenTypes, out)
}

func newWorkspaceTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a team from an Astro Workspace",
		Long:    "Remove a team from an Astro Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceTeam(cmd, args, out)
		},
	}
	return cmd
}

func listWorkspaceTeam(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.ListWorkspaceTeams(out, astroCoreClient, "")
}

func removeWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		id = args[0]
	}
	cmd.SilenceUsage = true
	return team.RemoveWorkspaceTeam(id, "", out, astroCoreClient)
}

func newWorkspaceTeamAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [id]",
		Short: "Add a team to an Astro Workspace with a specific role",
		Long:  "Add a team to an Astro Workspace with a specific role\n$astro workspace team add [id] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "The Workspace's unique identifier")
	cmd.Flags().StringVarP(&addWorkspaceRole, "role", "r", "WORKSPACE_MEMBER", "The role for the "+
		"new team. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func addWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	var id string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		id = args[0]
	}
	cmd.SilenceUsage = true
	return team.AddWorkspaceTeam(id, addWorkspaceRole, workspaceID, out, astroCoreClient)
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [id]",
		Aliases: []string{"up"},
		Short:   "Update a the role of a team in an Astro Workspace",
		Long:    "Update the role of a team in an Astro Workspace\n$astro workspace team update [id] --role [" + allowedWorkspaceRoleNames + "].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateWorkspaceRole, "role", "r", "", "The new role for the "+
		"team. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func updateWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	var err error
	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		updateWorkspaceRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}

	cmd.SilenceUsage = true
	return team.UpdateWorkspaceTeamRole(id, updateWorkspaceRole, "", out, astroCoreClient)
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(astroCoreClient, out)
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input

	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	return workspace.Switch(id, astroCoreClient, out)
}

func workspaceCreate(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return workspace.Create(workspaceName, workspaceDescription, enforceCD, out, astroCoreClient)
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	return workspace.Update(id, workspaceName, workspaceDescription, enforceCD, out, astroCoreClient)
}

func workspaceDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	return workspace.Delete(id, out, astroCoreClient)
}

func addWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return user.AddWorkspaceUser(email, addWorkspaceRole, workspaceID, out, astroCoreClient)
}

func listWorkspaceUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return user.ListWorkspaceUsers(out, astroCoreClient, workspaceID)
}

func updateWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		updateWorkspaceRole = input.Text("Enter a user Workspace role(" + allowedWorkspaceRoleNamesProse + ") to update user: ")
	}

	cmd.SilenceUsage = true
	return user.UpdateWorkspaceUserRole(email, updateWorkspaceRole, workspaceID, out, astroCoreClient)
}

func removeWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return user.RemoveWorkspaceUser(email, workspaceID, out, astroCoreClient)
}

func listWorkspaceToken(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return workspacetoken.ListTokens(astroCoreClient, workspaceID, nil, out)
}

func createWorkspaceToken(cmd *cobra.Command, out io.Writer) error {
	if tokenName == "" {
		// no role was provided so ask the user for it
		tokenName = input.Text("Enter a name for the new Workspace API token: ")
	}
	if tokenRole == "" {
		fmt.Println("select a Workspace Role for the new API token:")
		// no role was provided so ask the user for it
		var err error
		tokenRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true

	return workspacetoken.CreateToken(tokenName, tokenDescription, tokenRole, workspaceID, tokenExpiration, cleanTokenOutput, out, astroCoreClient)
}

func updateWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return workspacetoken.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole, workspaceID, out, astroCoreClient, astroCoreIamClient)
}

func rotateWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}
	cmd.SilenceUsage = true
	return workspacetoken.RotateToken(tokenID, name, workspaceID, cleanTokenOutput, forceRotate, out, astroCoreClient, astroCoreIamClient)
}

func deleteWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		tokenID = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return workspacetoken.DeleteToken(tokenID, name, workspaceID, forceDelete, out, astroCoreClient, astroCoreIamClient)
}

func addOrgTokenToWorkspace(cmd *cobra.Command, args []string, out io.Writer) error {
	// if an id was provided in the args we use it
	if len(args) > 0 {
		// make sure the id is lowercase
		orgTokenID = strings.ToLower(args[0])
	}
	if tokenRole == "" {
		fmt.Println("select a Workspace Role for the Organization Token:")
		// no role was provided so ask the user for it
		var err error
		tokenRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true
	return organization.AddOrgTokenToWorkspace(orgTokenID, orgTokenName, tokenRole, workspaceID, out, astroCoreClient, astroCoreIamClient)
}

func coalesceWorkspace() (string, error) {
	wsFlag := workspaceID
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, "failed to get current Workspace")
	}

	if wsFlag != "" {
		return wsFlag, nil
	}

	if wsCfg != "" {
		return wsCfg, nil
	}

	return "", errors.New("no valid Workspace source found")
}

func selectWorkspaceRole() (string, error) {
	tokenRolesMap := map[string]string{}
	tab := &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"#", "ROLE"},
	}
	for i := range validWorkspaceRoles {
		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			validWorkspaceRoles[i],
		}, false)
		tokenRolesMap[strconv.Itoa(index)] = validWorkspaceRoles[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := tokenRolesMap[choice]
	if !ok {
		return "", errInvalidWorkspaceRoleKey
	}
	return selected, nil
}
