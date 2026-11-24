package ide

import (
	"archive/tar"
	"compress/gzip"
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/pkg/browser"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	errNoWorkspace             = errors.New("no Workspace set in context; switch to one with astro workspace switch")
	ErrInvalidProjectSelection = errors.New("invalid project selection")
	ErrNoProjectsFound         = errors.New("no Astro IDE projects found in the Workspace")
	// DefaultDirPerm is the default permission for directories
	DefaultDirPerm           os.FileMode = 0o755
	openURL                              = browser.OpenURL
	gitignoreParseWarningMsg             = "Could not parse .gitignore (%v); continuing without its filtering"
)

// The functions here that note something along the way take notes, the
// writer those go to: stdout in text, as they always have, and stderr under
// --output json, where stdout carries only the result. A question (its
// intro, its picker and its prompt) is always on stderr, in text too, and
// under json it is refused instead.

// List returns the current Workspace's Astro IDE projects, by name.
func List(client astrov1alpha1.APIClient) (*ProjectList, error) {
	projects, err := ListProjects(client)
	if err != nil {
		return nil, err
	}
	list := &ProjectList{Projects: make([]Project, 0, len(projects))}
	for i := range projects {
		list.Projects = append(list.Projects, projectOf(&projects[i]))
	}
	return list, nil
}

func ListProjects(client astrov1alpha1.APIClient) ([]astrov1alpha1.AstroIdeProject, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrov1alpha1.AstroIdeProject{}, err
	}
	run := astrosession.Resolve(astrosession.Request{Login: &ctx, NoSaved: errNoWorkspace})
	if err := run.Require(); err != nil {
		return []astrov1alpha1.AstroIdeProject{}, err
	}

	sorts := []astrov1alpha1.ListAstroIdeProjectsParamsSorts{"name:asc"}
	limit := 1000
	workspaceListParams := &astrov1alpha1.ListAstroIdeProjectsParams{
		Limit: &limit,
		Sorts: &sorts,
	}

	resp, err := client.ListAstroIdeProjectsWithResponse(httpContext.Background(), ctx.Organization, run.Workspace, workspaceListParams)
	if err != nil {
		return []astrov1alpha1.AstroIdeProject{}, err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrov1alpha1.AstroIdeProject{}, err
	}
	if resp.JSON200 == nil {
		return []astrov1alpha1.AstroIdeProject{}, errors.New("the API did not return the Astro IDE projects")
	}

	return resp.JSON200.Projects, nil
}

func selectIDEProject(projects []astrov1alpha1.AstroIdeProject, notes io.Writer) (astrov1alpha1.AstroIdeProject, error) {
	if len(projects) == 0 {
		return astrov1alpha1.AstroIdeProject{}, ErrNoProjectsFound
	}

	if len(projects) == 1 {
		fmt.Fprintln(notes, "Only one project found. Using it by default:\n"+
			fmt.Sprintf("\n Project Name: %s", ansi.Bold(projects[0].Name))+
			fmt.Sprintf("\n Project ID: %s\n", ansi.Bold(projects[0].Id)))

		return projects[0], nil
	}

	list := picker.List{
		Title:   "\nSelect a project:",
		Header:  []string{"PROJECT NAME", "ID"},
		Ask:     []input.Option{input.About("a project"), input.AnsweredBy("--project-id")},
		Invalid: ErrInvalidProjectSelection,
	}
	for i := range projects {
		list.AddRow(false, projects[i].Name, projects[i].Id)
	}
	i, err := list.Pick(os.Stderr, os.Stdin)
	if err != nil {
		return astrov1alpha1.AstroIdeProject{}, err
	}
	return projects[i], nil
}

// createNewProject asks for a name, creates a project by it and returns its ID
func createNewProject(client astrov1alpha1.APIClient, v1Client astrov1.APIClient, organizationID, workspaceID string, notes io.Writer) (string, error) {
	about := input.About("a name for the new project")
	if err := input.MayAsk("\n> ", about); err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stderr, "Enter project name:")
	name, err := input.Text("\n> ", about)
	if err != nil {
		return "", err
	}

	req := astrov1alpha1.CreateAstroIdeProjectRequest{
		Name: &name,
	}

	resp, err := client.CreateAstroIdeProjectWithResponse(httpContext.Background(), organizationID, workspaceID, req)
	if err != nil {
		return "", fmt.Errorf("could not create the project: %w", err)
	}

	if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return "", err
	}
	if resp.JSON200 == nil {
		return "", errors.New("could not create the project: the API did not return it")
	}

	workspaceName, err := getWorkspaceName(v1Client, organizationID, workspaceID)
	if err != nil {
		workspaceName = workspaceID
	}
	fmt.Fprintf(notes, "Created project %s in Workspace %s\n", input.Cell(name), input.Cell(workspaceName))
	return resp.JSON200.Id, nil
}

