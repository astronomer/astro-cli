package astro

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/util"
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
	tokenID                    string
	orgTokenID                 string
	workspaceTokenID           string
	forceRotate                bool
	tokenExpiration            int
	validWorkspaceRoles        []string
	workspaceListOutput        cliout.Format
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
	deleteCmd := newWorkspaceDeleteCmd(out)
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceSwitchCmd(out),
		newWorkspaceCreateCmd(out),
		newWorkspaceUpdateCmd(out),
		deleteCmd,
		newWorkspaceUserRootCmd(out),
		newWorkspaceTokenRootCmd(out),
		newWorkspaceTeamRootCmd(out),
	)
	cmd.AddCommand(cliout.RemovedAlias("de", deleteCmd))
	applyPreferredFlagsIn(cmd)
	return cmd
}

func newWorkspaceListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astro Workspaces in your Organization",
		Long:    "List all Astro Workspaces you have access to in your current Organization. Use 'astro organization switch' to change Organizations.",
		Example: `  astro workspace list
  astro workspace list -o json`,
	}
	addListLimit(cmd, "Workspaces", "", func(cmd *cobra.Command, limit int) error {
		return workspaceList(cmd.Context(), out, cmd.ErrOrStderr(), limit)
	})
	cliout.AddOutputFlag(cmd, &workspaceListOutput)
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "switch",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Astro Workspace",
		Long:    "Switch your active Astro Workspace. Subsequent deployment, user, and team commands run against the selected Workspace unless overridden with --workspace.",
		Example: `
  # Choose a Workspace from a list
  astro workspace switch

  # Switch to a Workspace by its name or ID
  astro workspace switch my-workspace

  # Switch, and print the Workspace as JSON
  astro workspace switch my-workspace -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(out, cmd.ErrOrStderr(), args)
		},
	}, cliout.Optional("WORKSPACE_NAME_OR_ID"))
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
			return workspaceCreate(out, cmd.ErrOrStderr())
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceLifecycleOutput)
	cmd.Flags().StringVarP(&workspaceName, "name", "n", "", "The Workspace's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVar(&workspaceDescription, "description", "", "Description of the Workspace. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVar(&enforceCD, "enforce-cicd", "OFF", "Provide this flag either ON/OFF. ON means deploys to Deployments must use an API Key or Token. This essentially forces Deploys to happen through CI/CD")
	return cmd
}

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Astro Workspace",
		Long:    "Update a Workspace's name, description, or CI/CD enforcement policy. Changing --enforce-cicd affects all Deployments in the Workspace: when enabled, only API token-authenticated deploys are allowed. A setting whose flag is left out keeps its current value. If no Workspace ID is provided, you will be prompted to select one.",
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
	}, cliout.Optional("WORKSPACE_ID"))
	cliout.AddOutputFlag(cmd, &workspaceLifecycleOutput)
	cmd.Flags().StringVarP(&workspaceName, "name", "n", "", "The Workspace's name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVar(&workspaceDescription, "description", "", "Description of the Workspace. If the description contains a space, specify the entire description in quotes \"\"")
	// No default: an update that does not name the setting keeps it.
	cmd.Flags().StringVar(&workspaceUpdateEnforceCD, "enforce-cicd", "", "Provide this flag either ON/OFF. ON means deploys to Deployments must use an API Key or Token. This essentially forces Deploys to happen through CI/CD (default: unchanged)")
	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete an Astro Workspace",
		Long:    "Permanently delete a Workspace. The Workspace must have no Deployments; delete or transfer them first. Deletion also removes all Workspace-scoped API tokens and revokes Workspace-level roles from Organization tokens that had access. This cannot be undone. The command asks for confirmation unless --yes is given.",
		Example: `
  # Delete a Workspace by its ID, after confirming
  astro workspace delete <WORKSPACE_ID>

  # Choose the Workspace to delete from a list
  astro workspace delete

  # Delete a Workspace without being asked, and print the result as JSON
  astro workspace delete <WORKSPACE_ID> --yes -o json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(out, cmd.ErrOrStderr(), args)
		},
	}, cliout.Optional("WORKSPACE_ID"))
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
	addWorkspaceFlag(cmd.PersistentFlags(), &workspaceID, "w", "Workspace whose users you'd like to manage", oneXWorkspaceID)
	cliout.AddOutputFlag(cmd, &workspaceUserOutput)

	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "add",
		Short: "Add a user to an Astro Workspace with a specific role",
		Long:  "Add a user to an Astro Workspace with a specific role. Without an email argument, you choose the user from a list.",
		Example: `
  # Add a user to the current Workspace as a member
  astro workspace user add user@company.com --role WORKSPACE_MEMBER

  # Add a user to another Workspace as an owner
  astro workspace user add user@company.com --role WORKSPACE_OWNER --workspace <WORKSPACE_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceUser(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("EMAIL"))
	cmd.Flags().StringVar(&addWorkspaceRole, "role", "WORKSPACE_MEMBER", "The role for the "+
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
	}
	addListLimit(cmd, "users", "", func(cmd *cobra.Command, limit int) error {
		return listWorkspaceUser(cmd.Context(), out, cmd.ErrOrStderr(), limit)
	})
	return cmd
}

func newWorkspaceUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update the role of a user in an Astro Workspace",
		Long:    "Update the role of a user in an Astro Workspace. Without an email argument you choose the user from a list, and without --role you are prompted for the role.",
		Example: `
  # Make a user a Workspace operator
  astro workspace user update user@company.com --role WORKSPACE_OPERATOR
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceUser(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("EMAIL"))
	cmd.Flags().StringVar(&updateWorkspaceRole, "role", "", "The new role for the "+
		"user. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newWorkspaceUserRemoveCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a user from an Astro Workspace",
		Long:    "Remove a user's role from a Workspace. The user loses access to all Deployments in the Workspace unless they have access through a team. This does not remove them from the Organization.",
		Example: `
  # Remove a user from the current Workspace
  astro workspace user remove user@company.com
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceUser(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("EMAIL"))
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
	deleteCmd := newWorkspaceTokenDeleteCmd(out)
	cmd.AddCommand(
		newWorkspaceTokenListCmd(out),
		newWorkspaceTokenCreateCmd(out),
		newWorkspaceTokenUpdateCmd(out),
		newWorkspaceTokenRotateCmd(out),
		deleteCmd,
		newWorkspaceTokenAddOrgTokenCmd(out),
		newWorkspaceTokenRemoveCmd(out),
		deprecatedTokenGroup(newWorkspaceOrgTokenManageCmd(out), "Organization"),
	)
	cmd.AddCommand(cliout.RemovedAlias("de", deleteCmd))
	addWorkspaceFlag(cmd.PersistentFlags(), &workspaceID, "w", "Workspace whose tokens you'd like to manage", oneXWorkspaceID)
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
	}
	addListLimit(cmd, "API tokens", "", func(cmd *cobra.Command, limit int) error {
		return listWorkspaceToken(cmd.Context(), out, cmd.ErrOrStderr(), limit)
	})
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
  astro workspace team list --workspace <WORKSPACE_ID>
  astro workspace team list -o json`,
	}
	addWorkspaceFlag(cmd.Flags(), &workspaceID, "w", "The Workspace's unique identifier")
	addListLimit(cmd, "teams", "", func(cmd *cobra.Command, limit int) error {
		return listWorkspaceTeam(cmd.Context(), out, cmd.ErrOrStderr(), limit)
	})
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

  # Create an operator token that expires in 30 days
  astro workspace token create --name "CI Token" --role WORKSPACE_OPERATOR \
    --expiration 30
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createWorkspaceToken(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&tokenName, "name", "n", "", "The token's name. If the name contains a space, specify the entire name within quotes \"\" ")
	addInertCleanOutputFlag(cmd)
	cmd.Flags().StringVar(&tokenDescription, "description", "", "Description of the token. If the description contains a space, specify the entire description within quotes \"\"")
	cmd.Flags().StringVar(&tokenRole, "role", "", "The role for the "+
		"token. Possible values are "+allowedWorkspaceRoleNamesProse)
	cmd.Flags().IntVar(&tokenExpiration, "expiration", 0, "Expiration of the token in days, from 1 to 3650. Without it, the token does not expire")
	return cmd
}

func newWorkspaceTokenUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update a Workspace or Organization API token in a Workspace",
		Long:    "Update an API token that has a role in an Astro Workspace. A Workspace token can change its name, description and role. An Organization token can change only its role in this Workspace: its name and description are changed with astro organization token update. Identify the token by its ID or by its current name (--name).",
		Example: `
  # Rename a Workspace token and change its role
  astro workspace token update <TOKEN_ID> --new-name "Updated Token" --role WORKSPACE_OPERATOR

  # Find a token by its name and change its description
  astro workspace token update --name "My Token" --description "Updated description"

  # Change the role an Organization token has in this Workspace
  astro workspace token update --name "My Org Token" --role WORKSPACE_MEMBER
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceToken(cmd.Context(), args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TOKEN_ID"))
	cmd.Flags().StringVar(&name, "name", "", "The current name of the token. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVar(&tokenName, "new-name", "", "The token's new name. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().StringVar(&tokenDescription, "description", "", "Updated description of the token. If the description contains a space, specify the entire description in quotes \"\"")
	cmd.Flags().StringVar(&tokenRole, "role", "", "The new role for the "+
		"token. Possible values are "+allowedWorkspaceRoleNamesProse+". Without it, the token keeps its role")
	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newWorkspaceTokenRotateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "rotate",
		Aliases: []string{"ro"},
		Short:   "Rotate a Workspace API token",
		Long:    "Rotate a Workspace API token. You can only rotate Workspace API tokens. You cannot rotate Organization API tokens with this command",
		Example: `
  # Rotate a token by its ID
  astro workspace token rotate <TOKEN_ID>

  # Rotate a token by its name without confirming
  astro workspace token rotate --name "My Token" --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rotateWorkspaceToken(cmd.Context(), args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TOKEN_ID"))
	addInertCleanOutputFlag(cmd)
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the token to be rotated. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceRotate, "yes", "y", false, "Don't ask for confirmation before rotating the Workspace API token")

	return cmd
}

func newWorkspaceTokenDeleteCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete a Workspace API token",
		Long:    "Delete one of the Workspace's own API tokens. Deleting a token revokes it permanently. An Organization token that has a role in the Workspace is not the Workspace's to delete: astro workspace token remove takes its role off the Workspace, and the token continues to work at other scopes.",
		Example: `
  # Delete a token by its ID
  astro workspace token delete <TOKEN_ID>

  # Delete a token by its name without confirming
  astro workspace token delete --name "My Token" --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteWorkspaceToken(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TOKEN_ID"))
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the token to be deleted. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Don't ask for confirmation before deleting the API token")

	return cmd
}

func newWorkspaceTokenRemoveCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "remove",
		Short: "Remove an Organization API token from a Workspace",
		Long:  "Remove an Organization API token's role in a Workspace. The token continues to work at other scopes. The Workspace's own tokens have no role to remove: astro workspace token delete deletes them. Identify the token by its ID or by its name (--name).",
		Example: `
  # Remove an Organization token from the current Workspace
  astro workspace token remove <ORG_TOKEN_ID>

  # Find the Organization token by its name, without confirming
  astro workspace token remove --name "My Org Token" --yes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeWorkspaceToken(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TOKEN_ID"))
	cmd.Flags().StringVarP(&name, "name", "n", "", "The name of the Organization API token to be removed from the Workspace. If the name contains a space, specify the entire name within quotes \"\" ")
	cmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Don't ask for confirmation before removing the API token from the Workspace")
	return cmd
}

func newWorkspaceTokenAddOrgTokenCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "add",
		Short: "Add an Organization API token to an Astro Workspace",
		Long:  "Add an Organization API token to an Astro Workspace, with a Workspace role. Identify the token by its ID or by its name (--name).",
		Example: `
  # Add an Organization token to the current Workspace as a member
  astro workspace token add <ORG_TOKEN_ID> --role WORKSPACE_MEMBER

  # Find the Organization token by its name
  astro workspace token add --name "My Org Token" --role WORKSPACE_OPERATOR
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenToWorkspace(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("ORG_TOKEN_ID"))
	addTokenNameFlag(cmd, "The name of the Organization API token you want to add to a Workspace. If the name contains a space, specify the entire name within quotes \"\" ", "org-token-name")
	cmd.Flags().StringVar(&tokenRole, "role", "", "The Workspace role to grant to the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newWorkspaceOrgTokenManageCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organization-token",
		Short: "Manage organization tokens in a Workspace",
		Long:  "Manage organization tokens in a Workspace",
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
	cmd := cliout.Args(&cobra.Command{
		Use:   "add",
		Short: "Add an Organization API token to a Workspace",
		Long:  "Add an Organization API token to a Workspace. Identify the token by its ID or by its name (--name).",
		Example: `  # Give an Organization token the member role in the current Workspace
  astro workspace token organization-token add <ORG_TOKEN_ID> --role WORKSPACE_MEMBER

  # Find the Organization token by its name
  astro workspace token organization-token add --name "My Org Token" \
    --role WORKSPACE_OPERATOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addOrgTokenWorkspaceRole(cmd, args, out)
		},
	}, cliout.Optional("ORG_TOKEN_ID"))
	addTokenNameFlag(cmd, "The name of the Organization API token you want to add to a Workspace. If the name contains a space, specify the entire name within quotes \"\" ", "org-token-name")
	cmd.Flags().StringVar(&tokenRole, "role", "", "The Workspace role to grant to the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newUpdateOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "update",
		Short: "Update an Organization API token's Workspace Role",
		Long:  "Update an Organization API token's Workspace Role. Identify the token by its ID or by its name (--name).",
		Example: `  # Make an Organization token an operator in the current Workspace
  astro workspace token organization-token update <ORG_TOKEN_ID> --role WORKSPACE_OPERATOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateOrgTokenWorkspaceRole(cmd, args, out)
		},
	}, cliout.Optional("ORG_TOKEN_ID"))
	addTokenNameFlag(cmd, "The name of the Organization API token you want to update in a Workspace. If the name contains a space, specify the entire name within quotes \"\" ", "org-token-name")
	cmd.Flags().StringVar(&tokenRole, "role", "", "The Workspace role to update the "+
		"Organization API token. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func newRemoveOrganizationTokenWorkspaceRole(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove an Organization API token's Workspace Role",
		Long:    "Remove an Organization API token's Workspace Role. Identify the token by its ID or by its name (--name).",
		Example: `  # Remove an Organization token from the current Workspace
  astro workspace token organization-token remove <ORG_TOKEN_ID>

  # Find the Organization token by its name
  astro workspace token organization-token remove --name "My Org Token"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeOrganizationTokenWorkspaceRole(cmd.Context(), args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("ORG_TOKEN_ID"))
	addTokenNameFlag(cmd, "The name of the Organization API token you want to remove from a Workspace. If the name contains a space, specify the entire name within quotes \"\" ", "org-token-name")
	return cmd
}

func newListOrganizationTokensInWorkspace(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Organization API tokens in a Workspace",
		Long:  "List all Organization API tokens in a Workspace",
		Example: `  # List the Organization tokens in the current Workspace
  astro workspace token organization-token list

  # List the Organization tokens in another Workspace
  astro workspace token organization-token list --workspace <WORKSPACE_ID>`,
	}
	addListLimit(cmd, "Organization API tokens", "The scope filter runs locally, so every page is read before the limit applies", func(cmd *cobra.Command, limit int) error {
		return listOrganizationTokensInWorkspace(cmd.Context(), out, cmd.ErrOrStderr(), limit)
	})
	return cmd
}

func newWorkspaceTeamRemoveCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "remove",
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
			return removeWorkspaceTeam(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TEAM_ID"))
	addWorkspaceFlag(cmd.Flags(), &workspaceID, "w", "The Workspace's unique identifier")
	return cmd
}

func listWorkspaceTeam(ctx context.Context, out, errOut io.Writer, limit int) error {
	return team.ListWorkspaceTeamsWithFormat(ctx, astroV1Client, workspaceID, limit, cliout.Renderer{Format: workspaceTeamOutput, Out: out, Err: errOut})
}

