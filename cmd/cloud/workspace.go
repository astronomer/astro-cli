package cloud

import (
	"io"
	"strings"

	"github.com/astronomer/astro-cli/cloud/team"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/pkg/input"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceID          string
	addWorkspaceRole     string
	updateWorkspaceRole  string
	workspaceName        string
	workspaceDescription string
	enforceCD            string
)

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
	cmd.Flags().StringVarP(&workspaceDescription, "description", "d", "", "Description of the Workspace. If the description contains a space, specify the entire workspace in quotes \"\"")
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
	cmd.Flags().StringVarP(&workspaceDescription, "description", "d", "", "Description of the Workspace. If the description contains a space, specify the entire workspace in quotes \"\"")
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
		Aliases: []string{"us"},
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
	return cmd
}

func newWorkspaceUserAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [email]",
		Short: "Add a user to an Astro Workspace with a specific role",
		Long: "Add a user to an Astro Workspace with a specific role\n$astro workspace user add [email] --role [WORKSPACE_MEMBER, " +
			"WORKSPACE_OPERATOR, WORKSPACE_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&addWorkspaceRole, "role", "r", "WORKSPACE_MEMBER", "The role for the "+
		"new user. Possible values are WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
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
		Long: "Update the role of a user in an Astro Workspace\n$astro workspace user update [email] --role [WORKSPACE_MEMBER, " +
			"WORKSPACE_OPERATOR, WORKSPACE_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceUser(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateWorkspaceRole, "role", "r", "", "The new role for the "+
		"user. Possible values are WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
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

func newWorkspaceTeamRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"te"},
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

func listWorkspaceTeam(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return team.ListWorkspaceTeams(out, astroCoreClient, "")
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
		Long: "Add a team to an Astro Workspace with a specific role\n$astro workspace team add [id] --role [WORKSPACE_MEMBER, " +
			"WORKSPACE_OPERATOR, WORKSPACE_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&addWorkspaceRole, "role", "r", "WORKSPACE_MEMBER", "The role for the "+
		"new team. Possible values are WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
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
	return team.AddWorkspaceTeam(id, addWorkspaceRole, "", out, astroCoreClient)
}

func newWorkspaceTeamUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [id]",
		Aliases: []string{"up"},
		Short:   "Update a the role of a team in an Astro Workspace",
		Long: "Update the role of a team in an Astro Workspace\n$astro workspace team update [id] --role [WORKSPACE_MEMBER, " +
			"WORKSPACE_OPERATOR, WORKSPACE_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateWorkspaceTeam(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&updateWorkspaceRole, "role", "r", "", "The new role for the "+
		"team. Possible values are WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
	return cmd
}

func updateWorkspaceTeam(cmd *cobra.Command, args []string, out io.Writer) error {
	var id string

	// if an id was provided in the args we use it
	if len(args) > 0 {
		id = args[0]
	}

	if updateWorkspaceRole == "" {
		// no role was provided so ask the user for it
		updateWorkspaceRole = input.Text("Enter a team workspace role(WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER) to update team: ")
	}

	cmd.SilenceUsage = true
	return team.UpdateWorkspaceTeamRole(id, updateWorkspaceRole, "", out, astroCoreClient)
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(astroClient, out)
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id, astroClient, out)
}

func workspaceCreate(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return workspace.Create(workspaceName, workspaceDescription, enforceCD, out, astroCoreClient)
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Update(id, workspaceName, workspaceDescription, enforceCD, out, astroCoreClient)
}

func workspaceDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

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
	return user.AddWorkspaceUser(email, addWorkspaceRole, "", out, astroCoreClient)
}

func listWorkspaceUser(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return user.ListWorkspaceUsers(out, astroCoreClient, "")
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
		updateWorkspaceRole = input.Text("Enter a user Workspace role(WORKSPACE_MEMBER, WORKSPACE_OPERATOR and WORKSPACE_OWNER) to update user: ")
	}

	cmd.SilenceUsage = true
	return user.UpdateWorkspaceUserRole(email, updateWorkspaceRole, "", out, astroCoreClient)
}

func removeWorkspaceUser(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		// make sure the email is lowercase
		email = strings.ToLower(args[0])
	}

	cmd.SilenceUsage = true
	return user.RemoveWorkspaceUser(email, "", out, astroCoreClient)
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
