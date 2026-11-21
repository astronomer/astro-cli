package astro

import (
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/ide"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	ideProjectID     string
	ideNoOpen        bool
	ideSessionID     string
	ideImportYes     bool
	ideProjectOutput cliout.Format
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
		Long:  "Create and manage Astro IDE projects in your Workspace.",
	}
	cmd.AddCommand(
		newIDEListProjectCmd(out),
		newIDEImportProjectCmd(out),
		newIDEExportProjectCmd(out),
	)
	cliout.AddOutputFlag(cmd, &ideProjectOutput)
	return cmd
}

// newIDEListProjectCmd returns a new cobra command for listing IDE projects
func newIDEListProjectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Astro IDE projects in your Workspace",
		Long:    "List all Astro IDE projects in your Workspace and optionally select one for future commands.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listIDEProjects(out)
		},
		Example: `  # List all IDE projects in your workspace
  astro ide project list`,
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
		Example: `  # Import a project from Astro IDE
  astro ide project import

  # Import a specific Astro IDE project
  astro ide project import --project-id <PROJECT_ID>

  # Import a project from a specific Astro IDE session
  astro ide project import --project-id <PROJECT_ID> --session-id <SESSION_ID>`,
	}
	cmd.Flags().StringVar(&ideProjectID, "project-id", "", "Project ID to import")
	cmd.Flags().StringVar(&ideSessionID, "session-id", "", "Session ID to import")
	cmd.Flags().BoolVarP(&ideImportYes, "yes", "y", false, "Import into the current directory without asking when it is not empty")
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
		Example: `  # Export a project to Astro IDE
  astro ide project export

  # Export it to a specific Astro IDE project
  astro ide project export --project-id <PROJECT_ID>

  # Export it even though the Astro IDE project is locked
  astro ide project export --project-id <PROJECT_ID> --force`,
	}
	cmd.Flags().StringVar(&ideProjectID, "project-id", "", "Project ID to export")
	cmd.Flags().BoolVar(&force, "force", false, "Force export to overwrite project lock")
	cmd.Flags().BoolVar(&ideNoOpen, "no-open", false, "Print the Astro IDE URL without opening the browser")
	return cmd
}

func listIDEProjects(out io.Writer) error {
	list, err := ide.List(astroV1Alpha1Client)
	if err != nil {
		return err
	}
	return emitIDEProjects(cliout.Renderer{Format: ideProjectOutput, Out: out}, list)
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

	res, err := ide.ImportProject(cmd.Context(), astroV1Alpha1Client, astroIDEExporter, ideProjectID, ideSessionID, orgID, wsID, ideImportYes, cliout.NotesTo(cmd, ideProjectOutput, out))
	if err != nil {
		return err
	}
	return emitIDEImport(cliout.Renderer{Format: ideProjectOutput, Out: out, Err: cmd.ErrOrStderr()}, res)
}

func exportProject(cmd *cobra.Command, out io.Writer) error {
	format := ideProjectOutput
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	orgID, wsID, err := validateWorkspaceAndOrgID(&ctx)
	if err != nil {
		return err
	}

	res, err := ide.ExportProject(astroV1Alpha1Client, astroV1Client, ideProjectID, orgID, wsID, force, cliout.NotesTo(cmd, format, out))
	if err != nil {
		return err
	}
	if err := emitIDEExport(cliout.Renderer{Format: format, Out: out, Err: cmd.ErrOrStderr()}, res); err != nil {
		return err
	}
	// A person at a terminal is shown the project in a browser; a run under
	// json reads its url from the result instead. The url is printed on
	// stderr either way (--no-open, CI and SSH included), so stdout stays the
	// result.
	if format == cliout.FormatText {
		ide.OpenInBrowser(res.URL, !ideNoOpen && input.CanOpenBrowser(), os.Stderr)
	}
	return nil
}

func validateWorkspaceAndOrgID(ctx *config.Context) (orgID, wsID string, err error) {
	orgID = ctx.Organization
	if orgID == "" {
		return "", "", errors.New("no Organization ID provided and none set in context; set a context or pass the Organization ID")
	}

	t := astrosession.Resolve(astrosession.Request{
		Login:   ctx,
		NoSaved: errors.New("no Workspace ID provided and none set in context; set a context or pass the Workspace ID"),
	})
	if err := t.Require(); err != nil {
		return "", "", err
	}
	wsID = t.Workspace

	return orgID, wsID, nil
}