// getWorkspaceName resolves a workspace's display name via the v1 public API.
// The IDE client (v1alpha1) does not expose workspace reads, so the v1 client is used here.
func getWorkspaceName(v1Client astrov1.APIClient, organizationID, workspaceID string) (string, error) {
	resp, err := v1Client.GetWorkspaceWithResponse(httpContext.Background(), organizationID, workspaceID)
	if err != nil {
		return "", err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return "", err
	}
	return resp.JSON200.Name, nil
}

// createSession creates a new session with the specified permission
func createSession(client astrov1alpha1.APIClient, organizationID, workspaceID, projectID string) (*astrov1alpha1.CreateAstroIdeSessionResponse, error) {
	sessionResp, err := client.CreateAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, astrov1alpha1.CreateAstroIdeSessionJSONRequestBody{})
	if err != nil {
		return nil, err
	}
	return sessionResp, checkSession(sessionResp)
}

func createSessionWithPermission(client astrov1alpha1.APIClient, organizationID, workspaceID, projectID string, permission astrov1alpha1.CreateAstroIdeSessionRequestPermission) (*astrov1alpha1.CreateAstroIdeSessionResponse, error) {
	sessionResp, err := client.CreateAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, astrov1alpha1.CreateAstroIdeSessionJSONRequestBody{
		Permission: &permission,
	})
	if err != nil {
		return nil, err
	}
	return sessionResp, checkSession(sessionResp)
}

// checkSession fails a session create the API refused, or answered with no
// session.
func checkSession(resp *astrov1alpha1.CreateAstroIdeSessionResponse) error {
	if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return errors.New("the API did not return the Astro IDE session")
	}
	return nil
}

// getProject retrieves project details by ID
func getProject(client astrov1alpha1.APIClient, organizationID, workspaceID, projectID string) (*astrov1alpha1.GetAstroIdeProjectResponse, error) {
	projectResp, err := client.GetAstroIdeProjectWithResponse(httpContext.Background(), organizationID, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := astrov1alpha1.NormalizeAPIError(projectResp.HTTPResponse, projectResp.Body); err != nil {
		return nil, err
	}
	if projectResp.JSON200 == nil {
		return nil, errors.New("the API did not return the Astro IDE project")
	}
	return projectResp, nil
}

// updateSessionPermission updates the session permission
func updateSessionPermission(client astrov1alpha1.APIClient, organizationID, workspaceID, projectID, sessionID string, permission astrov1alpha1.UpdateAstroIdeSessionRequestPermission) error {
	updateResp, err := client.UpdateAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, astrov1alpha1.UpdateAstroIdeSessionJSONRequestBody{
		Permission: permission,
	})
	if err != nil {
		return err
	}
	return astrov1alpha1.NormalizeAPIError(updateResp.HTTPResponse, updateResp.Body)
}

// uploadAndImportArchive handles archive upload and import logic
func importArchiveToIde(client astrov1alpha1.APIClient, organizationID, workspaceID, projectID, sessionID, archivePath string) error {
	// Upload the archive
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Import the package
	mode := astrov1alpha1.ImportAstroIdeSessionTarParamsModeOVERWRITE
	importParams := &astrov1alpha1.ImportAstroIdeSessionTarParams{
		Mode: &mode,
	}
	importResp, err := client.ImportAstroIdeSessionTarWithBodyWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, importParams, "application/gzip", file)
	if err != nil {
		return err
	}
	return astrov1alpha1.NormalizeAPIError(importResp.HTTPResponse, importResp.Body)
}

