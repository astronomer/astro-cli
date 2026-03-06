package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/spf13/cobra"
)

var (
	orgSwitch              = organization.Switch
	wsSwitch               = workspace.Switch
	shouldDisplayLoginLink bool
)

func newOrganizationCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "organization",
		Aliases: []string{"org"},
		Short:   "Manage Astronomer Organizations",
		Long:    "Manage your Astro Organizations. These commands are for users in more than one Organization",
	}
	cmd.AddCommand(
		newOrganizationSwitchCmd(out),
	)
	return cmd
}

func newOrganizationSwitchCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "switch [organization name/id]",
		Aliases: []string{"sw"},
		Short:   "Switch to a different Organization",
		Long:    "Switch to a different Organization",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return organizationSwitch(cmd, out, args)
		},
	}

	cmd.Flags().BoolVarP(&shouldDisplayLoginLink, "login-link", "l", false, "Get login link to login on a separate device for organization switch")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "The Workspace's unique identifier")

	return cmd
}

func organizationSwitch(cmd *cobra.Command, out io.Writer, args []string) error {
	cmd.SilenceUsage = true

	organizationNameOrID := ""

	if len(args) == 1 {
		organizationNameOrID = args[0]
	}

	if err := orgSwitch(organizationNameOrID, astroCoreClient, platformCoreClient, out, shouldDisplayLoginLink); err != nil {
		return err
	}

	if workspaceID != "" {
		return wsSwitch(workspaceID, astroCoreClient, out)
	}
	return nil
}
