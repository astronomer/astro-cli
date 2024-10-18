package airflow

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	maxExtractSize = 100 << 20
)

var (
	astroTemplateRepoURL = "https://github.com/astronomer/templates"
	runtimeTemplateURL   = "https://updates.astronomer.io/astronomer-templates"
)

type Template struct {
	Name string
}

type TemplatesResponse struct {
	Templates []Template
}

func FetchTemplateList() ([]string, error) {
	HTTPClient := &httputil.HTTPClient{}
	doOpts := &httputil.DoOptions{
		Path:   runtimeTemplateURL,
		Method: http.MethodGet,
	}

	resp, err := HTTPClient.Do(doOpts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var templatesResponse TemplatesResponse
	err = json.Unmarshal(body, &templatesResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	uniqueTemplates := make(map[string]struct{})
	var templateNames []string

	for _, template := range templatesResponse.Templates {
		if _, exists := uniqueTemplates[template.Name]; !exists {
			templateNames = append(templateNames, template.Name)
			uniqueTemplates[template.Name] = struct{}{}
		}
	}

	return templateNames, nil
}

func SelectTemplate(templateList []string) (string, error) {
	TemplatesTab := printutil.Table{
		Padding:        []int{5, 30},
		DynamicPadding: true,
		Header:         []string{"#", "TEMPLATE"},
	}
	if len(templateList) == 0 {
		return "", fmt.Errorf("no available templates found")
	}

	templateMap := make(map[string]string)

	// Add rows for each template and index them
	for i, template := range templateList {
		index := i + 1
		TemplatesTab.AddRow([]string{strconv.Itoa(index), template}, false)
		templateMap[strconv.Itoa(index)] = template
	}

	TemplatesTab.Print(os.Stdout)

	// Prompt user for selection
	choice := input.Text("\n> ")
	selected, ok := templateMap[choice]
	if !ok {
		return "", fmt.Errorf("invalid template selection")
	}

	return selected, nil
}

func InitFromTemplate(templateDir, destDir string) error {
	HTTPClient := &httputil.HTTPClient{}
	tarballURL := fmt.Sprintf("%s/tarball/main", astroTemplateRepoURL)

	doOpts := &httputil.DoOptions{
		Path:   tarballURL,
		Method: http.MethodGet,
	}

	resp, err := HTTPClient.Do(doOpts)
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
