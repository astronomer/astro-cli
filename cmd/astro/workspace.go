package astro

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	errInvalidWorkspaceRoleKey = errors.New("invalid workspace role selection")
	workspaceID                string
	addWorkspaceRole           string
	updateWorkspaceRole        string
	workspaceName              string
	workspaceDescription       string
	enforceCD                  string
	workspaceUpdateEnforceCD   string
	workspaceDeleteYes         bool
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
	workspaceListOutput        string
)

const (
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
	applyPreferredFlagsIn(cmd)
	return cmd
}

func newWorkspaceListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astro Workspaces in your organization",
		Long:    "List all Astro Workspaces you have access to in your current Organization. Use 'astro organization switch' to change Organizations.",
		Example: `  astro workspace list
  astro workspace list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, out)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceListOutput)
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [WORKSPACE_NAME_OR_ID]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Astro Workspace",
		Long:    "Switch your active Astro Workspace. Subsequent deployment, user, and team commands run against the selected Workspace unless overridden with --workspace.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  # Choose a Workspace from a list
  astro workspace switch

  # Switch to a Workspace by its name or ID
  astro workspace switch my-workspace

  # Switch, and print the Workspace as JSON
  astro workspace switch my-workspace -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, out, args)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceLifecycleOutput)
	return cmd
}

func newWorkspaceCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Astro Workspace",
		Long:    "Create a new Workspace in your current Organization. Workspaces group Deployments and control user access independently. Enable --enforce-cicd to require that all deploys to Deployments in this Workspace use an API token, blocking manual deploys from the CLI or UI.",
		Example: `
  # Create a Workspace with a name and a description
  astro workspace create --name "My Workspace" --description "Production pipelines"

  # Create a Workspace that only accepts deploys made with an API token
  astro workspace create --name "My Workspace" --enforce-cicd ON

  # Create a Workspace, and print it as JSON
  astro workspace create --name "My Workspace" -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, out)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceLifecycleOutput)
	cmd.Flags().StringVarP(&workspaceName, "name", "n", "", "The Workspace's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&workspaceDescription, "description", "d", "", "Description of the Workspace. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&enforceCD, "enforce-cicd", "e", "OFF", "Provide this flag either ON/OFF. ON means deploys to deployments must use an API Key or Token. This essentially forces Deploys to happen through CI/CD")
	return cmd
}

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [WORKSPACE_ID]",
		Aliases: []string{"up"},
		Short:   "Update an Astro Workspace",
		Long:    "Update a Workspace's name, description, or CI/CD enforcement policy. Changing --enforce-cicd affects all Deployments in the Workspace: when enabled, only API token-authenticated deploys are allowed. A setting whose flag is left out keeps its current value. If no Workspace ID is provided, you will be prompted to select one.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  # Rename a Workspace
  astro workspace update <WORKSPACE_ID> --name "New Name"

  # Change the description and require deploys made with an API token
  astro workspace update <WORKSPACE_ID> --description "Updated description" --enforce-cicd ON

  # Rename a Workspace, and print it as JSON
  astro workspace update <WORKSPACE_ID> --name "New Name" -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, out, args)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceLifecycleOutput)
	cmd.Flags().StringVarP(&workspaceName, "name", "n", "", "The Workspace's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&workspaceDescription, "description", "d", "", "Description of the Workspace. If the description contains a space, specify the entire description in quotes \"\"")
	// No default: an update that does not name the setting keeps it.
	cmd.Flags().StringVarP(&workspaceUpdateEnforceCD, "enforce-cicd", "e", "", "Provide this flag either ON/OFF. ON means deploys to deployments must use an API Key or Token. This essentially forces Deploys to happen through CI/CD (default: unchanged)")
	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [WORKSPACE_ID]",
		Aliases: []string{"de"},
		Short:   "Delete an Astro Workspace",
		Long:    "Permanently delete a Workspace. The Workspace must have zero Deployments — delete or transfer all Deployments first. Deletion also removes all Workspace-scoped API tokens and revokes Workspace-level roles from Organization tokens that had access. This action cannot be undone, so the command asks for confirmation first unless --yes is given.",
		Args:    cobra.MaximumNArgs(1),
		Example: `
  # Delete a Workspace by its ID, after confirming
  astro workspace delete <WORKSPACE_ID>

  # Choose the Workspace to delete from a list
  astro workspace delete

  # Delete a Workspace without being asked, and print the result as JSON
  astro workspace delete <WORKSPACE_ID> --yes -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, out, args)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceLifecycleOutput)
	cmd.Flags().BoolVarP(&workspaceDeleteYes, "yes", "y", false, "Don't ask for confirmation before deleting the Workspace")
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
	addWorkspaceFlag(cmd.PersistentFlags(), "", "Workspace whose users you'd like to manage")
	cliout.AddOutputFlag(cmd, &workspaceUserOutput)

	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [EMAIL]",
		Short: "Add a user to an Astro Workspace with a specific role",
		Long:  "Add a user to an Astro Workspace with a specific role. Without an email argument, you choose the user from a list.",
		Example: `
  # Add a user to the current Workspace as a member
  astro workspace user add user@company.com --role WORKSPACE_MEMBER

  # Add a user to another Workspace as an owner
  astro workspace user add user@company.com --role WORKSPACE_OWNER --workspace <WORKSPACE_ID>
