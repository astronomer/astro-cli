package apc

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	sa "github.com/astronomer/astro-cli/internal/platform/apc/service_account"
)

var (
	workspaceSAUserID   string
	workspaceSACategory string
	workspaceSAName     string
	workspaceSARole     string

	workspaceSaCreateExample = `
  # Create a service account in a Workspace
  astro workspace service-account create --workspace <WORKSPACE_ID> --name my_label \
    --role WORKSPACE_EDITOR
`
	workspaceSaListExample = `
  astro workspace service-account list --workspace <WORKSPACE_ID>
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
	addWorkspaceFlag(cmd.Flags(), "Workspace whose service accounts to manage (default: the current context's)", "w")
	cmd.Flags().StringVarP(&workspaceSAUserID, "user-id", "u", "", "ID of the user you want to link this service account to")
	cmd.Flags().StringVarP(&workspaceSACategory, "category", "c", "default", "Category of the new service account")
	// --name, as an Astro API token's; 1.x's --label, -l, is its alias.
	cmd.Flags().StringVarP(&workspaceSAName, "name", "n", "", "Name of the new service account")
	utils.AddAlias(cmd.Flags(), "label", "l", "name")
	cmd.Flags().StringVarP(&workspaceSARole, "role", "r", houston.WorkspaceViewerRole, "Role (permissions) attached to the created service account")
	_ = cmd.MarkFlagRequired("name") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
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
	addWorkspaceFlag(cmd.Flags(), "Workspace whose service accounts to manage (default: the current context's)", "w")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceSaDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <SERVICE_ACCOUNT_ID>",
		Aliases: []string{"de"},
		Short:   "Delete a service account in the APC platform",
		Long:    "Delete a service account in the APC platform. This cannot be undone, so the command asks for confirmation first unless --yes is given.",
		Args:    cobra.ExactArgs(1),
		Example: `  # Delete a service account from the current Workspace, after you confirm
  astro workspace service-account delete <SERVICE_ACCOUNT_ID>

  # Delete it without the confirmation, from a script
  astro workspace service-account delete <SERVICE_ACCOUNT_ID> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaDelete(cmd, out, args)
		},
	}
	addYesFlag(cmd, "Don't ask for confirmation before deleting the service account")
	addWorkspaceFlag(cmd.Flags(), "Workspace whose service accounts to manage (default: the current context's)", "w")
	addAccessOutputFlag(cmd)
	return cmd
}

func workspaceSaCreate(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if err := validateWorkspaceRole(workspaceSARole); err != nil {
		return fmt.Errorf("failed to find a valid role: %w", err)
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	created, err := sa.CreateUsingWorkspaceUUID(ws, workspaceSAName, workspaceSACategory, workspaceSARole, houstonClient)
	if err != nil {
		return err
	}
	return renderServiceAccountCreated(r, &created)
}

func workspaceSaList(cmd *cobra.Command, out io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out}
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
	r := cliout.Renderer{Format: accessOutput, Out: out}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	ok, err := confirmRemoval(out, serviceAccountDeleteQuestion(args[0]), serviceAccountDeleteCanceled)
	if err != nil || !ok {
		return err
	}
	deleted, err := sa.DeleteUsingWorkspaceUUID(args[0], ws, houstonClient)
	if err != nil {
		return err
	}
	removal := workspaceServiceAccountRemovalJSON{ID: deleted.ID, Label: orNull(deleted.Label), WorkspaceID: ws, Action: accessActionDeleted}
	return r.Emit(removal, serviceAccountDeletedText(&deleted))
}
