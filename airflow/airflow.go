package airflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/iancoleman/strcase"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/version"
)

func initDirs(root string, dirs []string) error {
	// Create the dirs
	for _, dir := range dirs {
		// Create full path to directory
		fullpath := filepath.Join(root, dir)

		// Move on if already exists
		_, err := fileutil.Exists(fullpath)
		if err != nil {
			return errors.Wrapf(err, "failed to check existence of '%s'", fullpath)
		}

		// Create directory
		if err := os.MkdirAll(dir, 0777); err != nil {
			return errors.Wrapf(err, "failed to create dir '%s'", dir)
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
		fileExist, err := fileutil.Exists(fullpath)
		if err != nil {
			return errors.Wrapf(err, "failed to check existence of '%s'", fullpath)
		}

		if fileExist {
			continue
		}

		// Write files out
		if err := fileutil.WriteStringToFile(fullpath, content); err != nil {
			return errors.Wrapf(err, "failed to create file '%s'", fullpath)
		}
	}

	return nil
}

// Init will scaffold out a new airflow project
func Init(path string, airflowVersion string) error {
	// List of directories to create
	dirs := []string{"dags", "plugins", "include"}

	// Map of files to create
	files := map[string]string{
		".dockerignore": include.Dockerignore,
		"Dockerfile": fmt.Sprintf(include.Dockerfile,
			version.GetTagFromVersion(airflowVersion)),
		"packages.txt":              "",
		"requirements.txt":          "",
		".env":                      "",
		"settings.yaml":             include.Settingsyml,
		"dags/example-dag.py":       include.Exampledag,
		"plugins/example-plugin.py": include.ExamplePlugin,
	}

	// Initailize directories
	if err := initDirs(path, dirs); err != nil {
		return errors.Wrap(err, "failed to create project directories")
	}

	// Initialize files
	if err := initFiles(path, files); err != nil {
		return errors.Wrap(err, "failed to create project files")
	}

	return nil
}

func validateOrCreateProjectName(path, projectName string) (string, error) {
	if len(projectName) != 0 {
		projectNameValid := regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).MatchString
		if !projectNameValid(projectName) {
			return "", errors.New(messages.CONFIG_PROJECT_NAME_ERROR)
		}
	} else {
		projectDirectory := filepath.Base(path)

		return strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1), nil
	}

	return projectName, nil
}
