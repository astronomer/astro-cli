package airflowrt

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	ConstraintsBaseURL = "https://cdn.astronomer.io/runtime-constraints"
	FreezeBaseURL      = "https://cdn.astronomer.io/runtime-freeze"
	StandaloneIndexURL = "https://pip.astronomer.io/v2/"
	DefaultPython      = "3.12"
	StandaloneDir      = ".astro/standalone"
	FilePermissions    = os.FileMode(0o644)
	DirPermissions     = os.FileMode(0o755)
)

// ConstraintFiles holds paths to cached constraint/freeze files and parsed versions.
type ConstraintFiles struct {
	FreezePath     string
	AirflowVersion string
	TaskSDKVersion string
}

// FetchConstraints downloads and caches constraints/freeze files from CDN.
// The projectPath is used to determine the cache directory (.astro/standalone/).
func FetchConstraints(projectPath, tag, pythonVersion string) (*ConstraintFiles, error) {
	if pythonVersion == "" {
		pythonVersion = DefaultPython
	}

	cacheDir := filepath.Join(projectPath, StandaloneDir)
	constraintsFile := filepath.Join(cacheDir, fmt.Sprintf("constraints-%s-python-%s.txt", tag, pythonVersion))
	freezeFile := filepath.Join(cacheDir, fmt.Sprintf("freeze-%s-python-%s.txt", tag, pythonVersion))

	// Check cache
	if fileExists(constraintsFile) && fileExists(freezeFile) {
		af, err := ParsePackageVersion(constraintsFile, "apache-airflow")
		if err == nil && af != "" {
			sdk, _ := ParsePackageVersion(constraintsFile, "apache-airflow-task-sdk")
			return &ConstraintFiles{FreezePath: freezeFile, AirflowVersion: af, TaskSDKVersion: sdk}, nil
		}
	}

	if err := os.MkdirAll(cacheDir, DirPermissions); err != nil {
		return nil, fmt.Errorf("error creating standalone directory: %w", err)
	}

	// Fetch constraints file
	constraintsURL := fmt.Sprintf("%s/runtime-%s-python-%s.txt", ConstraintsBaseURL, tag, pythonVersion)
	if err := DownloadFile(constraintsURL, constraintsFile); err != nil {
		return nil, fmt.Errorf("error fetching constraints from %s: %w", constraintsURL, err)
	}

	// Fetch freeze file
	freezeURL := fmt.Sprintf("%s/runtime-%s-python-%s.txt", FreezeBaseURL, tag, pythonVersion)
	if err := DownloadFile(freezeURL, freezeFile); err != nil {
		return nil, fmt.Errorf("error fetching freeze file from %s: %w", freezeURL, err)
	}

	af, err := ParsePackageVersion(constraintsFile, "apache-airflow")
	if err != nil {
		return nil, err
	}
	sdk, _ := ParsePackageVersion(constraintsFile, "apache-airflow-task-sdk")

	return &ConstraintFiles{FreezePath: freezeFile, AirflowVersion: af, TaskSDKVersion: sdk}, nil
}

// DownloadFile fetches a URL and writes the body to dest. Variable for testing.
var DownloadFile = func(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, body, FilePermissions)
}

// ParsePackageVersion reads a constraints file and extracts the version for a given package.
func ParsePackageVersion(constraintsFile, packageName string) (string, error) {
	data, err := os.ReadFile(constraintsFile)
	if err != nil {
		return "", err
	}
	prefix := packageName + "=="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), nil
		}
	}
	return "", fmt.Errorf("could not find %s version in constraints file", packageName)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