// saveSessionAndCleanup handles session saving and cleanup
func saveSessionAndCleanup(client astrov1alpha1.APIClient, organizationID, workspaceID, projectID, sessionID string) error {
	// Save the session
	saveResp, err := client.SaveAstroIdeSessionWithResponse(httpContext.Background(), organizationID, workspaceID, projectID, sessionID, astrov1alpha1.SaveAstroIdeSessionJSONRequestBody{
		Message: "Imported from Astro CLI",
	})
	if err != nil {
		return err
	}
	if err := astrov1alpha1.NormalizeAPIError(saveResp.HTTPResponse, saveResp.Body); err != nil {
		return err
	}

	return updateSessionPermission(client, organizationID, workspaceID, projectID, sessionID, astrov1alpha1.UpdateAstroIdeSessionRequestPermissionREADONLY)
}

// OpenInBrowser shows an exported project's URL, on out, and opens it in the
// default browser when open says to. It does nothing for a project with no
// URL.
func OpenInBrowser(url string, open bool, out io.Writer) {
	if url == "" {
		return
	}
	switch {
	case !open:
		fmt.Fprintf(out, "Open the project at %s\n", url)
	case openURL(url) != nil:
		fmt.Fprintf(out, "Could not open the browser. Open this link: %s\n", url)
	default:
		fmt.Fprintf(out, "Opened the project in your browser: %s\n", url)
	}
}

// resolveProjectID handles project creation or selection when projectID is
// not provided. It reports whether it created the project.
func resolveProjectID(client astrov1alpha1.APIClient, v1Client astrov1.APIClient, projectID, organizationID, workspaceID string, force bool, notes io.Writer) (id string, created bool, err error) {
	// Handle project creation or selection
	if projectID == "" && !force {
		ask := []input.Option{input.About("whether to create a new project"), input.AnsweredBy("--project-id")}
		if err := input.MayAsk("\n> ", ask...); err != nil {
			return "", false, err
		}
		fmt.Fprintln(os.Stderr, "Create a new project? (y/n)")
		choice, err := input.Text("\n> ", ask...)
		if err != nil {
			return "", false, err
		}
		if choice == "y" || choice == "Y" {
			id, err := createNewProject(client, v1Client, organizationID, workspaceID, notes)
			return id, err == nil, err
		}
	}

	// Select from existing projects if needed
	if projectID == "" {
		projects, err := ListProjects(client)
		if err != nil {
			return "", false, err
		}
		selectedProject, err := selectIDEProject(projects, notes)
		if err != nil {
			return "", false, err
		}
		return selectedProject.Id, false, nil
	}

	return projectID, false, nil
}

// handleProjectLock checks for project locks and handles permission upgrades
func handleProjectLock(client astrov1alpha1.APIClient, sessionResp *astrov1alpha1.CreateAstroIdeSessionResponse, organizationID, workspaceID, projectID string, force bool) (*astrov1alpha1.CreateAstroIdeSessionResponse, error) {
	if sessionResp.JSON200.Permission != "READ_ONLY" {
		return sessionResp, nil
	}

	if !force {
		// Get project details to show who owns the lock
		projectResp, err := getProject(client, organizationID, workspaceID, projectID)
		if err != nil {
			return nil, fmt.Errorf("could not get the project details: %w", err)
		}

		// Show project lock information and instructions
		if projectResp.JSON200.Lock != nil && projectResp.JSON200.Lock.Subject.FullName != nil {
			lastEditedAt := projectResp.JSON200.Lock.LastEditedAt
			if parsedTime, err := time.Parse(time.RFC3339, lastEditedAt); err == nil {
				lastEditedAt = parsedTime.Format("January 2, 2006 at 3:04 PM")
			}
			return nil, fmt.Errorf("project is locked by user %s, last edited at %s; use --force to overwrite the lock", *projectResp.JSON200.Lock.Subject.FullName, lastEditedAt)
		}
		return nil, fmt.Errorf("project is locked; use --force to overwrite the lock")
	}

	// Create a new session with READWRITE permission
	return createSessionWithPermission(client, organizationID, workspaceID, projectID, astrov1alpha1.CreateAstroIdeSessionRequestPermissionREADWRITE)
}

