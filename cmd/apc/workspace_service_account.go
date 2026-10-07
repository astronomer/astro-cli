package apc

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	sa "github.com/astronomer/astro-cli/internal/platform/apc/service_account"
)

var (
	workspaceSAUserID   string
	workspaceSACategory string
	workspaceSALabel    string
	workspaceSARole     string

	workspaceSaCreateExample = `
  # Create a service account in a Workspace
  astro workspace service-account create --workspace-id <WORKSPACE_ID> --label my_label \
    --role WORKSPACE_EDITOR
`
	workspaceSaListExample = `
  astro workspace service-account list --workspace-id <WORKSPACE_ID>
`
)

func newWorkspaceSaRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage APC workspace service accounts",
		Long:    "Service accounts represent revocable tokens with access to the APC platform",
	}
	cmd.AddCommand(
		newWorkspaceSaCreateCmd(out),
		newWorkspaceSaListCmd(out),
		newWorkspaceSaDeleteCmd(out),
	)

	return cmd
}

func newWorkspaceSaCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a service account in the APC platform",
		Long:    "Create a service account in the APC platform",
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
	_ = cmd.MarkFlagRequired("label") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceSaListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List service accounts inside a workspace",
		Long:    "List service accounts inside a workspace",
		Example: workspaceSaListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the workspace, you can leave it empty if you want to use your current context's workspace ID")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceSaDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <SERVICE_ACCOUNT_ID>",
		Aliases: []string{"de"},
		Short:   "Delete a service account in the APC platform",
		Long:    "Delete a service account in the APC platform",
		Args:    cobra.ExactArgs(1),
		Example: `  astro workspace service-account delete <SERVICE_ACCOUNT_ID>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaDelete(cmd, out, args)
		},
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the workspace, you can leave it empty if you want to use your current context's workspace ID")
	addAccessOutputFlag(cmd)
	return cmd
}

func workspaceSaCreate(cmd *cobra.Command, out io.Writer) error {
	r, err := accessRenderer(out)
	if err != nil {
		return err
	}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if err := validateWorkspaceRole(workspaceSARole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	created, err := sa.CreateUsingWorkspaceUUID(ws, workspaceSALabel, workspaceSACategory, workspaceSARole, houstonClient)
	if err != nil {
		return err
	}
	return renderServiceAccountCreated(r, &created)
}

func workspaceSaList(cmd *cobra.Command, out io.Writer) error {
	r, err := accessRenderer(out)
	if err != nil {
		return err
	}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	sas, err := sa.GetWorkspaceServiceAccounts(ws, houstonClient)
	if err != nil {
		return err
	}
	return renderServiceAccountList(r, sas)
}

func workspaceSaDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	r, err := accessRenderer(out)
	if err != nil {
		return err
	}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	deleted, err := sa.DeleteUsingWorkspaceUUID(args[0], ws, houstonClient)
	if err != nil {
		return err
	}
	removal := workspaceServiceAccountRemovalJSON{ID: deleted.ID, Label: orNull(deleted.Label), WorkspaceID: ws, Action: accessActionDeleted}
	return r.Emit(removal, serviceAccountDeletedText(&deleted))
}
