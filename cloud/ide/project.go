package ide

import (
	"archive/tar"
	"compress/gzip"
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	ErrInvalidProjectSelection = errors.New("invalid project selection")
	// DefaultDirPerm is the default permission for directories
	DefaultDirPerm os.FileMode = 0o755
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

// List all IDE projects
func List(client astrocore.CoreClient, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	ws, err := ListProjects(client)
	if err != nil {
		return err
	}

	tab := newTableOut()
	for i := range ws {
		name := ws[i].Name
		workspace := ws[i].Id

		var color bool

		if c.Workspace == ws[i].Id {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, workspace}, color)
	}

	tab.Print(out)

	return nil
}

func ListProjects(client astrocore.CoreClient) ([]astrocore.AstroIdeProject, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.AstroIdeProject{}, err
	}

	sorts := []astrocore.ListAstroIdeProjectsParamsSorts{"name:asc"}
	limit := 1000
	workspaceListParams := &astrocore.ListAstroIdeProjectsParams{
		Limit: &limit,
		Sorts: &sorts,
	}

	resp, err := client.ListAstroIdeProjectsWithResponse(httpContext.Background(), ctx.Organization, ctx.Workspace, workspaceListParams)
	if err != nil {
		return []astrocore.AstroIdeProject{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrocore.AstroIdeProject{}, err
	}

	projects := resp.JSON200.Projects

	return projects, nil
}

func selectIDEProject(projects []astrocore.AstroIdeProject) (astrocore.AstroIdeProject, error) {
	if len(projects) == 0 {
		return astrocore.AstroIdeProject{}, nil
	}

	if len(projects) == 1 {
		fmt.Println("Only one Project was found. Using the following Project by default: \n" +
			fmt.Sprintf("\n Project Name: %s", ansi.Bold(projects[0].Name)) +
			fmt.Sprintf("\n Project ID: %s\n", ansi.Bold(projects[0].Id)))

		return projects[0], nil
	}

	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "PROJECTNAME", "ID"},
	}

	fmt.Println("\nPlease select the project from the list below:")

	projectMap := map[string]astrocore.AstroIdeProject{}
	for i := range projects {
		index := i + 1
		table.AddRow([]string{
			strconv.Itoa(index),
			projects[i].Name,
			projects[i].Id,
		}, false)
		projectMap[strconv.Itoa(index)] = projects[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := projectMap[choice]
	if !ok {
		return astrocore.AstroIdeProject{}, ErrInvalidProjectSelection
	}
	return selected, nil
}

// createNewProject creates a new project and returns its ID
func createNewProject(client astrocore.CoreClient, organizationID, workspaceID string, out io.Writer) (string, error) {
	fmt.Println("Enter project name:")
	name := input.Text("\n>")

	req := astrocore.CreateAstroIdeProjectRequest{
		Name: &name,
	}

	resp, err := client.CreateAstroIdeProjectWithResponse(httpContext.Background(), organizationID, workspaceID, req)
	if err != nil {
		return "", fmt.Errorf("failed to create project: %w", err)
	}

	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return "", err
	}

	fmt.Fprintf(out, "Successfully created project '%s' in workspace '%s'\n", name, workspaceID)
	return resp.JSON200.Id, nil
}

// createSession creates a new session with the specified permission
func createSession(client astrocore.CoreClient, organizationID, workspaceID, projectID string, permission astrocore.CreateAstroIdeSessionRequestPermission) (*astrocore.CreateAstroIdeSessionResponse, error) {
	sessionResp, err := client.CreateAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, astrocore.CreateAstroIdeSessionJSONRequestBody{
		Permission: &permission,
	})
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(sessionResp.HTTPResponse, sessionResp.Body); err != nil {
		return nil, err
	}
	return sessionResp, nil
}