// ExportProject exports the project in the current directory to Astro IDE,
// and returns what it exported. It neither prints its result nor opens the
// project: the caller does (see OpenInBrowser).
func ExportProject(client astrov1alpha1.APIClient, v1Client astrov1.APIClient, projectID, organizationID, workspaceID string, force bool, notes io.Writer) (res *Export, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not read the current directory: %w", err)
	}

	// Archive the directory first: a local failure then leaves nothing
	// behind in the Astro IDE, a project this run would create included.
	tempDir, err := os.MkdirTemp("", "astro-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir) //nolint:errcheck // best-effort cleanup

	archivePath := filepath.Join(tempDir, "project.tar.gz")
	stats, err := createTarGzArchive(".", archivePath, notes)
	if err != nil {
		return nil, err
	}

	// Resolve project ID (create or select)
	projectID, created, err := resolveProjectID(client, v1Client, projectID, organizationID, workspaceID, force, notes)
	if err != nil {
		return nil, err
	}
	// A project this run created outlives a failure after it, so the
	// failure names it: the caller would otherwise never learn it exists.
	defer func() {
		if err != nil && created {
			err = fmt.Errorf("created the Astro IDE project %s, but the export to it failed: %w", projectID, err)
		}
	}()

	// Create session and handle permissions
	sessionResp, err := createSession(client, organizationID, workspaceID, projectID)
	if err != nil {
		return nil, err
	}

	// Handle project lock and permission upgrades
	sessionResp, err = handleProjectLock(client, sessionResp, organizationID, workspaceID, projectID, force)
	if err != nil {
		return nil, err
	}

	// Upload and import archive
	if err := importArchiveToIde(client, organizationID, workspaceID, projectID, sessionResp.JSON200.Id, archivePath); err != nil {
		return nil, err
	}

	// Save session and cleanup
	if err := saveSessionAndCleanup(client, organizationID, workspaceID, projectID, sessionResp.JSON200.Id); err != nil {
		return nil, err
	}

	res = &Export{
		ProjectID:      projectID,
		ProjectCreated: created,
		Directory:      dir,
		Files:          stats.files,
		Bytes:          stats.bytes,
		Action:         ActionExported,
	}
	// The project's name and URL, read back; the export has happened either way.
	if projectResp, err := getProject(client, organizationID, workspaceID, projectID); err == nil {
		res.ProjectName = projectResp.JSON200.Name
		if projectResp.JSON200.Url != nil {
			res.URL = *projectResp.JSON200.Url
		}
	}
	return res, nil
}

// createTarGzArchive creates a tar.gz archive of the given directory, and
// counts the regular files it holds.
func createTarGzArchive(sourceDir, targetFile string, notes io.Writer) (stats archiveStats, err error) {
	matcher := gitignoreMatcher(sourceDir, notes)

	// Create the target file
	target, err := os.Create(targetFile)
	if err != nil {
		return stats, err
	}
	// The archive is only whole once the tar, the gzip and the file have
	// each flushed and closed, so a failure to do so fails the archive.
	gzipWriter := gzip.NewWriter(target)
	tarWriter := tar.NewWriter(gzipWriter)
	defer func() {
		err = errors.Join(err, tarWriter.Close(), gzipWriter.Close(), target.Close())
	}()

	// Walk through the source directory
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the target file itself
		if path == targetFile {
			return nil
		}

		// Determine relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Skip root itself; we don't archive a top-level "." entry
		if relPath == "." {
			return nil
		}

		if shouldSkipArchiveEntry(relPath, info, matcher) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Create a header for the file/dir
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		// Tar names use forward slashes on every platform; on Windows
		// filepath.Rel returns backslashes, which the IDE would store as
		// literal characters in a top-level file name.
		header.Name = filepath.ToSlash(relPath)

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

			n, err := io.Copy(tarWriter, file)
			if err != nil {
				return err
			}
			stats.files++
			stats.bytes += n
		}

		return nil
	})
	return stats, err
}

// gitignoreMatcher creates a matcher from .gitignore patterns in the given root.
func gitignoreMatcher(root string, notes io.Writer) gitignore.Matcher {
	fs := osfs.New(root)
	patterns, err := gitignore.ReadPatterns(fs, []string{})
	if err != nil {
		ansi.Warn(notes, fmt.Sprintf(gitignoreParseWarningMsg, err))
		return gitignore.NewMatcher(nil)
	}
	return gitignore.NewMatcher(patterns)
}

// localFiles matches the per-machine files Astro tools write into a project
// (airflowrt.LocalFileRules): a standalone Airflow's database, logs and admin
// password, local instance files, Desktop's MCP token. They are true of this
// machine only, so an export leaves them out whatever the project's own
// .gitignore says, as a generated image does.
var localFiles = localFileMatcher()

