package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/workspace"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var workspaceID string

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer Workspaces",
		Long:    "Create and manage Workspaces on Astro. Workspaces can contain multiple Deployments and can be shared across users.",
	}
	cmd.AddCommand(
		newWorkspaceListCmd(out),
		newWorkspaceSwitchCmd(out),
	)
	return cmd
}

func newWorkspaceListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astronomer Workspaces in your Organization",
		Long:    "List all Astronomer Workspaces in your Organization.",
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
		Short:   "Switch to a different Astronomer Workspace",
		Long:    "Switch to a different Astronomer Workspace",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return workspaceSwitch(cmd, out, args)
		},
	}
	return cmd
}

func workspaceList(cmd *cobra.Command, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return workspace.List(astroGQLClient, out)
}

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	id := ""

	if len(args) == 1 {
		id = args[0]
	}

	return workspace.Switch(id, astroGQLClient, out)
}

func coalesceWorkspace() (string, error) {
	wsFlag := workspaceID
	wsCfg, err := workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, "failed to get current workspace")
	}

	if wsFlag != "" {
		return wsFlag, nil
	}

	if wsCfg != "" {
		return wsCfg, nil
	}

	return "", errors.New("no valid workspace source found")
}