// getProject retrieves project details by ID
func getProject(client astrocore.CoreClient, organizationID, workspaceID, projectID string) (*astrocore.GetAstroIdeProjectResponse, error) {
	projectResp, err := client.GetAstroIdeProjectWithResponse(httpContext.Background(), organizationID, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(projectResp.HTTPResponse, projectResp.Body); err != nil {
		return nil, err
	}
	return projectResp, nil
}

// updateSessionPermission updates the session permission
func updateSessionPermission(client astrocore.CoreClient, organizationID, workspaceID, projectID, sessionID string, permission astrocore.UpdateAstroIdeSessionRequestPermission) error {
	updateResp, err := client.UpdateAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, astrocore.UpdateAstroIdeSessionJSONRequestBody{
		Permission: permission,
	})
	if err != nil {
		return err
	}
	return astrocore.NormalizeAPIError(updateResp.HTTPResponse, updateResp.Body)
}

// uploadAndImportArchive handles archive upload and import logic
func importArchiveToIde(client astrocore.CoreClient, organizationID, workspaceID, projectID, sessionID, archivePath string) error {
	// Upload the archive
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Import the package
	mode := astrocore.ImportAstroIdeSessionTarParamsModeOVERWRITE
	importParams := &astrocore.ImportAstroIdeSessionTarParams{
		Mode: &mode,
	}
	importResp, err := client.ImportAstroIdeSessionTarWithBodyWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, importParams, "application/x-gzip", file)
	if err != nil {
		return err
	}
	return astrocore.NormalizeAPIError(importResp.HTTPResponse, importResp.Body)
}

// saveSessionAndCleanup handles session saving and cleanup
func saveSessionAndCleanup(client astrocore.CoreClient, organizationID, workspaceID, projectID, sessionID string) error {
	// Save the session
	saveResp, err := client.SaveAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, astrocore.SaveAstroIdeSessionJSONRequestBody{
		Message: "Imported from Astro CLI",
	})
	if err != nil {
		return err
	}
	if err := astrocore.NormalizeAPIError(saveResp.HTTPResponse, saveResp.Body); err != nil {
		return err
	}

	return updateSessionPermission(client, organizationID, workspaceID, projectID, sessionID, astrocore.UpdateAstroIdeSessionRequestPermissionREADONLY)
}

// openProjectInBrowser opens the project URL in the default browser
func openProjectInBrowser(domain, workspaceID, projectID string, out io.Writer) {
	// Construct the URL
	url := fmt.Sprintf("https://cloud.%s/%s/astro-ide/%s", domain, workspaceID, projectID)

	// Open the URL in browser
	cmd := exec.Command("open", url)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "Failed to open browser: %v\n", err)
	}
}

