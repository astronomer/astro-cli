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

var (
	AstroTemplateRepoURL = "https://github.com/astronomer/templates"
	RuntimeTemplateURL   = "https://updates.astronomer.io/astronomer-templates"
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
		Path:   RuntimeTemplateURL,
		Method: http.MethodGet,
	}

	resp, err := HTTPClient.Do(doOpts)
	if err != nil && resp == nil {
		return nil, fmt.Errorf("failed to get response: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d, response: %s", resp.StatusCode, string(body))
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
	templatesTab := printutil.Table{
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
		templatesTab.AddRow([]string{strconv.Itoa(index), template}, false)
		templateMap[strconv.Itoa(index)] = template
	}

	templatesTab.Print(os.Stdout)

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
	tarballURL := fmt.Sprintf("%s/tarball/main", AstroTemplateRepoURL)

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

	// Extract the tarball to the temporary directory
	err = extractTarGz(resp.Body, destDir, templateDir)
	if err != nil {
		return fmt.Errorf("failed to extract tarball: %w", err)
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
		templatePath := strings.TrimPrefix(header.Name, baseDir+"/")
		if !strings.Contains(templatePath, templateDir+"/") {
			continue
		}

		relativePath := strings.TrimPrefix(templatePath, templateDir+"/")
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

			if _, err := io.Copy(outFile, tarReader); err != nil { //nolint
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
