package airflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/fileutil"

	semver "github.com/Masterminds/semver/v3"
)

const (
	defaultDirPerm os.FileMode = 0777

	defaultAirflowVersion = uint64(0x1) //nolint:gomnd
	componentName         = "airflow"

	airflowVersionLabelName        = "io.astronomer.docker.airflow.version"
	triggererAllowedAirflowVersion = "2.2.0"

	webserverHealthCheckInterval = 10 * time.Second
)

var repoNameSanitizeRegexp = regexp.MustCompile(`^[^a-z0-9]*`) // must not start with anything except lowercase letter or number

func initDirs(root string, dirs []string) error {
	// Create the dirs
	for _, dir := range dirs {
		// Create full path to directory
		fullpath := filepath.Join(root, dir)

		// Move on if already exists
		_, err := fileutil.Exists(fullpath, nil)
		if err != nil {
			return fmt.Errorf("failed to check existence of '%s': %w", fullpath, err)
		}

		// Create directory
		if err := os.MkdirAll(fullpath, defaultDirPerm); err != nil {
			return fmt.Errorf("failed to create dir '%s': %w", dir, err)
		}
	}

	return nil
}

func initFiles(root string, files map[string]string) error {
	// Create the files
	for file, content := range files {
		// Create full path to file
		fullpath := filepath.Join(root, file)

		// Move on if already exists
		fileExist, err := fileutil.Exists(fullpath, nil)
		if err != nil {
			return fmt.Errorf("failed to check existence of '%s': %w", fullpath, err)
		}

		if fileExist {
			continue
		}

		// Write files out
		if err := fileutil.WriteStringToFile(fullpath, content); err != nil {
			return fmt.Errorf("failed to create file '%s': %w", fullpath, err)
		}
	}

	return nil
}

// Init will scaffold out a new airflow project
func Init(path, airflowImageTag string) error {
	// List of directories to create
	dirs := []string{"dags", "plugins", "include"}

	// Map of files to create
	files := map[string]string{
		".dockerignore":         include.Dockerignore,
		"Dockerfile":            fmt.Sprintf(include.Dockerfile, airflowImageTag),
		".gitignore":            include.Gitignore,
		"packages.txt":          "",
		"requirements.txt":      "",
		".env":                  "",
		"airflow_settings.yaml": include.Settingsyml,
		"dags/example-dag.py":   include.Exampledag,
	}

	containerEngine := config.CFG.ContainerEngine.GetString()
	if containerEngine == string(PodmanEngine) {
		files["pod-config.yml"] = include.PodmanConfigYml
	}

	// Initailize directories
	if err := initDirs(path, dirs); err != nil {
		return fmt.Errorf("failed to create project directories: %w", err)
	}

	// Initialize files
	if err := initFiles(path, files); err != nil {
		return fmt.Errorf("failed to create project files: %w", err)
	}

	return nil
}

func ParseVersionFromDockerFile(airflowHome, dockerfile string) (uint64, error) {
	// parse dockerfile
	cmd, err := docker.ParseFile(filepath.Join(airflowHome, dockerfile))
	if err != nil {
		return 0, fmt.Errorf("failed to parse dockerfile: %s: %w", filepath.Join(airflowHome, dockerfile), err)
	}

	_, airflowTag := docker.GetImageTagFromParsedFile(cmd)
	semVer, err := semver.NewVersion(airflowTag)
	if err != nil {
		return defaultAirflowVersion, nil // Default to Airflow 1 if the user has a custom image without a semVer tag
	}

	return semVer.Major(), nil
}

// repositoryName creates an airflow repository name
func repositoryName(name string) string {
	return fmt.Sprintf("%s/%s", sanitizeRepoName(name), componentName)
}

// imageName creates an airflow image name
func imageName(name, tag string) string {
	return fmt.Sprintf("%s:%s", repositoryName(name), tag)
}

// sanitizeRepoName updates the repoName to be compatible with docker image naming convention
func sanitizeRepoName(repoName string) string {
	return repoNameSanitizeRegexp.ReplaceAllString(repoName, "")
}