`,
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
		Long:    "List all users and their roles in a Workspace.",
		Example: `  astro workspace user list
  astro workspace user list --workspace <WORKSPACE_ID>
  astro workspace user list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceUser(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [EMAIL]",
		Aliases: []string{"up"},
		Short:   "Update the role of a user in an Astro Workspace",
		Long:    "Update the role of a user in an Astro Workspace. Without an email argument you choose the user from a list, and without --role you are prompted for the role.",
		Example: `
  # Make a user a Workspace operator
  astro workspace user update user@company.com --role WORKSPACE_OPERATOR
`,
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
		Use:     "remove [EMAIL]",
		Aliases: []string{"rm"},
		Short:   "Remove a user from an Astro Workspace",
		Long:    "Remove a user's role from a Workspace. The user loses access to all Deployments in the Workspace unless they have access through a team. This does not remove them from the Organization.",
		Example: `
  # Remove a user from the current Workspace
  astro workspace user remove user@company.com
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceUser(cmd, args, out)
		},
	}
	return cmd
}

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
	addWorkspaceFlag(cmd.PersistentFlags(), "", "Workspace whose tokens you'd like to manage")
	cliout.AddOutputFlag(cmd, &workspaceTokenOutput)
	return cmd
}

func newWorkspaceTokenListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the API tokens in an Astro Workspace",
		Long:    "List all API tokens with a role in a Workspace, including both Workspace-scoped tokens and Organization-scoped tokens that have been granted a Workspace role.",
		Example: `
  # List the tokens in the current Workspace
  astro workspace token list

  # List the tokens in another Workspace
  astro workspace token list --workspace <WORKSPACE_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceToken(cmd, out)
		},
	}
	return cmd
}

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
	cliout.AddOutputFlag(cmd, &workspaceTeamOutput)
	return cmd
}

func newWorkspaceTeamListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all the teams in an Astro Workspace",
		Long:    "List all teams and their assigned roles in a Workspace.",
		Example: `  astro workspace team list
  astro workspace team list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWorkspaceTeam(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceTokenCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an API token in an Astro Workspace",
		Long:    "Create an API token in an Astro Workspace. Without --name or --role, you are prompted for them.",
		Example: `
  # Create a member token
  astro workspace token create --name "My Token" --role WORKSPACE_MEMBER

  # Create an operator token that expires in 30 days, printing only the token
  astro workspace token create --name "CI Token" --role WORKSPACE_OPERATOR \
    --expiration 30 --clean-output
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createWorkspaceToken(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&tokenName, "name", "n", "", "The token's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Description of the token. If the description contains a space, specify the entire description within quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The role for the "+
		"token. Possible values are "+allowedWorkspaceRoleNamesProse)
	cmd.Flags().IntVarP(&tokenExpiration, "expiration", "e", 0, "Expiration of the token in days, from 1 to 3650. Without it, the token does not expire")
	return cmd
}

func newWorkspaceTokenUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TOKEN_ID]",
		Aliases: []string{"up"},
		Short:   "Update a Workspace or Organization API token",
		Long:    "Update a Workspace or Organization API token that has a role in an Astro Workspace. Identify the token by its ID or by its current name (--name).",
		Example: `
  # Rename a token and change its role
  astro workspace token update <TOKEN_ID> --new-name "Updated Token" --role WORKSPACE_OPERATOR

  # Find a token by its name and change its description
  astro workspace token update --name "My Token" --description "Updated description"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The current name of the token. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenName, "new-name", "n", "", "The token's new name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenDescription, "description", "d", "", "Updated description of the token. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The new role for the "+
		"token. Possible values are "+allowedWorkspaceRoleNamesProse+". Without it, the token keeps its role")
	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newWorkspaceTokenRotateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate [TOKEN_ID]",
		Aliases: []string{"ro"},
		Short:   "Rotate a Workspace API token",
		Long:    "Rotate a Workspace API token. You can only rotate Workspace API tokens. You cannot rotate Organization API tokens with this command",
		Example: `
  # Rotate a token by its ID
  astro workspace token rotate <TOKEN_ID>

  # Rotate a token by its name without confirming, printing only the new token
  astro workspace token rotate --name "My Token" --yes --clean-output
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rotateWorkspaceToken(cmd, args, out)
		},
	}
	cmd.Flags().BoolVarP(&cleanTokenOutput, "clean-output", "c", false, "Print only the token as output. For use of the command in scripts")
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be rotated. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceRotate, "yes", "y", false, "Don't ask for confirmation before rotating the Workspace API token")

	return cmd
}

func newWorkspaceTokenDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [TOKEN_ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Workspace API token or remove an Organization API token from a Workspace",
		Long:    "Delete a Workspace API token or remove an Organization token's Workspace role. Deleting a Workspace token revokes it permanently. Removing an Organization token only revokes its Workspace role — the token continues to work at other scopes.",
		Example: `
  # Delete a token by its ID
  astro workspace token delete <TOKEN_ID>

  # Delete a token by its name without confirming
  astro workspace token delete --name "My Token" --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteWorkspaceToken(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "The name of the token to be deleted. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Don't ask for confirmation before deleting or removing the API token")

	return cmd
}

func newWorkspaceTokenAddOrgTokenCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [ORG_TOKEN_ID]",
		Short: "Add an Organization API token to an Astro Workspace",
		Long:  "Add an Organization API token to an Astro Workspace. Identify the token by its ID or by its name (--org-token-name).",
		Example: `
  # Add an Organization token to the current Workspace as a member
  astro workspace token add <ORG_TOKEN_ID> --role WORKSPACE_MEMBER

  # Find the Organization token by its name
  astro workspace token add --org-token-name "My Org Token" --role WORKSPACE_OPERATOR
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenToWorkspace(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to add to a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Workspace role to grant to the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

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
		Long:  "Add an Organization API token to a Workspace. Identify the token by its ID or by its name (--org-token-name).",
		Example: `  # Give an Organization token the member role in the current Workspace
  astro workspace token organization-token add <ORG_TOKEN_ID> --role WORKSPACE_MEMBER

  # Find the Organization token by its name
  astro workspace token organization-token add --org-token-name "My Org Token" \
    --role WORKSPACE_OPERATOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenWorkspaceRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to add to a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Workspace role to grant to the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newUpdateOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [ORG_TOKEN_ID]",
		Short: "Update an Organization API token's Workspace Role",
		Long:  "Update an Organization API token's Workspace Role. Identify the token by its ID or by its name (--org-token-name).",
		Example: `  # Make an Organization token an operator in the current Workspace
  astro workspace token organization-token update <ORG_TOKEN_ID> --role WORKSPACE_OPERATOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateOrgTokenWorkspaceRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to update in a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVarP(&tokenRole, "role", "r", "", "The Workspace role to update the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newRemoveOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [ORG_TOKEN_ID]",
		Short: "Remove an Organization API token's Workspace Role",
		Long:  "Remove an Organization API token's Workspace Role. Identify the token by its ID or by its name (--org-token-name).",
		Example: `  # Remove an Organization token from the current Workspace
  astro workspace token organization-token remove <ORG_TOKEN_ID>

  # Find the Organization token by its name
  astro workspace token organization-token remove --org-token-name "My Org Token"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeOrganizationTokenWorkspaceRole(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&orgTokenName, "org-token-name", "n", "", "The name of the Organization API token you want to remove from a Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	return cmd
}

func newListOrganizationTokensInWorkspace(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Organization API tokens in a workspace",
		Long:  "List all Organization API tokens in a workspace",
		Example: `  # List the Organization tokens in the current Workspace
  astro workspace token organization-token list

  # List the Organization tokens in another Workspace
  astro workspace token organization-token list --workspace <WORKSPACE_ID>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listOrganizationTokensInWorkspace(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [TEAM_ID]",
		Aliases: []string{"rm"},
		Short:   "Remove a team from an Astro Workspace",
		Long:    "Remove a team from an Astro Workspace",
		Example: `
  # Remove a team from the current Workspace
  astro workspace team remove <TEAM_ID>

  # Remove a team from another Workspace
  astro workspace team remove <TEAM_ID> --workspace <WORKSPACE_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "The Workspace's unique identifier")
	addWorkspaceFlag(cmd.Flags(), "w", "The Workspace's unique identifier")
	return cmd
}

func listWorkspaceTeam(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceTeamOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return team.ListWorkspaceTeamsWithFormat(astroV1Client, "", cliout.Renderer{Format: format, Out: out})
}

func removeWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceTeamOutput)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	r, err := team.RemoveWorkspaceTeam(id, workspaceID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &r, fmt.Sprintf("Astro Team %s was successfully removed from workspace %s", r.Name, r.WorkspaceID))
}

func newWorkspaceTeamAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [TEAM_ID]",
		Short: "Add a team to an Astro Workspace with a specific role",
		Long:  "Add a team to an Astro Workspace with a specific role. Without a team ID, you choose the team from a list.",
		Example: `
  # Add a team to the current Workspace as a member
  astro workspace team add <TEAM_ID> --role WORKSPACE_MEMBER

  # Add a team to another Workspace as an operator
  astro workspace team add <TEAM_ID> --role WORKSPACE_OPERATOR --workspace <WORKSPACE_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "The Workspace's unique identifier")
	addWorkspaceFlag(cmd.Flags(), "w", "The Workspace's unique identifier")
	cmd.Flags().StringVarP(&addWorkspaceRole, "role", "r", "WORKSPACE_MEMBER", "The role for the "+
		"new team. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func addWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceTeamOutput)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	t, err := team.AddWorkspaceTeam(id, addWorkspaceRole, workspaceID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &t, fmt.Sprintf("The team %s was successfully added to the workspace with the role %s", teamLabel(&t), t.WorkspaceRole))
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [TEAM_ID]",
		Aliases: []string{"up"},
		Short:   "Update the role of a team in an Astro Workspace",
		Long:    "Update the role of a team in an Astro Workspace. Without a team ID, you choose the team from a list.",
		Example: `
  # Make a team a Workspace operator
  astro workspace team update <TEAM_ID> --role WORKSPACE_OPERATOR

  # Make a team the owner of another Workspace
  astro workspace team update <TEAM_ID> --role WORKSPACE_OWNER --workspace <WORKSPACE_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "The Workspace's unique identifier")
	addWorkspaceFlag(cmd.Flags(), "w", "The Workspace's unique identifier")
	cmd.Flags().StringVarP(&updateWorkspaceRole, "role", "r", "", "The new role for the "+
		"team. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func updateWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceTeamOutput)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		updateWorkspaceRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}

	cmd.SilenceUsage = true
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	t, err := team.UpdateWorkspaceTeamRole(id, updateWorkspaceRole, workspaceID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &t, fmt.Sprintf("The workspace team %s role was successfully updated to %s", teamLabel(&t), t.WorkspaceRole))
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceListOutput)
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.ListWithFormat(astroV1Client, cliout.Renderer{Format: format, Out: out})
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	format, err := cliout.ParseFormat(workspaceLifecycleOutput)
	if err != nil {
		return err
	}

	workspaceNameOrID := ""

	if len(args) == 1 {
		workspaceNameOrID = args[0]
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	ws, err := workspace.SwitchTo(workspaceNameOrID, astroV1Client, questionsTo(cmd, format, out))
	if err != nil {
		return err
	}
	return emitWorkspaceSwitch(cliout.Renderer{Format: format, Out: out}, ws)
}

func workspaceCreate(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceLifecycleOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	ws, err := workspace.Create(workspaceName, workspaceDescription, enforceCD, astroV1Client)
	if err != nil {
		return err
	}
	return emitWorkspace(cliout.Renderer{Format: format, Out: out}, ws, fmt.Sprintf("Astro Workspace %s was successfully created", ws.Name))
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	format, err := cliout.ParseFormat(workspaceLifecycleOutput)
	if err != nil {
		return err
	}
	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	// Left out, the setting stays as it is: Update keeps the Workspace's own
	// for an empty value.
	enforce := ""
	if cmd.Flags().Changed("enforce-cicd") {
		enforce = workspaceUpdateEnforceCD
		if enforce == "" {
			return workspace.ErrWrongEnforceInput
		}
	}
	res, err := workspace.Update(id, workspaceName, workspaceDescription, enforce, questionsTo(cmd, format, out), astroV1Client)
	if err != nil {
		return err
	}
	// The line names the Workspace as it was called before the update.
	return emitWorkspace(cliout.Renderer{Format: format, Out: out}, &res.Workspace, fmt.Sprintf("Astro Workspace %s was successfully updated", res.PreviousName))
}

func workspaceDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	format, err := cliout.ParseFormat(workspaceLifecycleOutput)
	if err != nil {
		return err
	}
	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	cmd.SilenceUsage = true
	removal, err := workspace.Delete(id, workspaceDeleteYes, questionsTo(cmd, format, out), astroV1Client)
	if err != nil || removal == nil {
		// nil, nil is a declined question, which has said so.
		return err
	}
	return emitWorkspaceRemoval(cliout.Renderer{Format: format, Out: out}, removal)
}

func addWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.AddWorkspaceUser(email, addWorkspaceRole, workspaceID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &u, fmt.Sprintf("The user %s was successfully added to the workspace with the role %s", u.Email, u.WorkspaceRole))
}

func listWorkspaceUser(cmd *cobra.Command, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceUserOutput)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return user.ListWorkspaceUsersWithFormat(astroV1Client, workspaceID, cliout.Renderer{Format: format, Out: out})
}

func updateWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a user Workspace role("+allowedWorkspaceRoleNamesProse+") to update user: ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		updateWorkspaceRole = answer
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.UpdateWorkspaceUserRole(email, updateWorkspaceRole, workspaceID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &u, fmt.Sprintf("The workspace user %s role was successfully updated to %s", u.Email, u.WorkspaceRole))
}

func removeWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := cliout.ParseFormat(workspaceUserOutput)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	r, err := user.RemoveWorkspaceUser(email, workspaceID, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(format, out, &r, fmt.Sprintf("The user %s was successfully removed from the workspace", r.Email))
}

func coalesceWorkspace() (string, error) {
	// An explicit --workspace-id flag is authoritative and must win before we
	// require a current-workspace context. Org-scoped API tokens leave that
	// context empty (no workspaceId claim), so consulting it first would error
	// out even when the caller supplied a valid workspace explicitly.
	if wsFlag := workspaceID; wsFlag != "" {
		return wsFlag, nil
	}
	if projectWorkspaceID != "" {
		return projectWorkspaceID, nil
	}

	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, "failed to get current Workspace")
	}

	if wsCfg != "" {
		return wsCfg, nil
	}

	return "", errors.New("no valid Workspace source found")
}

func selectWorkspaceRole() (string, error) {
	list := picker.List{
		Header:  []string{"ROLE"},
		Ask:     []input.Option{input.About("a role"), input.AnsweredBy("--role")},
		Invalid: errInvalidWorkspaceRoleKey,
	}
	for _, role := range validWorkspaceRoles {
		list.AddRow(false, role)
	}
	i, err := list.Pick(os.Stdout, os.Stdin)
	if err != nil {
		return "", err
	}
	return validWorkspaceRoles[i], nil
}
