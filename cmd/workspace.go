package cmd

import (
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	sa "github.com/astronomer/astro-cli/serviceaccount"
	"github.com/astronomer/astro-cli/workspace"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceUpdateAttrs   = []string{"label"}
	createDesc             string
	workspaceDeleteExample = `
  $ astro workspace delete <workspace-id>
`

	workspaceSaCreateExample = `
  # Create service-account
  $ astro workspace service-account create --workspace-id=<workspace-id> --label=my_label --role=ROLE
`
	workspaceSaGetExample = `
$ astro workspace service-account get --workspace-id=<workspace-id>
`
)

func newWorkspaceCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer Workspaces",
		Long:    "Workspaces contain a group of Airflow Cluster Deployments. The creator of the workspace can invite other users into it",
	}
	cmd.AddCommand(
		newWorkspaceListCmd(client, out),
		newWorkspaceCreateCmd(client, out),
		newWorkspaceDeleteCmd(client, out),
		newWorkspaceSwitchCmd(client, out),
		newWorkspaceUpdateCmd(client, out),
		newWorkspaceUserRootCmd(client, out),
		newWorkspaceSaRootCmd(client, out),
	)
	return cmd
}

func newWorkspaceListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Astronomer Workspaces",
		Long:    "List Astronomer Workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, args, client, out)
		},
	}
	return cmd
}

func newWorkspaceCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create WORKSPACE",
		Aliases: []string{"cr"},
		Short:   "Create an Astronomer Workspace",
		Long:    "Create an Astronomer Workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&createDesc, "desc", "d", "", "description for your new workspace")
	return cmd
}

func newWorkspaceDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete WORKSPACE",
		Aliases: []string{"de"},
		Short:   "Delete an Astronomer Workspace",
		Long:    "Delete an Astronomer Workspace",
		Example: workspaceDeleteExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, args, client, out)
		},
	}
	return cmd
}

func newWorkspaceSwitchCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch WORKSPACE",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Astronomer Workspace",
		Long:    "Switch to a different Astronomer Workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, client, out, args)
		},
	}
	return cmd
}

func newWorkspaceUpdateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Astronomer Workspace",
		Long:    "Update a Workspace name, as well as users and roles assigned to a Workspace",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) <= 1 {
				return errors.New("must specify a workspace ID and at least one attribute to update")
			}
			return updateArgValidator(args[1:], workspaceUpdateAttrs)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, client, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Workspace User resources",
		Long:  "Users can be added or removed from Workspaces",
	}
	cmd.AddCommand(
		newWorkspaceUserAddCmd(client, out),
		newWorkspaceUserUpdateCmd(client, out),
		newWorkspaceUserRmCmd(client, out),
		newWorkspaceUserListCmd(client, out),
	)
	return cmd
}

func newWorkspaceUserAddCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add EMAIL",
		Short: "Add a User to a Workspace",
		Long:  "Add a User to a Workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserAdd(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", "WORKSPACE_VIEWER", "role assigned to user")
	return cmd
}

func newWorkspaceUserUpdateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update user role",
		Short: "Update a User's Role for a Workspace",
		Long:  "Update a User's Role for a Workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserUpdate(cmd, client, out, args)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceRole, "role", "WORKSPACE_VIEWER", "role assigned to user")
	return cmd
}

func newWorkspaceUserRmCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove EMAIL",
		Aliases: []string{"rm"},
		Short:   "Remove a User from a Workspace",
		Long:    "Remove a User from a Workspace",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserRm(cmd, client, out, args)
		},
	}
	return cmd
}

func newWorkspaceUserListCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Astronomer Workspaces",
		Long:    "List Astronomer Workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUserList(cmd, client, out, args)
		},
	}
	return cmd
}

func newWorkspaceSaRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage Astronomer Service Accounts",
		Long:    "Service Accounts represent a revokable token with access to an Astronomer Cluster",
	}
	cmd.AddCommand(
		newWorkspaceSaCreateCmd(client, out),
		newWorkspaceSaGetCmd(client, out),
		newWorkspaceSaDeleteCmd(client, out),
	)
	return cmd
}

// nolint:dupl
func newWorkspaceSaCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a Service Account in an Astronomer Cluster",
		Long:    "Create a Service Account in an Astronomer Cluster",
		Example: workspaceSaCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaCreate(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "[ID]")
	cmd.Flags().StringVarP(&userID, "user-id", "u", "", "[ID]")
	cmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	cmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	cmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")
	cmd.Flags().StringVarP(&role, "role", "r", "viewer", "ROLE")
	return cmd
}

func newWorkspaceSaGetCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a Service Account by workspace",
		Long:    "Get a Service Account by workspace",
		Example: workspaceSaGetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaGet(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "[ID]")
	_ = cmd.MarkFlagRequired("workspace-id")
	return cmd
}

func newWorkspaceSaDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [SA-ID]",
		Aliases: []string{"de"},
		Short:   "Delete a Service Account in the astronomer platform",
		Long:    "Delete a Service Account in the astronomer platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaDelete(cmd, args, client, out)
		},
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "[ID]")
	_ = cmd.MarkFlagRequired("workspace-id")
	return cmd
}

func workspaceCreate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	if createDesc == "" {
		createDesc = "N/A"
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Create(args[0], createDesc, client, out)
}

func workspaceList(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(client, out)
}

func workspaceDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Delete(args[0], client, out)
}

func workspaceUpdate(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	argsMap, err := argsToMap(args[1:])
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Update(args[0], client, out, argsMap)
}

func workspaceUserAdd(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.Add(ws, args[0], workspaceRole, client, out)
}

func workspaceUserUpdate(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	if err := validateWorkspaceRole(workspaceRole); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.UpdateRole(ws, args[0], workspaceRole, client, out)
}

func workspaceUserRm(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Remove(ws, args[0], client, out)
}

func workspaceSwitch(cmd *cobra.Command, client *houston.Client, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id, client, out)
}

func workspaceUserList(_ *cobra.Command, client *houston.Client, out io.Writer, _ []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}
	return workspace.ListRoles(ws, client, out)
}

func workspaceSaCreate(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
	if label == "" {
		return errors.New("must provide a service-account label with the --label (-l) flag")
	}

	if err := validateRole(role); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}
	fullRole := strings.Join([]string{"WORKSPACE", strings.ToUpper(role)}, "_")
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.CreateUsingWorkspaceUUID(workspaceID, label, category, fullRole, client, out)
}

func workspaceSaGet(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.GetWorkspaceServiceAccounts(workspaceID, client, out)
}

func workspaceSaDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.DeleteUsingWorkspaceUUID(args[0], workspaceID, client, out)
}
