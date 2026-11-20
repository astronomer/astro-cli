package apc

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	errUpdateWorkspaceInvalidArgs  = cliout.Usage(errors.New("set at least one of --name or --description to update"))
	errCreateWorkspaceMissingLabel = errors.New("a Workspace needs a name; set --name")
)

var (
	workspaceCreateDescription string
	workspaceCreateLabel       string
	workspaceUpdateLabel       string
	workspaceUpdateDescription string
	workspacePaginated         bool
	workspacePageSize          int
	workspaceOutput            cliout.Format
	workspaceDeleteExample     = `  # Delete a Workspace, after you confirm
  astro workspace delete <WORKSPACE_ID>

  # Delete it without the confirmation, from a script
  astro workspace delete <WORKSPACE_ID> --yes`
)

const defaultPageSize = 100

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage APC Workspaces",
		Long:    "A Workspace groups Deployments. The creator of a Workspace can invite other users to it.",
	}
	deleteCmd := newWorkspaceDeleteCmd(out)
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceCreateCmd(out),
		deleteCmd,
		newWorkspaceSwitchCmd(out),
		newWorkspaceUpdateCmd(out),
		newWorkspaceUserRootCmd(out),
		newWorkspaceSaRootCmd(out),
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
		Short:   "List APC Workspaces",
		Long:    "List APC Workspaces",
		Example: `  astro workspace list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(out)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceOutput)
	return cmd
}

func newWorkspaceCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an APC Workspace",
		Long:    "Create an APC Workspace",
		Example: `  # Create a Workspace with a name and a description
  astro workspace create --name my-workspace --description "Production pipelines"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(out)
		},
	}
	cmd.Flags().StringVarP(&workspaceCreateLabel, "name", "n", "", "Name of the new Workspace")
	utils.AddAlias(cmd.Flags(), "label", "name")
	cmd.Flags().StringVar(&workspaceCreateDescription, "description", "", "Description of the new Workspace")
	_ = cmd.MarkFlagRequired("name") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &workspaceOutput)

	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <WORKSPACE_ID>",
		Aliases: []string{"rm"},
		Short:   "Delete an APC Workspace",
		Long:    "Delete an APC Workspace. This cannot be undone; it asks for confirmation unless --yes is given.",
		Example: workspaceDeleteExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(out, args)
		},
	}
	addYesFlag(cmd, "Don't ask for confirmation before deleting the Workspace")
	cliout.AddOutputFlag(cmd, &workspaceOutput)
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [WORKSPACE_ID]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different APC Workspace",
		Long:    "Switch to a different APC Workspace. Without a Workspace ID, it shows a list to choose from.",
		Example: `  # Switch to a Workspace by its ID
  astro workspace switch <WORKSPACE_ID>

  # Choose a Workspace from a list
  astro workspace switch`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(out, args)
		},
	}

	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		cmd.Flags().BoolVar(&workspacePaginated, "paginated", false, "Page through the Workspace list")
		cmd.Flags().IntVar(&workspacePageSize, "page-size", 0, "Page size of the Workspace list with --paginated")
	}
	cliout.AddOutputFlag(cmd, &workspaceOutput)
	return cmd
}

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <WORKSPACE_ID>",
		Aliases: []string{"up"},
		Short:   "Update an APC Workspace",
		Long:    "Update a Workspace's name or description.",
		Example: `  # Change a Workspace's name and description
  astro workspace update <WORKSPACE_ID> --name my-new-name --description "My new description"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(out, args)
		},
	}

	cmd.Flags().StringVarP(&workspaceUpdateLabel, "name", "n", "", "New name for the Workspace")
	utils.AddAlias(cmd.Flags(), "label", "name")
	cmd.Flags().StringVar(&workspaceUpdateDescription, "description", "", "New description for the Workspace")
	cliout.AddOutputFlag(cmd, &workspaceOutput)

	return cmd
}

func workspaceCreate(out io.Writer) error {
	r := cliout.Renderer{Format: workspaceOutput, Out: out}
	if workspaceCreateLabel == "" {
		return errCreateWorkspaceMissingLabel
	}

	if workspaceCreateDescription == "" {
		workspaceCreateDescription = "N/A"
	}

	w, err := workspace.Create(workspaceCreateLabel, workspaceCreateDescription, houstonClient)
	if err != nil {
		return err
	}
	return emitWorkspace(r, w, "\n Created Workspace")
}

func workspaceList(out io.Writer) error {
	r := cliout.Renderer{Format: workspaceOutput, Out: out}
	ws, err := workspace.List(houstonClient)
	if err != nil {
		return err
	}
	// The list marks the current context's workspace, and fails without a
	// context to read it from, as it always has.
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	return emitWorkspaceList(r, ws, c.Workspace)
}

func workspaceDelete(out io.Writer, args []string) error {
	r := cliout.Renderer{Format: workspaceOutput, Out: out}

	if err := confirm(fmt.Sprintf("\nDelete Workspace %s? This cannot be undone.", args[0])); err != nil {
		return err
	}
	w, err := workspace.Delete(args[0], houstonClient)
	if err != nil {
		return err
	}
	return emitWorkspaceRemoval(r, args[0], w)
}

func workspaceUpdate(out io.Writer, args []string) error {
	r := cliout.Renderer{Format: workspaceOutput, Out: out}
	argsMap := map[string]string{}
	if workspaceUpdateDescription != "" {
		argsMap["description"] = workspaceUpdateDescription
	}
	if workspaceUpdateLabel != "" {
		argsMap["label"] = workspaceUpdateLabel
	}

	if len(argsMap) == 0 {
		return errUpdateWorkspaceInvalidArgs
	}

	w, err := workspace.Update(args[0], houstonClient, argsMap)
	if err != nil {
		return err
	}
	return emitWorkspace(r, w, "\n Updated Workspace")
}

func workspaceSwitch(out io.Writer, args []string) error {
	r := cliout.Renderer{Format: workspaceOutput, Out: out}

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	pageSize := config.CFG.PageSize.GetInt()

	capped := false
	if config.CFG.Interactive.GetBool() || workspacePaginated {
		if workspacePageSize <= 0 && pageSize > 0 {
			workspacePageSize = pageSize
		}

		if !(workspacePageSize > 0 && workspacePageSize <= defaultPageSize) {
			capped = true
			workspacePageSize = defaultPageSize
		}
	}

	// overriding workspace pagesize if houston version is before 0.30.0, since that doesn't support pagination
	if !houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		workspacePageSize = 0
	}

	// With no ID the switch asks which workspace, which a run under
	// --output json may not: the picker refuses, naming the argument. It
	// refuses here, before the page-size note, so a refused run prints only
	// the refusal.
	if id == "" {
		if err := workspace.MayPick(workspacePageSize > 0); err != nil {
			return err
		}
	}
	if capped {
		logger.Warnf("Page size cannot be more than %d, reducing the page size to %d", defaultPageSize, defaultPageSize)
	}

	w, quit, err := workspace.Switch(id, workspacePageSize, houstonVersion, houstonClient)
	if err != nil {
		return err
	}
	if quit {
		// Quit at the picker: nothing switched, and nothing to show.
		return nil
	}
	return emitSwitched(r, w)
}
