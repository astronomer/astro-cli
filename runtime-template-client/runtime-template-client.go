package runtimetemplateclient

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Client interface {
	DownloadAndExtractTemplate(repoURL, branch, templateDir, destDir string) error
}

type HTTPAstroTemplateClient struct {
	*http.Client
}

func NewHTTPAstroTemplateClient(client *http.Client) *HTTPAstroTemplateClient {
	return &HTTPAstroTemplateClient{
		client,
	}
}

func (c *HTTPAstroTemplateClient) DownloadAndExtractTemplate(repoURL, templateDir, destDir string) error {
	tarballURL := fmt.Sprintf("%s/tarball/main", repoURL)

	resp, err := http.Get(tarballURL)
	if err != nil {
		return fmt.Errorf("failed to download tarball: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download tarball, status code: %d", resp.StatusCode)
	}

	tempDir, err := os.MkdirTemp("", "extracted-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract the tarball to the temporary directory
	var extractedBaseDir string
	if extractedBaseDir, err := extractTarGz(resp.Body, tempDir, templateDir); err != nil {
		return fmt.Errorf("failed to extract tarball: %w %s", err, extractedBaseDir)
	}

	// Copy the extracted template directory to the destination directory
	srcDir := filepath.Join(tempDir, extractedBaseDir, templateDir)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("template directory %s not found", templateDir)
	}
	if err := copyDir(srcDir, destDir); err != nil {
		return fmt.Errorf("failed to copy template directory: %w", err)
	}

	return nil
}

func extractTarGz(r io.Reader, dest string, templateDir string) (string, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	tarReader := tar.NewReader(gr)
	var baseDir string

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tarball: %w", err)
		}

		// Skip over irrelevant files like pax_global_header
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		// Extract the base directory from the first valid header
		if baseDir == "" {
			parts := strings.Split(header.Name, "/")
			if len(parts) > 1 {
				baseDir = parts[0]
			}
		}

		// Skip files that are not part of the desired template directory
		if !strings.Contains(header.Name, fmt.Sprintf("/%s/", templateDir)) {
			continue
		}

		relativePath := strings.TrimPrefix(header.Name, baseDir+"/")
		targetPath := filepath.Join(dest, relativePath)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
				return "", fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			outFile, err := os.Create(targetPath)
			if err != nil {
				return "", fmt.Errorf("failed to create file: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return "", fmt.Errorf("failed to copy file contents: %w", err)
			}
		}
	}
	return baseDir, nil
}

func copyDir(src string, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	dir, err := os.Open(src)
	if err != nil {
		return err
	}
	defer dir.Close()

	objects, err := dir.Readdir(-1)
	if err != nil {
		return err
	}
	for _, obj := range objects {
		srcFilePath := filepath.Join(src, obj.Name())
		dstFilePath := filepath.Join(dst, obj.Name())

		if obj.IsDir() {
			if err := copyDir(srcFilePath, dstFilePath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcFilePath, dstFilePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