// ExportProject exports a project from CLI to Astro IDE
func ExportProject(client astrocore.CoreClient, projectID, organizationID, workspaceID, domain string, force bool, out io.Writer) error {
	var err error

	// Handle project creation or selection
	if projectID == "" && !force {
		fmt.Println("Do you want to create a new project? (y/n)")
		choice := input.Text("\n> ")
		if choice == "y" || choice == "Y" {
			projectID, err = createNewProject(client, organizationID, workspaceID, out)
			if err != nil {
				return err
			}
		}
	}

	// Select from existing projects if needed
	if projectID == "" {
		projects, err := ListProjects(client)
		if err != nil {
			return err
		}
		selectedProject, err := selectIDEProject(projects)
		if err != nil {
			return err
		}
		projectID = selectedProject.Id
	}

	// Create temporary directory and archive
	tempDir, err := os.MkdirTemp("", "astro-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, "project.tar.gz")
	if err := createTarGzArchive(".", archivePath); err != nil {
		return err
	}

	// Create session and handle permissions
	sessionResp, err := createSession(client, organizationID, workspaceID, projectID, astrocore.CreateAstroIdeSessionRequestPermissionREADWRITE)
	if err != nil {
		return err
	}

	if sessionResp.JSON200.Permission == "READ_ONLY" {
		if !force {
			// Get project details to show who owns the lock
			projectResp, err := getProject(client, organizationID, workspaceID, projectID)
			if err != nil {
				return fmt.Errorf("failed to get project details: %w", err)
			}

			// Show project lock information and instructions
			if projectResp.JSON200.Lock != nil && projectResp.JSON200.Lock.Subject.FullName != nil {
				return fmt.Errorf("project is locked by user '%s'. Use --force flag to overwrite the existing project lock", *projectResp.JSON200.Lock.Subject.FullName)
			}
			return fmt.Errorf("project is locked. Use --force flag to overwrite the existing project lock")
		}

		// Create a new session with READWRITE permission
		sessionResp, err = createSession(client, organizationID, workspaceID, projectID, astrocore.CreateAstroIdeSessionRequestPermissionREADWRITE)
		if err != nil {
			return err
		}
	}

	// Upload and import archive
	if err := importArchiveToIde(client, organizationID, workspaceID, projectID, sessionResp.JSON200.Id, archivePath); err != nil {
		return err
	}

	// Save session and cleanup
	if err := saveSessionAndCleanup(client, organizationID, workspaceID, projectID, sessionResp.JSON200.Id); err != nil {
		return err
	}

	fmt.Fprintf(out, "Successfully exported project to %s\n", projectID)

	// Open project in browser
	openProjectInBrowser(domain, workspaceID, projectID, out)

	return nil
}

// createTarGzArchive creates a tar.gz archive of the given directory
func createTarGzArchive(sourceDir, targetFile string) error {
	// Create the target file
	target, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer target.Close()

	// Create a gzip writer
	gzipWriter := gzip.NewWriter(target)
	defer gzipWriter.Close()

	// Create a tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk through the source directory
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the target file itself
		if path == targetFile {
			return nil
		}

		// Create a header for the file
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Set the relative path in the archive
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write the header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(tarWriter, file)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// ImportProject imports a project from Astro IDE to the local directory
func ImportProject(client astrocore.CoreClient, projectID, organizationID, workspaceID string, out io.Writer) error {
	// Validate current directory is empty
	entries, err := os.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read current directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("current directory is not empty. Please run this command in an empty directory")
	}

	// If projectID is not provided, select one
	if projectID == "" {
		projects, err := ListProjects(client)
		if err != nil {
			return err
		}
		selectedProject, err := selectIDEProject(projects)
		if err != nil {
			return err
		}
		projectID = selectedProject.Id
	}

	// Create a new session with READ_ONLY permission
	sessionResp, err := createSession(client, organizationID, workspaceID, projectID, astrocore.CreateAstroIdeSessionRequestPermissionREADONLY)
	if err != nil {
		return err
	}
	sessionID := sessionResp.JSON200.Id

	// Create a temporary file for the archive
	tempFile, err := os.CreateTemp("", "astro-export-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Export the project
	exportParams := &astrocore.ExportAstroIdeSessionTarParams{}
	exportResp, err := client.ExportAstroIdeSessionTarWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, exportParams)
	if err != nil {
		return err
	}
	if err := astrocore.NormalizeAPIError(exportResp.HTTPResponse, exportResp.Body); err != nil {
		return err
	}

	// Write the response body to the temporary file
	if _, err := tempFile.Write(exportResp.Body); err != nil {
		return fmt.Errorf("failed to write archive to temporary file: %w", err)
	}

	// Close the file before extracting
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Extract the archive
	if err := extractTarGzArchive(tempFile.Name(), "."); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	fmt.Fprintf(out, "Successfully exported project from %s\n", projectID)
	return nil
}

// extractTarGzArchive extracts a tar.gz archive to the target directory
func extractTarGzArchive(archivePath, targetDir string) error {
	// Open the archive file
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	// Create a gzip reader
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	// Create a tar reader
	tarReader := tar.NewReader(gzipReader)

	// Extract each file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Create the target path
		targetPath := filepath.Join(targetDir, header.Name) //nolint

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(targetPath), DefaultDirPerm); err != nil {
			return err
		}

		// Handle different types of files
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(targetPath, DefaultDirPerm); err != nil {
				return err
			}
		case tar.TypeReg:
			// Create file
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode)) //nolint
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tarReader); err != nil { //nolint
				file.Close()
				return err
			}
			file.Close()
		}
	}

	return nil
}
