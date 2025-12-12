package cloud

import (
	"errors"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/ide"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
)

var (
	ideProjectID string
	ideSessionID string
)

func newIDECommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ide",
		Short: "Manage Astro IDE resources",
		Long:  "Create and manage Astro IDE resources.",
	}
	cmd.AddCommand(newIDEProjectCmd(out))
	return cmd
}

func newIDEProjectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Astro IDE projects",
		Long:  "Create and manage Astro IDE projects in your workspace.",
	}
	cmd.AddCommand(
		newIDEListProjectCmd(out),
		newIDEImportProjectCmd(out),
		newIDEExportProjectCmd(out),
	)
	return cmd
}

// newIDEListProjectCmd returns a new cobra command for listing IDE projects
func newIDEListProjectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astro IDE projects in your workspace",
		Long:    "List all Astro IDE projects in your workspace and optionally select one for future commands.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listIDEProjects(cmd, out)
		},
		Example: `
# List all IDE projects in your workspace
astro IDE project list
`,
	}
	return cmd
}

func newIDEImportProjectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import",
		Aliases: []string{"i"},
		Short:   "Import a project from Astro IDE",
		Long:    "Import a project from Astro IDE to your local directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return importIDEProject(cmd, out)
		},
		Example: `
# Import a project from Astro IDE
astro ide project import

# Import a project from a specific Astro IDE project
astro ide project import --project-id <project-id>

# Import a project from a specific Astro IDE session
astro ide project import --project-id <project-id> --session-id <session-id>
`,
	}
	cmd.Flags().StringVarP(&ideProjectID, "project-id", "p", "", "Project ID to import")
	cmd.Flags().StringVarP(&ideSessionID, "session-id", "s", "", "Session ID to import")
	return cmd
}

func newIDEExportProjectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export",
		Aliases: []string{"e"},
		Short:   "Export a project to Astro IDE",
		Long:    "Export a project from your local directory to Astro IDE.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return exportProject(cmd, out)
		},
		Example: `
# Export a project to Astro IDE
astro ide export

# Export a project to a specific Astro IDE project
astro ide project export --project-id <project-id>

# Force export to an Astro IDE project
astro ide project export --project-id <project-id> --force
`,
	}
	cmd.Flags().StringVarP(&ideProjectID, "project-id", "p", "", "Project ID to export")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force export to overwrite project lock")
	return cmd
}

func listIDEProjects(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return ide.List(astroCoreClient, out)
}

func importIDEProject(cmd *cobra.Command, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	orgID, wsID, err := validateWorkspaceAndOrgID(&ctx)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return ide.ImportProject(astroCoreClient, ideProjectID, ideSessionID, orgID, wsID, out)
}

func exportProject(cmd *cobra.Command, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	orgID, wsID, err := validateWorkspaceAndOrgID(&ctx)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return ide.ExportProject(astroCoreClient, ideProjectID, orgID, wsID, ctx.Domain, force, out)
}

func validateWorkspaceAndOrgID(ctx *config.Context) (orgID, wsID string, err error) {
	orgID = ctx.Organization
	if orgID == "" {
		return "", "", errors.New("no organization ID provided and no organization set in context. Please set context or provide organization ID")
	}

	wsID = ctx.Workspace
	if wsID == "" {
		return "", "", errors.New("no workspace ID provided and no workspace set in context. Please set context or provide workspace ID")
	}

	return orgID, wsID, nil
}
