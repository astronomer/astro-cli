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
  astro workspace service-account create --workspace <WORKSPACE_ID> --name ci-deployer \
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
		Short:   "Manage APC Workspace service accounts",
		Long:    "Service accounts represent revocable tokens with access to the APC platform",
	}
	deleteCmd := newWorkspaceSaDeleteCmd(out)
	cmd.AddCommand(
		newWorkspaceSaCreateCmd(out),
		newWorkspaceSaListCmd(out),
		deleteCmd,
	)
	cmd.AddCommand(cliout.RemovedAlias("de", deleteCmd))

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
			return workspaceSaCreate(out, cmd.ErrOrStderr())
		},
	}
	addWorkspaceFlag(cmd.Flags(), "Workspace whose service accounts to manage (default: the current context's)", "w")
	cmd.Flags().StringVar(&workspaceSAUserID, "user-id", "", "ID of the user to link this service account to")
	cmd.Flags().StringVar(&workspaceSACategory, "category", "default", "Category of the new service account")
	// --name, as an Astro API token's; 1.x's --label, -l, is its alias.
	cmd.Flags().StringVarP(&workspaceSAName, "name", "n", "", "Name of the new service account")
	utils.AddAlias(cmd.Flags(), "label", "name")
	cmd.Flags().StringVar(&workspaceSARole, "role", houston.WorkspaceViewerRole, "Role (permissions) attached to the created service account")
	_ = cmd.MarkFlagRequired("name") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceSaListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List service accounts inside a Workspace",
		Long:    "List service accounts inside a Workspace",
		Example: workspaceSaListExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaList(out, cmd.ErrOrStderr())
		},
	}
	addWorkspaceFlag(cmd.Flags(), "Workspace whose service accounts to manage (default: the current context's)", "w")
	addAccessOutputFlag(cmd)
	return cmd
}

func newWorkspaceSaDeleteCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete a service account in the APC platform",
		Long:    "Delete a service account in the APC platform. This cannot be undone; it asks for confirmation unless --yes is given.",
		Example: `  # Delete a service account from the current Workspace, after you confirm
  astro workspace service-account delete <SERVICE_ACCOUNT_ID>

  # Delete it without the confirmation, from a script
  astro workspace service-account delete <SERVICE_ACCOUNT_ID> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSaDelete(out, cmd.ErrOrStderr(), args)
		},
	}, cliout.Required("SERVICE_ACCOUNT_ID"))
	addYesFlag(cmd, "Don't ask for confirmation before deleting the service account")
	addWorkspaceFlag(cmd.Flags(), "Workspace whose service accounts to manage (default: the current context's)", "w")
	addAccessOutputFlag(cmd)
	return cmd
}

func workspaceSaCreate(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: errOut}
	if workspaceSAName == "" {
		return errServiceAccountNameEmpty
	}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if err := validateWorkspaceRole(workspaceSARole); err != nil {
		return fmt.Errorf("could not find a valid role: %w", err)
	}
	created, err := sa.CreateUsingWorkspaceUUID(ws, workspaceSAName, workspaceSACategory, workspaceSARole, houstonClient)
	if err != nil {
		return err
	}
	return renderServiceAccountCreated(r, &created)
}

func workspaceSaList(out, errOut io.Writer) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: errOut}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	sas, err := sa.GetWorkspaceServiceAccounts(ws, houstonClient)
	if err != nil {
		return err
	}
	return renderServiceAccountList(r, sas)
}

func workspaceSaDelete(out, errOut io.Writer, args []string) error {
	r := cliout.Renderer{Format: accessOutput, Out: out, Err: errOut}
	ws, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if err := confirm(serviceAccountDeleteQuestion(args[0])); err != nil {
		return err
	}
	deleted, err := sa.DeleteUsingWorkspaceUUID(args[0], ws, houstonClient)
	if err != nil {
		return err
	}
	removal := workspaceServiceAccountRemovalJSON{ID: deleted.ID, Label: orNull(deleted.Label), WorkspaceID: ws, Action: accessActionDeleted}
	return r.Emit(removal, serviceAccountDeletedText(&deleted))
}
