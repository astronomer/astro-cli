package software

import (
	"errors"
	"io"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/software/workspace"
	"github.com/spf13/cobra"
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
	workspaceDeleteExample     = `
  $ astro workspace delete <workspace-id>
`
)

const defaultPageSize = 100

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer Workspaces",
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
		Short:   "List Astronomer Workspaces",
		Long:    "List Astronomer Workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceList(cmd, out)
		},
	}
	return cmd
}

func newWorkspaceCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Astronomer Workspace",
		Long:    "Create an Astronomer Workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceCreateLabel, "label", "l", "", "Label for your new workspace")
	cmd.Flags().StringVarP(&workspaceCreateDescription, "description", "d", "", "Description for your new workspace")
	_ = cmd.MarkFlagRequired("label")

	return cmd
}

func newWorkspaceDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [workspace ID]",
		Aliases: []string{"de"},
		Short:   "Delete an Astronomer Workspace",
		Long:    "Delete an Astronomer Workspace",
		Example: workspaceDeleteExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceDelete(cmd, out, args)
		},
	}
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [workspace ID]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Astronomer Workspace",
		Long:    "Switch to a different Astronomer Workspace. If you do not provide the workspace ID, you will switch to the previously used workspace.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, out, args)
		},
	}

	if houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		cmd.Flags().BoolVarP(&workspacePaginated, "paginated", "p", false, "Paginated workspace list")
		cmd.Flags().IntVarP(&workspacePageSize, "page-size", "s", 0, "Page size of the workspace list if paginated is set to true")
	}
	return cmd
}

func newWorkspaceUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [workspace ID]",
		Aliases: []string{"up"},
		Short:   "Update an Astronomer Workspace",
		Long:    "Update a Workspace name, as well as users and roles assigned to a Workspace",
		Example: "astro workspace update cl0wftysg00137a93je05hngx --label=my-new-label --description=\"my new description\"",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceUpdate(cmd, out, args)
		},
	}

	cmd.Flags().StringVarP(&workspaceUpdateLabel, "label", "l", "", "The new label you want to give to your workspace")
	cmd.Flags().StringVarP(&workspaceUpdateDescription, "description", "d", "", "The new description you want to give to your workspace")

	return cmd
}

func workspaceCreate(cmd *cobra.Command, out io.Writer) error {
	if workspaceCreateLabel == "" {
		return errCreateWorkspaceMissingLabel
	}

	if workspaceCreateDescription == "" {
		workspaceCreateDescription = "N/A"
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.Create(workspaceCreateLabel, workspaceCreateDescription, houstonClient, out)
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(houstonClient, out)
}

func workspaceDelete(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return workspace.Delete(args[0], houstonClient, out)
}

func workspaceUpdate(cmd *cobra.Command, out io.Writer, args []string) error {
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

	return workspace.Update(args[0], houstonClient, out, argsMap)
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	pageSize := config.CFG.PageSize.GetInt()

	if config.CFG.Interactive.GetBool() || workspacePaginated {
		if workspacePageSize <= 0 && pageSize > 0 {
			workspacePageSize = pageSize
		}

		if !(workspacePageSize > 0 && workspacePageSize <= defaultPageSize) {
			logger.Logger.Warnf("Page size cannot be more than %d, reducing the page size to %d", defaultPageSize, defaultPageSize)
			workspacePageSize = defaultPageSize
		}
	}

	// overriding workspace pagesize if houston version is before 0.30.0, since that doesn't support pagination
	if !houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.30.0"}) {
		workspacePageSize = 0
	}

	return workspace.Switch(id, workspacePageSize, houstonClient, out)
}