func localFileMatcher() gitignore.Matcher {
	rules := airflowrt.LocalFileRules()
	patterns := make([]gitignore.Pattern, 0, len(rules))
	for _, rule := range rules {
		patterns = append(patterns, gitignore.ParsePattern(rule, nil))
	}
	return gitignore.NewMatcher(patterns)
}

// inGitDir reports whether relPath is the project's .git directory or under
// it, as goos's filesystem names it: on macOS and Windows, which ignore case
// (and on Windows trailing dots and spaces), .GIT is that directory too. It
// is the rule an import refuses git metadata by (foldFor), so an export never
// carries what an import of it would refuse.
func inGitDir(relPath, goos string) bool {
	first, _, _ := strings.Cut(relPath, string(filepath.Separator))
	return foldFor(goos)(first) == ".git"
}

// shouldSkipArchiveEntry determines whether a path should be skipped.
func shouldSkipArchiveEntry(relPath string, info os.FileInfo, matcher gitignore.Matcher) bool {
	// Always skip the .git directory
	if inGitDir(relPath, runtime.GOOS) {
		return true
	}

	pathParts := strings.Split(relPath, string(filepath.Separator))

	// Never a per-machine file, even under .astro, which is otherwise exported
	// whatever the .gitignore says
	if localFiles.Match(pathParts, info.IsDir()) {
		return true
	}

	// Never a temporary file an interrupted import left behind
	base := filepath.Base(relPath)
	if info.Mode().IsRegular() && isImportTemp(base) {
		return true
	}

	// Include .astro directory, gitignore and dockerignore files
	includeAllowed := relPath == ".astro" || strings.HasPrefix(relPath, ".astro"+string(filepath.Separator)) || relPath == ".gitignore" || relPath == ".dockerignore" || base == ".airflowignore"
	if includeAllowed {
		return false
	}

	// Respect .gitignore for everything else
	return matcher.Match(pathParts, info.IsDir())
}

// ImportProject imports a project from Astro IDE into the current directory,
// and returns what it imported. A directory that is not empty is confirmed
// first, unless yes.
func ImportProject(ctx httpContext.Context, client astrov1alpha1.APIClient, exporter SessionExporter, projectID, sessionID, organizationID, workspaceID string, yes bool, notes io.Writer) (*Import, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not read the current directory: %w", err)
	}
	// Validate current directory is empty
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("could not read the current directory: %w", err)
	}
	if len(entries) > 0 && !yes {
		if err := input.ConfirmOrAbort(fmt.Sprintf("Current directory is not empty. Import the project here? %s", dir), input.AnsweredBy("--yes")); err != nil {
			return nil, err
		}
	}

	// If projectID is not provided, select one
	if projectID == "" {
		projects, err := ListProjects(client)
		if err != nil {
			return nil, err
		}
		selectedProject, err := selectIDEProject(projects, notes)
		if err != nil {
			return nil, err
		}
		projectID = selectedProject.Id
	}

	if sessionID == "" {
		// Create a new session with READ_ONLY permission.
		sessionResp, err := createSessionWithPermission(client, organizationID, workspaceID, projectID, astrov1alpha1.CreateAstroIdeSessionRequestPermissionREADONLY)
		if err != nil {
			return nil, err
		}
		sessionID = sessionResp.JSON200.Id
	}

	// Create a temporary file for the archive
	tempFile, err := os.CreateTemp("", "astro-export-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("could not create a temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name()) //nolint:errcheck // best-effort cleanup
	defer tempFile.Close()

	// Export the project, streamed into the temporary file
	if err := downloadSession(ctx, exporter, organizationID, workspaceID, projectID, sessionID, tempFile); err != nil {
		return nil, err
	}

	// Extract the archive from the same open file, so what is checked and
	// what is written are read from the one file this run wrote.
	stats, err := extractTarGzArchive(ctx, tempFile, ".")
	if err != nil {
		return nil, fmt.Errorf("could not extract the archive: %w", err)
	}

	res := &Import{
		ProjectID: projectID,
		SessionID: sessionID,
		Directory: dir,
		Files:     stats.files,
		Bytes:     stats.bytes,
		Action:    ActionImported,
	}
	// The project's name, read back; the import has happened either way.
	if projectResp, err := getProject(client, organizationID, workspaceID, projectID); err == nil {
		res.ProjectName = projectResp.JSON200.Name
	}
	return res, nil
}
