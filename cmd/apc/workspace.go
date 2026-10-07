package apc

import (
	"errors"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	errUpdateWorkspaceInvalidArgs  = errors.New("must specify at least one attribute to update (--label or --description)")
	errCreateWorkspaceMissingLabel = errors.New("must specify a label for your workspace")
)

var (
	workspaceCreateDescription string
	workspaceCreateLabel       string
	workspaceUpdateLabel       string
	workspaceUpdateDescription string
	workspacePaginated         bool
	workspacePageSize          int
	workspaceOutput            string
	workspaceDeleteExample     = `
  astro workspace delete <WORKSPACE_ID>
`
)

const defaultPageSize = 100

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage APC Workspaces",
		Long:    "Workspaces contain a group of Airflow Cluster Deployments. The creator of the workspace can invite other users into it",
	}
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceCreateCmd(out),
		newWorkspaceDeleteCmd(out),
		newWorkspaceSwitchCmd(out),
		newWorkspaceUpdateCmd(out),
		newWorkspaceUserRootCmd(out),
		newWorkspaceSaRootCmd(out),
		newWorkspaceTeamRootCmd(out),
	)
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
			return workspaceList(cmd, out)
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
		Example: `  # Create a Workspace with a label and a description
  astro workspace create --label my-workspace --description "Production pipelines"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceCreateLabel, "label", "l", "", "Label for your new workspace")
	cmd.Flags().StringVarP(&workspaceCreateDescription, "description", "d", "", "Description for your new workspace")
	_ = cmd.MarkFlagRequired("label") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	cliout.AddOutputFlag(cmd, &workspaceOutput)

	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <WORKSPACE_ID>",
		Aliases: []string{"de"},
		Short:   "Delete an APC Workspace",
		Long:    "Delete an APC Workspace",
		Example: workspaceDeleteExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, out, args)
		},
	}
	cliout.AddOutputFlag(cmd, &workspaceOutput)
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [WORKSPACE_ID]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different APC Workspace",
		Long:    "Switch to a different APC Workspace. If you do not provide the workspace ID, you choose one from a list.",
		Example: `  # Switch to a Workspace by its ID
  astro workspace switch <WORKSPACE_ID>

  # Choose a Workspace from a list
  astro workspace switch`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, out, args)
		},
	}

	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		cmd.Flags().BoolVarP(&workspacePaginated, "paginated", "p", false, "Paginated workspace list")
		cmd.Flags().IntVarP(&workspacePageSize, "page-size", "s", 0, "Page size of the workspace list if paginated is set to true")
	}
	cliout.AddOutputFlag(cmd, &workspaceOutput)
	return cmd
}

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <WORKSPACE_ID>",
		Aliases: []string{"up"},
		Short:   "Update an APC Workspace",
		Long:    "Update a Workspace name, as well as users and roles assigned to a Workspace",
		Example: `  # Change a Workspace's label and description
  astro workspace update <WORKSPACE_ID> --label my-new-label --description "My new description"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, out, args)
		},
	}

	cmd.Flags().StringVarP(&workspaceUpdateLabel, "label", "l", "", "The new label you want to give to your workspace")
	cmd.Flags().StringVarP(&workspaceUpdateDescription, "description", "d", "", "The new description you want to give to your workspace")
	cliout.AddOutputFlag(cmd, &workspaceOutput)

	return cmd
}

// workspaceRenderer parses -o, before anything else so a bad value is a
// usage error, and returns the Renderer the command publishes through.
func workspaceRenderer(out io.Writer) (cliout.Renderer, error) {
	format, err := cliout.ParseFormat(workspaceOutput)
	if err != nil {
		return cliout.Renderer{}, err
	}
	return cliout.Renderer{Format: format, Out: out}, nil
}

func workspaceCreate(cmd *cobra.Command, out io.Writer) error {
	r, err := workspaceRenderer(out)
	if err != nil {
		return err
	}
	if workspaceCreateLabel == "" {
		return errCreateWorkspaceMissingLabel
	}

	if workspaceCreateDescription == "" {
		workspaceCreateDescription = "N/A"
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	w, err := workspace.Create(workspaceCreateLabel, workspaceCreateDescription, houstonClient)
	if err != nil {
		return err
	}
	return emitWorkspace(r, w, "\n Successfully created workspace")
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	r, err := workspaceRenderer(out)
	if err != nil {
		return err
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
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

func workspaceDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	r, err := workspaceRenderer(out)
	if err != nil {
		return err
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	w, err := workspace.Delete(args[0], houstonClient)
	if err != nil {
		return err
	}
	return emitWorkspaceRemoval(r, args[0], w)
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
	r, err := workspaceRenderer(out)
	if err != nil {
		return err
	}
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

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	w, err := workspace.Update(args[0], houstonClient, argsMap)
	if err != nil {
		return err
	}
	return emitWorkspace(r, w, "\n Successfully updated workspace")
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	r, err := workspaceRenderer(out)
	if err != nil {
		return err
	}
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

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
