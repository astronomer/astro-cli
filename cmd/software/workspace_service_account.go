package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	sa "github.com/astronomer/astro-cli/software/service_account"
	"github.com/spf13/cobra"
)

var (
	workspaceSAUserID   string
	workspaceSACategory string
	workspaceSALabel    string
	workspaceSARole     string

	workspaceSaCreateExample = `
  # Create service-account
  $ astro workspace service-account create --workspace-id=<workspace-id> --label=my_label --role=ROLE
`
	workspaceSaListExample = `
$ astro workspace service-account list --workspace-id=<workspace-id>
`
)

func newWorkspaceSaRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage Workspace Service Accounts resources",
		Long:    "Service Accounts represent a revokable token with access to an Astronomer Cluster",
	}
	cmd.AddCommand(
		newWorkspaceSaCreateCmd(out),
		newWorkspaceSaListCmd(out),
		newWorkspaceSaDeleteCmd(out),
	)

	return cmd
}

func newWorkspaceSaCreateCmd(out io.Writer) *cobra.Command { //nolint:dupl
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a Service Account in an Astronomer Cluster",
		Long:    "Create a Service Account in an Astronomer Cluster",
		Example: workspaceSaCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the workspace, you can leave it empty if you want to use your current context's workspace ID")
	cmd.Flags().StringVarP(&workspaceSAUserID, "user-id", "u", "", "ID of the user you want to link this service account to")
	cmd.Flags().StringVarP(&workspaceSACategory, "category", "c", "default", "Category of the new service account")
	cmd.Flags().StringVarP(&workspaceSALabel, "label", "l", "", "Label of the new service account")
	cmd.Flags().StringVarP(&workspaceSARole, "role", "r", houston.WorkspaceViewerRole, "Role (permissions) attached to the created service account")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}

func newWorkspaceSaListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Service Accounts inside a workspace",
		Long:    "List Service Accounts inside a workspace",
		Example: workspaceSaListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the workspace, you can leave it empty if you want to use your current context's workspace ID")
	return cmd
}

func newWorkspaceSaDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [service-account id]",
		Aliases: []string{"de"},
		Short:   "Delete a Service Account in the Astro Private Cloud platform",
		Long:    "Delete a Service Account in the Astro Private Cloud platform",
		Args:    cobra.ExactArgs(1),
		Example: "astro workspace sa delete cl0wh207g0496759fx0qof80q",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaDelete(cmd, out, args)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the workspace, you can leave it empty if you want to use your current context's workspace ID")
	return cmd
}

func workspaceSaCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if err := validateWorkspaceRole(workspaceSARole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.CreateUsingWorkspaceUUID(ws, workspaceSALabel, workspaceSACategory, workspaceSARole, houstonClient, out)
}

func workspaceSaList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.GetWorkspaceServiceAccounts(ws, houstonClient, out)
}

func workspaceSaDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.DeleteUsingWorkspaceUUID(args[0], ws, houstonClient, out)
}
