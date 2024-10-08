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

	"github.com/astronomer/astro-cli/pkg/httputil"
)

const (
	maxExtractSize = 100 << 20
)

type Client interface {
	DownloadAndExtractTemplate(repoURL, branch, templateDir, destDir string) error
}

type HTTPAstroTemplateClient struct {
	*httputil.HTTPClient
}

func NewHTTPAstroTemplateClient(client *httputil.HTTPClient) *HTTPAstroTemplateClient {
	return &HTTPAstroTemplateClient{
		client,
	}
}

func (c *HTTPAstroTemplateClient) DownloadAndExtractTemplate(repoURL, templateDir, destDir string) error {
	tarballURL := fmt.Sprintf("%s/tarball/main", repoURL)

	doOpts := &httputil.DoOptions{
		Path:   tarballURL,
		Method: http.MethodGet,
	}

	resp, err := c.Do(doOpts)
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
	err = extractTarGz(resp.Body, tempDir, templateDir)
	if err != nil {
		return fmt.Errorf("failed to extract tarball: %w", err)
	}

	// Copy the extracted template directory to the destination directory
	srcDir := filepath.Join(tempDir, templateDir)

	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("template directory %s not found", templateDir)
	}
	if err := copyDir(srcDir, destDir); err != nil {
		return fmt.Errorf("failed to copy template directory: %w", err)
	}

	return nil
}

func extractTarGz(r io.Reader, dest, templateDir string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	tarReader := tar.NewReader(gr)
	var baseDir string

	limitTarReader := io.LimitReader(tarReader, maxExtractSize)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tarball: %w", err)
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
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, limitTarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to copy file contents: %w", err)
			}

			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to close file: %w", err)
			}
		}
	}
	return nil
}

func copyDir(src, dst string) error {
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
