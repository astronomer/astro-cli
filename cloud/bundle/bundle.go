package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// BundleFileEntry represents a single file entry returned by the list endpoint.
type BundleFileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

// ListFiles lists files in a deployment bundle and prints them as a table.
func ListFiles(out io.Writer, coreClient astrocore.CoreClient, orgID, deploymentID, path string) error {
	params := &astrocore.ListBundleFilesParams{}
	if path != "" {
		params.Path = &path
	}
	resp, err := coreClient.ListBundleFilesWithResponse(context.Background(), orgID, deploymentID, params)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	var files []BundleFileEntry
	if err := json.Unmarshal(resp.Body, &files); err != nil {
		return fmt.Errorf("failed to parse file list: %w", err)
	}

	tab := &printutil.Table{
		Padding:        []int{40, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "SIZE", "TYPE"},
		NoResultsMsg:   "No files found in bundle",
	}
	for _, f := range files {
		tab.AddRow([]string{f.Name, fmt.Sprintf("%d", f.Size), f.Type}, false)
	}
	return tab.Print(out)
}

// UploadFile uploads a local file to a bundle at the given remote path.
func UploadFile(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, remotePath, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	err = streamClient.UploadBundleFile(context.Background(), orgID, deploymentID, remotePath, f, info.Size())
	if err != nil {
		return err
	}
	fmt.Printf("Successfully uploaded %s to %s\n", localPath, remotePath)
	return nil
}

// DownloadFile downloads a single file from the bundle and writes it to a local file.
func DownloadFile(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, remotePath, outputPath string, out io.Writer) error {
	body, err := streamClient.DownloadBundleFile(context.Background(), orgID, deploymentID, remotePath)
	if err != nil {
		return err
	}
	defer body.Close()

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully downloaded %s to %s\n", remotePath, outputPath)
	return nil
}

// DeleteFile deletes a file from the bundle.
func DeleteFile(coreClient astrocore.CoreClient, orgID, deploymentID, path string) error {
	resp, err := coreClient.DeleteBundleFileWithResponse(context.Background(), orgID, deploymentID, path)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully deleted %s\n", path)
	return nil
}

// MoveFile moves a file within the bundle.
func MoveFile(coreClient astrocore.CoreClient, orgID, deploymentID, sourcePath, destination string) error {
	body := astrocore.MoveBundleFileJSONRequestBody{
		Destination: destination,
	}
	resp, err := coreClient.MoveBundleFileWithResponse(context.Background(), orgID, deploymentID, sourcePath, body)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully moved %s to %s\n", sourcePath, destination)
	return nil
}

// DuplicateFile duplicates a file within the bundle.
func DuplicateFile(coreClient astrocore.CoreClient, orgID, deploymentID, sourcePath, destination string) error {
	body := astrocore.DuplicateBundleFileJSONRequestBody{
		Destination: destination,
	}
	resp, err := coreClient.DuplicateBundleFileWithResponse(context.Background(), orgID, deploymentID, sourcePath, body)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully duplicated %s to %s\n", sourcePath, destination)
	return nil
}

// Sync tars and gzips a local directory and uploads it as a bundle archive.
func Sync(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, localDir, targetPath string, overwrite bool) error {
	// Create temp dir for the archive
	tmpDir, err := os.MkdirTemp("", "astro-bundle-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "bundle.tar")
	tarGzPath := tarPath + ".gz"

	// Create tar
	err = fileutil.Tar(localDir, tarPath, false, []string{".git/"})
	if err != nil {
		return fmt.Errorf("failed to create tar: %w", err)
	}

	// Gzip the tar
	err = fileutil.GzipFile(tarPath, tarGzPath)
	if err != nil {
		return fmt.Errorf("failed to gzip tar: %w", err)
	}

	// Open and upload
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	err = streamClient.UploadBundleArchive(context.Background(), orgID, deploymentID, f, info.Size(), overwrite, targetPath)
	if err != nil {
		return err
	}
	fmt.Println("Successfully synced bundle")
	return nil
}

// UploadArchive uploads a pre-built archive file to the bundle.
func UploadArchive(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, archivePath, targetPath string, overwrite bool) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat archive file: %w", err)
	}

	err = streamClient.UploadBundleArchive(context.Background(), orgID, deploymentID, f, info.Size(), overwrite, targetPath)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully uploaded archive %s\n", archivePath)
	return nil
}

// DownloadArchive downloads the entire bundle as an archive.
func DownloadArchive(streamClient astrocore.BundleFilesStreamClient, orgID, deploymentID, outputPath string, out io.Writer) error {
	body, err := streamClient.DownloadBundleArchive(context.Background(), orgID, deploymentID)
	if err != nil {
		return err
	}
	defer body.Close()

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Successfully downloaded bundle archive to %s\n", outputPath)
	return nil
}
