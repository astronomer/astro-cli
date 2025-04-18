package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/polaris"
	"github.com/astronomer/astro-cli/context"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	polarisProjectName string
	polarisDescription string
	polarisVisibility  string
	projectID          string
	sessionID          string
)

// NewPolarisCmd returns a new cobra command for managing Polaris
func NewPolarisCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "polaris",
		Short: "Manage Polaris resources",
		Long:  "Create and manage Polaris resources.",
	}
	cmd.AddCommand(NewPolarisProjectCmd(out))
	return cmd
}

// NewPolarisProjectCmd returns a new cobra command for managing Polaris projects
func NewPolarisProjectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"pr", "projects"},
		Short:   "Manage Polaris projects",
		Long:    "Create and manage Polaris projects in your workspace.",
	}
	cmd.AddCommand(NewPolarisProjectCreateCmd(out))
	cmd.AddCommand(NewPolarisProjectListCmd(out))
	cmd.AddCommand(NewPolarisProjectExportCmd(out))
	cmd.AddCommand(NewPolarisProjectImportCmd(out))
	return cmd
}

// NewPolarisProjectCreateCmd returns a new cobra command for creating Polaris projects
func NewPolarisProjectCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   "Create a new Polaris project",
		Long:    "Create a new Polaris project in your workspace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createPolarisProject(cmd, out)
		},
		Example: `
# Create a new Polaris project
astro polaris project create --workspace-id <workspace-id> --organization-id <organization-id> --name <project-name>

# Create a new Polaris project with description and visibility
astro polaris project create --workspace-id <workspace-id> --organization-id <organization-id> --name <project-name> --description "My project description" --visibility private
`,
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "Workspace ID for the project")
	cmd.Flags().StringVarP(&organizationID, "organization-id", "o", "", "Organization ID for the project")
	cmd.Flags().StringVarP(&polarisProjectName, "name", "n", "", "Name of the project")
	cmd.Flags().StringVarP(&polarisDescription, "description", "d", "", "Description of the project")
	cmd.Flags().StringVar(&polarisVisibility, "visibility", "PRIVATE", "Visibility of the project (PRIVATE or WORKSPACE)")
	return cmd
}

// NewPolarisProjectListCmd returns a new cobra command for listing Polaris projects
func NewPolarisProjectListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Polaris projects in your workspace",
		Long:    "List all Polaris projects in your workspace and optionally select one for future commands.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listPolarisProjects(cmd, out)
		},
		Example: `
# List all Polaris projects in your workspace
astro polaris project list

# List all Polaris projects in a specific workspace and organization
astro polaris project list --workspace-id <workspace-id> --organization-id <organization-id>

# List and select a project for future commands
astro polaris project list --select
`,
	}
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "Workspace ID to list projects from")
	cmd.Flags().StringVarP(&organizationID, "organization-id", "o", "", "Organization ID to list projects from")
	return cmd
}

// NewPolarisProjectImportCmd returns a new cobra command for importing Polaris projects
func NewPolarisProjectImportCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import",
		Aliases: []string{"i"},
		Short:   "Import a Polaris project",
		Long:    "Import a Polaris project from a tar.gz file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return importPolarisProject(cmd, out)
		},
		Example: `
# Import an Astro project from the Astro-IDE project in your workspace
astro polaris project import

# Import a Polaris project from a specific Astro-IDE project
astro polaris project import --project-id <project-id> --session-id <session-id> --workspace-id <workspace-id> --organization-id <organization-id>
`,
	}
	cmd.Flags().StringVarP(&projectID, "project-id", "p", "", "Project ID to import")
	cmd.Flags().StringVarP(&sessionID, "session-id", "s", "", "Session ID to import")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "Workspace ID for the project")
	cmd.Flags().StringVarP(&organizationID, "organization-id", "o", "", "Organization ID for the project")
	return cmd
}

// NewPolarisProjectExportCmd returns a new cobra command for exporting Polaris projects
func NewPolarisProjectExportCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export",
		Aliases: []string{"e"},
		Short:   "Export a Polaris project",
		Long:    "Export a Polaris project to a tar.gz file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return exportPolarisProject(cmd, out)
		},
		Example: `
# Export an Astro project from the Astro-IDE project in your workspace
astro polaris project export

# Export an Astro project from a specific Astro-IDE project
astro polaris project export --project-id <project-id> --workspace-id <workspace-id> --organization-id <organization-id>
`,
	}
	cmd.Flags().StringVarP(&projectID, "project-id", "p", "", "Project ID to export")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "Workspace ID for the project")
	cmd.Flags().StringVarP(&organizationID, "organization-id", "o", "", "Organization ID for the project")
	return cmd
}

func createPolarisProject(cmd *cobra.Command, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if organizationID == "" {
		organizationID = ctx.Organization
	}

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	// Validate visibility
	if polarisVisibility != "PRIVATE" && polarisVisibility != "WORKSPACE" {
		return errors.New("visibility must be either 'PRIVATE' or 'WORKSPACE'")
	}

	cmd.SilenceUsage = true
	return polaris.CreatePolarisProject(polarisCoreClient, organizationID, workspaceID, polarisProjectName, polarisDescription, polarisVisibility, out)
}

func listPolarisProjects(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return polaris.List(polarisCoreClient, out)
}

func exportPolarisProject(cmd *cobra.Command, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if organizationID == "" {
		organizationID = ctx.Organization
	}

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	cmd.SilenceUsage = true
	return polaris.ExportProject(polarisCoreClient, projectID, workspaceID, organizationID, out)
}

func importPolarisProject(cmd *cobra.Command, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if organizationID == "" {
		organizationID = ctx.Organization
	}

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	cmd.SilenceUsage = true
	return polaris.ImportProject(polarisCoreClient, projectID, sessionID, organizationID, workspaceID, out)
}