func removeWorkspaceTeam(cmd *cobra.Command, args []string, out, errOut io.Writer) error {
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	r, err := team.RemoveWorkspaceTeam(cmd.Context(), id, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(workspaceTeamOutput, out, errOut, &r, fmt.Sprintf("Removed team %s from Workspace %s", r.Name, r.WorkspaceID))
}

func newWorkspaceTeamAddCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "add",
		Short: "Add a team to an Astro Workspace with a specific role",
		Long:  "Add a team to an Astro Workspace with a specific role. Without a team ID, you choose the team from a list.",
		Example: `
  # Add a team to the current Workspace as a member
  astro workspace team add <TEAM_ID> --role WORKSPACE_MEMBER

  # Add a team to another Workspace as an operator
  astro workspace team add <TEAM_ID> --role WORKSPACE_OPERATOR --workspace <WORKSPACE_ID>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceTeam(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TEAM_ID"))
	addWorkspaceFlag(cmd.Flags(), &workspaceID, "w", "The Workspace's unique identifier", oneXWorkspaceID)
	cmd.Flags().StringVar(&addWorkspaceRole, "role", "WORKSPACE_MEMBER", "The role for the "+
		"new team. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func addWorkspaceTeam(cmd *cobra.Command, args []string, out, errOut io.Writer) error {
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	t, err := team.AddWorkspaceTeam(cmd.Context(), id, addWorkspaceRole, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(workspaceTeamOutput, out, errOut, &t, fmt.Sprintf("Added team %s to the Workspace with role %s", teamLabel(&t), t.WorkspaceRole))
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "update",
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
			return updateWorkspaceTeam(cmd, args, out, cmd.ErrOrStderr())
		},
	}, cliout.Optional("TEAM_ID"))
	addWorkspaceFlag(cmd.Flags(), &workspaceID, "w", "The Workspace's unique identifier")
	cmd.Flags().StringVar(&updateWorkspaceRole, "role", "", "The new role for the "+
		"team. Possible values are "+allowedWorkspaceRoleNamesProse)
	return cmd
}

func updateWorkspaceTeam(cmd *cobra.Command, args []string, out, errOut io.Writer) error {
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}
	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		var err error
		updateWorkspaceRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}

	if id == "" {
		if err := mayPick("a team", teamIDAnswer); err != nil {
			return err
		}
	}
	t, err := team.UpdateWorkspaceTeamRole(cmd.Context(), id, updateWorkspaceRole, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(workspaceTeamOutput, out, errOut, &t, fmt.Sprintf("Updated Workspace team %s to %s", teamLabel(&t), t.WorkspaceRole))
}

func workspaceList(ctx context.Context, out, errOut io.Writer, limit int) error {
	return workspace.ListWithFormat(ctx, astroV1Client, limit, cliout.Renderer{Format: workspaceListOutput, Out: out, Err: errOut})
}

func workspaceSwitch(out, errOut io.Writer, args []string) error {
	workspaceNameOrID := ""

	if len(args) == 1 {
		workspaceNameOrID = args[0]
	}
	ws, err := workspace.SwitchTo(workspaceNameOrID, astroV1Client, os.Stderr)
	if err != nil {
		return err
	}
	return emitWorkspaceSwitch(cliout.Renderer{Format: workspaceLifecycleOutput, Out: out, Err: errOut}, ws)
}

func workspaceCreate(out, errOut io.Writer) error {
	ws, err := workspace.Create(workspaceName, workspaceDescription, enforceCD, astroV1Client)
	if err != nil {
		return err
	}
	return emitWorkspace(cliout.Renderer{Format: workspaceLifecycleOutput, Out: out, Err: errOut}, ws, fmt.Sprintf("Created Workspace %s", ws.Name))
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	// Left out, the setting stays as it is: Update keeps the Workspace's own
	// for an empty value.
	enforce := ""
	if cmd.Flags().Changed("enforce-cicd") {
		enforce = workspaceUpdateEnforceCD
		if enforce == "" {
			return workspace.ErrWrongEnforceInput
		}
	}
	res, err := workspace.Update(id, workspaceName, workspaceDescription, enforce, os.Stderr, astroV1Client)
	if err != nil {
		return err
	}
	// The line names the Workspace as it was called before the update.
	return emitWorkspace(cliout.Renderer{Format: workspaceLifecycleOutput, Out: out, Err: cmd.ErrOrStderr()}, &res.Workspace, fmt.Sprintf("Updated Workspace %s", res.PreviousName))
}

func workspaceDelete(out, errOut io.Writer, args []string) error {
	id := ""

	if len(args) == 1 {
		id = args[0]
	}
	removal, err := workspace.Delete(id, workspaceDeleteYes, os.Stderr, astroV1Client)
	if err != nil {
		return err
	}
	return emitWorkspaceRemoval(cliout.Renderer{Format: workspaceLifecycleOutput, Out: out, Err: errOut}, removal)
}

func addWorkspaceUser(cmd *cobra.Command, args []string, out, errOut io.Writer) error {
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.AddWorkspaceUser(cmd.Context(), email, addWorkspaceRole, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(workspaceUserOutput, out, errOut, &u, fmt.Sprintf("Added user %s to the Workspace with role %s", u.Email, u.WorkspaceRole))
}

func listWorkspaceUser(ctx context.Context, out, errOut io.Writer, limit int) error {
	return user.ListWorkspaceUsersWithFormat(ctx, astroV1Client, workspaceID, limit, cliout.Renderer{Format: workspaceUserOutput, Out: out, Err: errOut})
}

func updateWorkspaceUser(cmd *cobra.Command, args []string, out, errOut io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}

	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a user Workspace role("+allowedWorkspaceRoleNamesProse+") to update user: ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		updateWorkspaceRole = answer
	}

	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	u, err := user.UpdateWorkspaceUserRole(cmd.Context(), email, updateWorkspaceRole, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(workspaceUserOutput, out, errOut, &u, fmt.Sprintf("Updated Workspace user %s to %s", u.Email, u.WorkspaceRole))
}

func removeWorkspaceUser(cmd *cobra.Command, args []string, out, errOut io.Writer) error {
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	if email == "" {
		if err := mayPick("a user", userEmailAnswer); err != nil {
			return err
		}
	}
	r, err := user.RemoveWorkspaceUser(cmd.Context(), email, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderLines(workspaceUserOutput, out, errOut, &r, fmt.Sprintf("Removed user %s from the Workspace", r.Email))
}

// workspaceTarget is the run's Workspace for cmd, from the one resolver
// (astrosession.Resolve): flag (--workspace, "" when the command has none or
// it was not given), the project when cmd follows it, the token's claim, the
// saved login. noSaved is what a saved login with no Workspace says (nil for
// astrosession.ErrNoWorkspace). extra is what else the command accepts in place of a Workspace
// (astrosession.ByID), for the advice a missing one gets.
func workspaceTarget(cmd *cobra.Command, flag string, extra astrosession.WorkspaceAdvice, noSaved error) astrosession.Target {
	req := astrosession.Request{Workspace: flag, Advice: astrosession.ByFlag | extra, NoSaved: noSaved, LoginError: true}
	if f, ok := projectFollowOf(cmd); ok {
		req.Project = f.workspace
		req.Advice |= astrosession.ByProject
	}
	t := astrosession.Resolve(req)
	// A Workspace the token supplied is said so, now that the command uses it.
	noteTokenWorkspace(cmd, t)
	return t
}

// requiredWorkspace is workspaceTarget's Workspace, or the error for its
// absence, so a command that asks a question or calls the API takes its
// Workspace first and nothing is asked on the way to the error.
func requiredWorkspace(cmd *cobra.Command, flag string) (string, error) {
	return requiredFrom(workspaceTarget(cmd, flag, 0, nil))
}

func requiredFrom(t astrosession.Target) (string, error) { //nolint:gocritic // a small value type
	if err := t.Require(); err != nil {
		return "", err
	}
	return t.Workspace, nil
}

// coalesceWorkspace is the Workspace the astro env and astro deployment
// commands act on: --workspace, else the project's, else the token's, else the
// saved login's.
func coalesceWorkspace(cmd *cobra.Command) (string, error) {
	return coalesceWorkspaceOr(cmd, 0)
}

// coalesceWorkspaceOr is coalesceWorkspace for a command that also takes
// something in place of a Workspace (extra), which its advice then names.
func coalesceWorkspaceOr(cmd *cobra.Command, extra astrosession.WorkspaceAdvice) (string, error) {
	_, ws, err := coalesceTargetOr(cmd, extra)
	return ws, err
}

// coalesceTarget is coalesceWorkspace with the Target it came from, for a
// command that tells a lookup where its Workspace came from.
func coalesceTarget(cmd *cobra.Command) (astrosession.Target, string, error) {
	return coalesceTargetOr(cmd, 0)
}

// coalesceTargetOr is coalesceTarget for a command that also takes something in
// place of a Workspace (extra).
func coalesceTargetOr(cmd *cobra.Command, extra astrosession.WorkspaceAdvice) (astrosession.Target, string, error) {
	t := workspaceTarget(cmd, workspaceID, extra, workspace.ErrCurrentNotSet)
	ws, err := coalesceFrom(t)
	return t, ws, err
}

func coalesceFrom(t astrosession.Target) (string, error) { //nolint:gocritic // a small value type
	ws, err := requiredFrom(t)
	if errors.Is(err, astrosession.ErrOrgTokenNoWorkspace) {
		return "", err
	}
	if err != nil {
		return "", wrapWorkspaceErr(err, "could not get the current Workspace")
	}
	return ws, nil
}

// isWorkspaceRefusal reports whether err is a Workspace the run's API token
// is not for (astrosession.WorkspaceDisagreeError). It says in full what to
// change, so what found it does not wrap it in what it was looking for.
func isWorkspaceRefusal(err error) bool {
	var disagree *astrosession.WorkspaceDisagreeError
	return errors.As(err, &disagree)
}

// wrapWorkspaceErr is err, saying msg first, unless err is a refusal
// (isWorkspaceRefusal), which stands as it is.
func wrapWorkspaceErr(err error, msg string) error {
	if isWorkspaceRefusal(err) {
		return err
	}
	return errors.Wrap(err, msg)
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
	i, err := list.Pick(os.Stderr, os.Stdin)
	if err != nil {
		return "", err
	}
	return validWorkspaceRoles[i], nil
}

// argOrID is the Deployment id a command was given: its argument, else
// --deployment-id (or whatever --deployment resolved to).
func argOrID(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return deploymentID
}

// deploymentWorkspace is the Workspace a command that acts on one Deployment
// looks it up in, and the client to look it up with. A Deployment given by id
// is identified by it, so a run that has no Workspace goes on without one: an
// Organization token selects none, and a saved login may have none current.
// The Deployment is fetched by id, and its own Workspace is the one the rest
// of the command works in. The client returned is then handed that fetched
// Deployment (deployment.WithFetched), so the command's own lookup of it asks
// the API for nothing more: the id is one GET. Given a Workspace the id is
// looked for in it, as it always was, with no extra call, and the client is
// the run's own. A name, or a pick, still needs a Workspace, and fails here as
// before, ahead of any question or API call. A failure to find the Workspace
// is worded with msg ("" for none).
func deploymentWorkspace(cmd *cobra.Command, id, msg string) (ws string, client astrov1.APIClient, err error) {
	t := workspaceTarget(cmd, workspaceID, 0, workspace.ErrCurrentNotSet)
	// Only an id identifies a Deployment with no Workspace; a name that is not
	// one goes on to say there is no Workspace to look it up in.
	if !t.Found() && util.IsCUID(id) {
		d, err := deployment.GetDeploymentByID(cmd.Context(), "", id, astroV1Client)
		if err != nil {
			// An interrupt is not a failed lookup.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", astroV1Client, err
			}
			return "", astroV1Client, fmt.Errorf("no Workspace is selected, so Deployment %s was fetched by its id, which failed (pass --workspace to look it up in a Workspace instead): %w", id, err)
		}
		return d.WorkspaceId, deployment.WithFetched(astroV1Client, d), nil
	}
	ws, err = coalesceFrom(t)
	if err != nil && msg != "" {
		return "", astroV1Client, wrapWorkspaceErr(err, msg)
	}
	if ws == "" {
		return ws, astroV1Client, err
	}
	// The client knows where the Workspace came from, so a lookup in it that
	// fails says so (deployment.WithWorkspaceSource).
	return ws, deployment.WithWorkspaceSource(astroV1Client, ws, t.WorkspaceSource), err
}
