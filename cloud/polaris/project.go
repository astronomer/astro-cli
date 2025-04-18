package polaris

import (
	"archive/tar"
	"compress/gzip"
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	astropolariscore "github.com/astronomer/astro-cli/astro-client-polaris-core"
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

// List all polaris projects
func List(client astropolariscore.PolarisClient, out io.Writer) error {
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

func ListProjects(client astropolariscore.PolarisClient) ([]astropolariscore.PolarisProject, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astropolariscore.PolarisProject{}, err
	}

	sorts := []astropolariscore.ListPolarisProjectsParamsSorts{"name:asc"}
	limit := 1000
	workspaceListParams := &astropolariscore.ListPolarisProjectsParams{
		Limit: &limit,
		Sorts: &sorts,
	}

	resp, err := client.ListPolarisProjectsWithResponse(httpContext.Background(), ctx.Organization, ctx.Workspace, workspaceListParams)
	if err != nil {
		return []astropolariscore.PolarisProject{}, err
	}
	err = astropolariscore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astropolariscore.PolarisProject{}, err
	}

	projects := resp.JSON200.Projects

	return projects, nil
}

func selectPolarisProject(projects []astropolariscore.PolarisProject) (astropolariscore.PolarisProject, error) {
	if len(projects) == 0 {
		return astropolariscore.PolarisProject{}, nil
	}

	if len(projects) == 1 {
		fmt.Println("Only one Workspace was found. Using the following Workspace by default: \n" +
			fmt.Sprintf("\n Workspace Name: %s", ansi.Bold(projects[0].Name)) +
			fmt.Sprintf("\n Workspace ID: %s\n", ansi.Bold(projects[0].Id)))

		return projects[0], nil
	}

	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "PROJECTNAME", "ID"},
	}

	fmt.Println("\nPlease select the project from the list below:")

	projectMap := map[string]astropolariscore.PolarisProject{}
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
		return astropolariscore.PolarisProject{}, ErrInvalidProjectSelection
	}
	return selected, nil
}

// ExportProject exports a project from CLI to Astro IDE
func ExportProject(client astropolariscore.PolarisClient, projectID, workspaceID, organizationID string, out io.Writer) error {
	if projectID == "" {
		// Ask user if they want to create a new project
		fmt.Println("Do you want to create a new project? (y/n)")
		choice := input.Text("\n> ")
		if choice == "y" || choice == "Y" {
			// Get project details from user
			fmt.Println("Enter project name:")
			name := input.Text("\n> ")

			// Create new project
			ctx, err := context.GetCurrentContext()
			if err != nil {
				return err
			}

			// Create the project request
			req := astropolariscore.CreatePolarisProjectRequest{
				Name: &name,
			}

			// Create the project
			resp, err := client.CreatePolarisProjectWithResponse(httpContext.Background(), organizationID, workspaceID, req)
			if err != nil {
				return fmt.Errorf("failed to create project: %w", err)
			}

			if err := astropolariscore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
				return err
			}

			fmt.Fprintf(out, "Successfully created project '%s' in workspace '%s'\n", name, ctx.Workspace)

			// Get the newly created project ID
			projectID = resp.JSON200.Id
		} else {
			// List existing projects and let user select one
			projects, err := ListProjects(client)
			if err != nil {
				return err
			}
			selectedProject, err := selectPolarisProject(projects)
			if err != nil {
				return err
			}
			projectID = selectedProject.Id
		}
	}

	// Create a temporary directory for the archive
	tempDir, err := os.MkdirTemp("", "astro-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Create tar.gz archive of current directory
	archivePath := filepath.Join(tempDir, "project.tar.gz")
	if err := createTarGzArchive(".", archivePath); err != nil {
		return err
	}

	// Create a new session
	sessionResp, err := client.CreatePolarisSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID)
	if err != nil {
		return err
	}
	if err := astropolariscore.NormalizeAPIError(sessionResp.HTTPResponse, sessionResp.Body); err != nil {
		return err
	}
	session := sessionResp.JSON200

	// Open the archive file
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	// Import the package
	mode := astropolariscore.ImportPolarisSessionTarParamsModeOVERWRITE
	importParams := &astropolariscore.ImportPolarisSessionTarParams{
		Mode: &mode,
	}
	importResp, err := client.ImportPolarisSessionTarWithBodyWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, session.Id, importParams, "application/x-gzip", archiveFile)
	if err != nil {
		return err
	}
	if err := astropolariscore.NormalizeAPIError(importResp.HTTPResponse, importResp.Body); err != nil {
		return err
	}

	// Save the session
	saveResp, err := client.SavePolarisSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, session.Id, astropolariscore.SavePolarisSessionJSONRequestBody{
		Message: "Imported from Astro CLI",
	})
	if err != nil {
		return err
	}
	if err := astropolariscore.NormalizeAPIError(saveResp.HTTPResponse, saveResp.Body); err != nil {
		return err
	}

	fmt.Fprintf(out, "Successfully exported project to %s\n", projectID)
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
func ImportProject(client astropolariscore.PolarisClient, projectID, sessionID, organizationID, workspaceID string, out io.Writer) error {
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
		selectedProject, err := selectPolarisProject(projects)
		if err != nil {
			return err
		}
		projectID = selectedProject.Id
	}

	// If sessionID is not provided, create a new session
	if sessionID == "" {
		sessionResp, err := client.CreatePolarisSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID)
		if err != nil {
			return err
		}
		if err := astropolariscore.NormalizeAPIError(sessionResp.HTTPResponse, sessionResp.Body); err != nil {
			return err
		}
		sessionID = sessionResp.JSON200.Id
	}

	// Create a temporary file for the archive
	tempFile, err := os.CreateTemp("", "astro-export-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Export the project
	exportParams := &astropolariscore.ExportPolarisSessionTarParams{}
	exportResp, err := client.ExportPolarisSessionTarWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, exportParams)
	if err != nil {
		return err
	}
	if err := astropolariscore.NormalizeAPIError(exportResp.HTTPResponse, exportResp.Body); err != nil {
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

// CreatePolarisProject creates a new Polaris project
func CreatePolarisProject(client astropolariscore.PolarisClient, organizationID, workspaceID, name, description, visibility string, out io.Writer) error {
	// Get context
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	// Create the project request
	req := astropolariscore.CreatePolarisProjectRequest{
		Name:        &name,
		Description: &description,
	}

	// Set visibility if provided
	if visibility != "" {
		vis := astropolariscore.CreatePolarisProjectRequestVisibility(visibility)
		req.Visibility = &vis
	}

	// Create the project
	resp, err := client.CreatePolarisProjectWithResponse(httpContext.Background(), ctx.Organization, ctx.Workspace, req)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	if err := astropolariscore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}

	fmt.Fprintf(out, "Successfully created project '%s' in workspace '%s'\n", name, ctx.Workspace)
	return nil
}
