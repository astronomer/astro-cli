package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceID string
)

func newWorkspaceCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astro Workspaces",
		Long:    "Create and manage Workspaces on Astro. Workspaces can contain multiple Deployments and can be shared across users.",
	}
	cmd.AddCommand(
		newWorkspaceSwitchCmd(out),
	)
	return cmd
}

func newWorkspaceSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [workspace name/id]",
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

func workspaceSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	workspaceNameOrID := ""

	if len(args) == 1 {
		workspaceNameOrID = args[0]
	}
	cmd.SilenceUsage = true
	return workspace.Switch(workspaceNameOrID, astroCoreClient, out)
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
